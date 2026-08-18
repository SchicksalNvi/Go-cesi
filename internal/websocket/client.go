package websocket

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"superview/internal/logger"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

type ClientMessage struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

func (c *Client) readPump() {
	defer func() {
		// 非阻塞发送 unregister,避免在 Hub 主循环繁忙时阻塞本 goroutine
		select {
		case c.hub.unregister <- c:
		default:
			logger.Debug("Unregister channel full, closing connection directly",
				zap.String("user_id", c.userID))
		}
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		// 更新 lastPong 供心跳检测使用,并延长读取截止时间
		c.mu.Lock()
		c.lastPong = time.Now()
		c.mu.Unlock()
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Debug("WebSocket unexpected close error",
					zap.String("user_id", c.userID),
					zap.Error(err))
			}
			break
		}

		// Handle incoming messages from client
		var clientMsg ClientMessage
		if err := json.Unmarshal(message, &clientMsg); err != nil {
			logger.Error("Error unmarshaling client message",
				zap.String("user_id", c.userID),
				zap.Error(err))
			continue
		}

		c.handleClientMessage(clientMsg)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleClientMessage(msg ClientMessage) {
	switch msg.Type {
	case "subscribe_node":
		if nodeName, ok := msg.Data["node_name"].(string); ok {
			// H-08: 订阅节点前校验节点 ACL,无权访问直接拒绝
			if !c.canAccessNode(nodeName) {
				logger.Warn("Client denied node subscription: no access",
					zap.String("user_id", c.userID),
					zap.String("node_name", nodeName))
				c.handleViolation("subscribe_node: no node access")
				return
			}
			c.subscribed.Store(nodeName, true)
			logger.Info("Client subscribed to node",
				zap.String("user_id", c.userID),
				zap.String("node_name", nodeName))

			// Send current node data
			if _, err := c.hub.service.GetNode(nodeName); err == nil {
				processes, _ := c.hub.service.GetNodeProcesses(nodeName)

				updateMsg := Message{
					Type: "process_update",
					Data: NodeUpdateMessage{
						NodeName:  nodeName,
						Processes: processes,
						Timestamp: time.Now(),
					},
				}

				if data, err := json.Marshal(updateMsg); err == nil {
					select {
					case c.send <- data:
					default:
						logger.Warn("Client send channel full",
							zap.String("user_id", c.userID))
					}
				}
			}
		}

	case "unsubscribe_node":
		if nodeName, ok := msg.Data["node_name"].(string); ok {
			c.subscribed.Delete(nodeName)
			logger.Info("Client unsubscribed from node",
				zap.String("user_id", c.userID),
				zap.String("node_name", nodeName))
		}

	case "request_node_update":
		if nodeName, ok := msg.Data["node_name"].(string); ok {
			// H-08: 强制刷新前校验节点 ACL
			if !c.canAccessNode(nodeName) {
				logger.Warn("Client denied node update request: no access",
					zap.String("user_id", c.userID),
					zap.String("node_name", nodeName))
				c.handleViolation("request_node_update: no node access")
				return
			}
			logger.Info("Client requested node update",
				zap.String("user_id", c.userID),
				zap.String("node_name", nodeName))

			// Force refresh and send updated data
			if node, err := c.hub.service.GetNode(nodeName); err == nil {
				node.RefreshProcesses()
				processes, _ := c.hub.service.GetNodeProcesses(nodeName)

				updateMsg := Message{
					Type: "process_update",
					Data: NodeUpdateMessage{
						NodeName:  nodeName,
						Processes: processes,
						Timestamp: time.Now(),
					},
				}

				if data, err := json.Marshal(updateMsg); err == nil {
					select {
					case c.send <- data:
					default:
						logger.Warn("Client send channel full",
							zap.String("user_id", c.userID))
					}
				}
			}
		}

	case "subscribe_logs":
		if nodeName, ok := msg.Data["node_name"].(string); ok {
			// H-08: 订阅日志前校验节点 ACL
			if !c.canAccessNode(nodeName) {
				logger.Warn("Client denied log subscription: no node access",
					zap.String("user_id", c.userID),
					zap.String("node_name", nodeName))
				c.handleViolation("subscribe_logs: no node access")
				return
			}
			if processName, ok := msg.Data["process_name"].(string); ok {
				logKey := fmt.Sprintf("%s:%s", nodeName, processName)
				c.subscribed.Store("logs:"+logKey, true)
				logger.Info("Client subscribed to process logs",
					zap.String("user_id", c.userID),
					zap.String("node_name", nodeName),
					zap.String("process_name", processName))
				// 不再发送初始日志，前端通过 REST API 获取初始日志
				// WebSocket 只负责推送增量更新
			}
		}

	case "unsubscribe_logs":
		if nodeName, ok := msg.Data["node_name"].(string); ok {
			if processName, ok := msg.Data["process_name"].(string); ok {
				logKey := fmt.Sprintf("%s:%s", nodeName, processName)
				c.subscribed.Delete("logs:" + logKey)
				logger.Info("Client unsubscribed from process logs",
					zap.String("user_id", c.userID),
					zap.String("node_name", nodeName),
					zap.String("process_name", processName))
			}
		}

	case "ping":
		// Respond with pong
		pongMsg := Message{
			Type: "pong",
			Data: map[string]interface{}{
				"timestamp": time.Now(),
			},
		}

		if data, err := json.Marshal(pongMsg); err == nil {
			select {
			case c.send <- data:
			default:
				logger.Warn("Client send channel full",
					zap.String("user_id", c.userID))
			}
		}

	default:
		logger.Warn("Unknown message type",
			zap.String("user_id", c.userID),
			zap.String("message_type", msg.Type))
	}
}

// SendToSubscribedClients sends a message to all clients subscribed to a specific node
func (h *Hub) SendToSubscribedClients(nodeName string, message Message) {
	data, err := json.Marshal(message)
	if err != nil {
		logger.Error("Error marshaling message for node",
			zap.String("node_name", nodeName),
			zap.Error(err))
		return
	}

	// Use collect-then-modify pattern to avoid race conditions
	h.clientsMu.RLock()
	clientsToRemove := make([]*Client, 0)
	
	for client := range h.clients {
		if _, subscribed := client.subscribed.Load(nodeName); subscribed {
			select {
			case client.send <- data:
				// Successfully sent
			default:
				// Client's send channel is full, collect for removal
				clientsToRemove = append(clientsToRemove, client)
			}
		}
	}
	h.clientsMu.RUnlock()
	
	// Remove clients with full channels via cleanup worker
	for _, client := range clientsToRemove {
		select {
		case h.cleanup <- client:
		default:
			// Cleanup channel full, force close
			logger.Warn("Cleanup channel full, force closing client",
				zap.String("user_id", client.userID))
			client.conn.Close()
		}
	}
}
