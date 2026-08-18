package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
	"superview/internal/auth"
	"superview/internal/config"
	"superview/internal/logger"
	"superview/internal/models"
	"superview/internal/supervisor"
)

// WebSocketConfig WebSocket配置
type WebSocketConfig struct {
	MaxConnections    int           // 最大连接数
	RateLimit         float64       // 每秒消息限制
	RateBurst         int           // 突发消息数量
	HeartbeatInterval time.Duration // 心跳间隔
	ReadTimeout       time.Duration // 读取超时
	WriteTimeout      time.Duration // 写入超时
	AllowedOrigins    []string      // 允许的来源
	MaxMessageSize    int64         // 最大消息大小
	MaxViolations     int           // 最大违规次数
}

// SessionValidator revalidates the database-backed identity associated with a
// long-lived WebSocket connection. A non-nil error revokes the connection.
type SessionValidator func(userID string, tokenVersion uint64) error

// globalAllowedOrigins 全局配置的允许来源（从 config.toml 加载）
var (
	globalAllowedOrigins []string
	allowedOriginsCache  []string // 解析后的来源缓存，避免每次握手重复解析
	allowedOriginsMu     sync.RWMutex
)

// SetAllowedOrigins 设置全局允许的来源（从配置文件加载时调用）
func SetAllowedOrigins(origins []string) {
	allowedOriginsMu.Lock()
	globalAllowedOrigins = origins
	allowedOriginsCache = nil // 失效缓存，下次解析时重建
	allowedOriginsMu.Unlock()
}

// resolveAllowedOrigins 解析允许的来源（config.toml > 环境变量 > 默认值），结果缓存。
func resolveAllowedOrigins() []string {
	allowedOriginsMu.RLock()
	if allowedOriginsCache != nil {
		cached := allowedOriginsCache
		allowedOriginsMu.RUnlock()
		return cached
	}
	allowedOriginsMu.RUnlock()

	allowedOriginsMu.Lock()
	defer allowedOriginsMu.Unlock()
	if allowedOriginsCache != nil {
		return allowedOriginsCache
	}

	origins := globalAllowedOrigins
	if len(origins) == 0 {
		if env := os.Getenv("WEBSOCKET_ALLOWED_ORIGINS"); env != "" {
			origins = strings.Split(env, ",")
		}
	}
	if len(origins) == 0 {
		origins = []string{
			"http://localhost:3000",
			"http://localhost:8081",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:8081",
		}
	}
	allowedOriginsCache = origins
	return origins
}

// GetDefaultWebSocketConfig 获取默认WebSocket配置
func GetDefaultWebSocketConfig() *WebSocketConfig {
	return &WebSocketConfig{
		MaxConnections:    500,              // Support up to 500 connections by default
		RateLimit:         10.0,             // 每秒10条消息
		RateBurst:         20,               // 突发20条消息
		HeartbeatInterval: 30 * time.Second, // 30秒心跳
		ReadTimeout:       60 * time.Second, // 60秒读取超时
		WriteTimeout:      10 * time.Second, // 10秒写入超时
		AllowedOrigins:    resolveAllowedOrigins(),
		MaxMessageSize:    1024, // 1KB最大消息大小
		MaxViolations:     5,    // 最大5次违规
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")

		for _, allowed := range resolveAllowedOrigins() {
			if origin == allowed {
				return true
			}
		}

		logger.Warn("WebSocket connection rejected", zap.String("origin", origin))
		return false
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// GetWebSocketConfigFromPerformance creates WebSocket config from performance config
func GetWebSocketConfigFromPerformance(perfConfig *config.PerformanceConfig) *WebSocketConfig {
	wsConfig := GetDefaultWebSocketConfig()

	// Override with performance config values if set
	if perfConfig.MaxWebSocketConnections > 0 {
		wsConfig.MaxConnections = perfConfig.MaxWebSocketConnections
	}

	return wsConfig
}

type Hub struct {
	clients   map[*Client]bool
	clientsMu sync.RWMutex

	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	cleanup    chan *Client // New: separate cleanup channel

	service *supervisor.SupervisorService
	config  *WebSocketConfig
	db      *gorm.DB // H-08: 用于节点名称解析

	sessionValidator   SessionValidator
	sessionValidatorMu sync.RWMutex

	connectionCount int64 // atomic

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup // New: for graceful shutdown
	closeOnce sync.Once      // Ensure Close is called only once

	// Refresh interval management
	refreshInterval time.Duration
	refreshMu       sync.RWMutex
	refreshStop     chan struct{}
	refreshWg       sync.WaitGroup

	// Log streaming offsets - shared across goroutines
	logOffsets   map[string]int
	logOffsetsMu sync.RWMutex
}

type Client struct {
	hub            *Hub
	conn           *websocket.Conn
	send           chan []byte
	closeSendOnce  sync.Once // ensure send channel is closed only once
	userID         string
	tokenVersion   uint64
	subscribed     sync.Map      // map[string]bool - thread-safe
	limiter        *rate.Limiter // 速率限制器
	lastPong       time.Time     // 最后一次pong时间
	mu             sync.RWMutex
	violationCount int  // 违规计数
	closed         bool // 连接是否已关闭
	// H-08: node ACL — 允许此客户端访问的节点 ID 集合。nil 表示无限制(超级管理员)。
	allowedNodeIDs   map[string]bool // H-08: 允许访问的节点名集合; nil 表示无限制
	allowedNodeIDsMu sync.RWMutex   // 保护 allowedNodeIDs 并发读写(L-13)
}

type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type NodeUpdateMessage struct {
	NodeName  string      `json:"node_name"`
	Processes interface{} `json:"processes"`
	Timestamp time.Time   `json:"timestamp"`
}

type ProcessStatusMessage struct {
	NodeName    string    `json:"node_name"`
	ProcessName string    `json:"process_name"`
	Status      string    `json:"status"`
	Timestamp   time.Time `json:"timestamp"`
}

type LogStreamMessage struct {
	NodeName    string                `json:"node_name"`
	ProcessName string                `json:"process_name"`
	LogType     string                `json:"log_type"`
	Entries     []supervisor.LogEntry `json:"entries"`
	Timestamp   time.Time             `json:"timestamp"`
}

// LogEntry represents a single log entry
// Deprecated: 使用 supervisor.LogEntry 代替，此定义保留仅为向后兼容
type LogEntry = supervisor.LogEntry

type SystemStatsMessage struct {
	TotalNodes       int       `json:"total_nodes"`
	ConnectedNodes   int       `json:"connected_nodes"`
	RunningProcesses int       `json:"running_processes"`
	StoppedProcesses int       `json:"stopped_processes"`
	Timestamp        time.Time `json:"timestamp"`
}

func NewHub(service *supervisor.SupervisorService) *Hub {
	return NewHubWithConfig(service, GetDefaultWebSocketConfig())
}

// NewHubWithDB 使用自定义配置和数据库句柄创建Hub(H-08 节点 ACL 名称解析需要)
func NewHubWithDB(service *supervisor.SupervisorService, db *gorm.DB) *Hub {
	hub := NewHub(service)
	hub.db = db
	return hub
}

// NewHubWithConfig 使用自定义配置创建Hub
func NewHubWithConfig(service *supervisor.SupervisorService, config *WebSocketConfig) *Hub {
	ctx, cancel := context.WithCancel(context.Background())

	hub := &Hub{
		clients:         make(map[*Client]bool),
		broadcast:       make(chan []byte, 256),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		cleanup:         make(chan *Client, 100), // Buffered cleanup channel
		service:         service,
		config:          config,

		ctx:             ctx,
		cancel:          cancel,
		refreshInterval: 5 * time.Second, // 默认 5 秒
		refreshStop:     make(chan struct{}),
		logOffsets:      make(map[string]int),
	}

	// Pre-add WaitGroup count for background goroutines
	hub.wg.Add(3) // heartbeat, cleanup, log streaming
	return hub
}

// Close 关闭Hub
func (h *Hub) Close() {
	// 使用 sync.Once 确保只关闭一次
	h.closeOnce.Do(func() {
		h.cancel()
		// Wait for all goroutines to finish with timeout
		done := make(chan struct{})
		go func() {
			h.wg.Wait()
			h.refreshWg.Wait()
			close(done)
		}()

		select {
		case <-done:
			logger.Info("Hub closed gracefully")
		case <-time.After(10 * time.Second):
			logger.Warn("Hub close timeout, some goroutines may still be running")
		}
	})
}

// SetRefreshInterval 设置刷新间隔并重启定期更新
func (h *Hub) SetRefreshInterval(interval time.Duration) {
	h.refreshMu.Lock()
	oldInterval := h.refreshInterval
	h.refreshInterval = interval
	h.refreshMu.Unlock()

	if oldInterval != interval {
		logger.Info("WebSocket refresh interval changed, restarting periodic updates",
			zap.Duration("old_interval", oldInterval),
			zap.Duration("new_interval", interval))

		// 停止旧的定期更新
		close(h.refreshStop)
		h.refreshWg.Wait()

		// 启动新的定期更新
		h.refreshStop = make(chan struct{})
		h.refreshWg.Add(1)
		go h.startPeriodicUpdates()
	}
}

// GetRefreshInterval 获取当前刷新间隔
func (h *Hub) GetRefreshInterval() time.Duration {
	h.refreshMu.RLock()
	defer h.refreshMu.RUnlock()
	return h.refreshInterval
}

// GetConnectionCount 获取当前连接数
func (h *Hub) GetConnectionCount() int64 {
	return atomic.LoadInt64(&h.connectionCount)
}

// SetSessionValidator configures database-backed revalidation for WebSocket
// sessions. It should normally be set before Run is started.
func (h *Hub) SetSessionValidator(validator SessionValidator) {
	h.sessionValidatorMu.Lock()
	h.sessionValidator = validator
	h.sessionValidatorMu.Unlock()
}

func (h *Hub) validateSession(userID string, tokenVersion uint64) error {
	h.sessionValidatorMu.RLock()
	validator := h.sessionValidator
	h.sessionValidatorMu.RUnlock()
	if validator == nil {
		return nil
	}
	return validator(userID, tokenVersion)
}

func (h *Hub) Run() {
	// Start background goroutines
	h.refreshWg.Add(1)
	go h.startPeriodicUpdates()
	go h.startHeartbeatChecker()
	go h.startCleanupWorker() // New: separate cleanup worker
	go h.startLogStreaming()  // New: log streaming worker

	for {
		select {
		case <-h.ctx.Done():
			logger.Info("Hub shutting down")
			return

		case client := <-h.register:
			// 检查连接数限制
			if atomic.LoadInt64(&h.connectionCount) >= int64(h.config.MaxConnections) {
				logger.Warn("Connection limit reached, rejecting new connection",
					zap.String("userID", client.userID),
					zap.Int64("current_connections", atomic.LoadInt64(&h.connectionCount)),
					zap.Int("max_connections", h.config.MaxConnections))
				client.conn.Close()
				continue
			}

			h.clientsMu.Lock()
			h.clients[client] = true
			h.clientsMu.Unlock()
			atomic.AddInt64(&h.connectionCount, 1)
			logger.Info("WebSocket client connected",
				zap.String("user_id", client.userID),
				zap.Int64("total_connections", atomic.LoadInt64(&h.connectionCount)))

			// Send initial data to new client
			go h.sendInitialData(client)

		case client := <-h.unregister:
			h.clientsMu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.closeSendOnce.Do(func() {
					close(client.send)
				})
				atomic.AddInt64(&h.connectionCount, -1)
			}
			h.clientsMu.Unlock()
			logger.Info("WebSocket client disconnected",
				zap.String("user_id", client.userID),
				zap.Int64("total_connections", atomic.LoadInt64(&h.connectionCount)))

		case message := <-h.broadcast:
			// Use collect-then-modify pattern to avoid race conditions
			h.clientsMu.RLock()
			clientsToRemove := make([]*Client, 0)

			for client := range h.clients {
				// H-08: nodes_update 广播按客户端节点 ACL 过滤,避免无权客户端
				// 通过实时通道枚举全部节点。
				payload := message
				if isNodesUpdateMessage(message) {
					payload = h.filterNodesUpdateForClient(message, client)
				}
				select {
				case client.send <- payload:
					// 发送成功
				default:
					// 客户端阻塞,收集待移除的客户端
					clientsToRemove = append(clientsToRemove, client)
				}
			}
			h.clientsMu.RUnlock()

			// 安全地移除阻塞的客户端
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
	}
}

// startCleanupWorker 启动清理工作协程
func (h *Hub) startCleanupWorker() {
	defer h.wg.Done()

	for {
		select {
		case <-h.ctx.Done():
			return
		case client := <-h.cleanup:
			h.clientsMu.Lock()
			client.mu.Lock()
			if !client.closed {
				if _, ok := h.clients[client]; ok {
					client.closed = true
					client.closeSendOnce.Do(func() {
						close(client.send)
					})
					delete(h.clients, client)
					atomic.AddInt64(&h.connectionCount, -1)
					logger.Debug("Client cleaned up",
						zap.String("user_id", client.userID))
				}
			}
			client.mu.Unlock()
			h.clientsMu.Unlock()
		}
	}
}

func (h *Hub) startPeriodicUpdates() {
	defer h.refreshWg.Done()

	h.refreshMu.RLock()
	interval := h.refreshInterval
	h.refreshMu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-h.refreshStop:
			return
		case <-ticker.C:
			h.broadcastNodesUpdate()
			h.broadcastSystemStats()
		}
	}
}

// startHeartbeatChecker 启动心跳检测
// startLogStreaming 启动日志流处理
func (h *Hub) startLogStreaming() {
	defer h.wg.Done()
	ticker := time.NewTicker(2 * time.Second) // 每2秒检查一次日志更新
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.pollAndStreamLogs()
		}
	}
}

// pollAndStreamLogs 轮询并流式传输日志
func (h *Hub) pollAndStreamLogs() {
	// 收集所有订阅的日志流
	subscribedLogs := h.getSubscribedLogStreams()
	if len(subscribedLogs) == 0 {
		return
	}

	// 并发拉取，避免单个慢节点的阻塞 RPC 拖垮整个 2 秒轮询周期。
	const maxConcurrent = 8
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for logKey := range subscribedLogs {
		wg.Add(1)
		sem <- struct{}{}
		go func(logKey string) {
			defer wg.Done()
			defer func() {
				<-sem
				if r := recover(); r != nil {
					logger.Error("panic while streaming log", zap.String("log_key", logKey), zap.Any("panic", r))
				}
			}()
			h.streamLogKey(logKey)
		}(logKey)
	}
	wg.Wait()
}

// streamLogKey 拉取单个订阅的日志流并推送给订阅的客户端。
func (h *Hub) streamLogKey(logKey string) {
	parts := strings.Split(logKey, ":")
	if len(parts) != 2 {
		return
	}

	nodeName, processName := parts[0], parts[1]

	// 获取节点
	node, err := h.service.GetNode(nodeName)
	if err != nil {
		return
	}

	// 获取当前偏移量（使用共享的 logOffsets）
	h.logOffsetsMu.RLock()
	currentOffset, exists := h.logOffsets[logKey]
	h.logOffsetsMu.RUnlock()

	if !exists {
		// 首次订阅：获取当前文件大小作为起始偏移量，不发送任何日志
		// 这样只会推送订阅之后的新日志
		fileSize, err := node.GetProcessLogSize(processName)
		if err != nil {
			logger.Debug("Failed to get log size",
				zap.String("node", nodeName),
				zap.String("process", processName),
				zap.Error(err))
			return
		}
		h.logOffsetsMu.Lock()
		h.logOffsets[logKey] = fileSize
		h.logOffsetsMu.Unlock()
		return // 不发送任何日志，等待下次轮询
	}

	// 从当前偏移量读取新日志
	logStream, err := node.GetProcessLogStream(processName, currentOffset, 50)
	if err != nil {
		logger.Debug("Failed to get log stream",
			zap.String("node", nodeName),
			zap.String("process", processName),
			zap.Error(err))
		return
	}

	// 只有当偏移量变化时才发送（说明有新日志）
	if logStream.LastOffset > currentOffset && len(logStream.Entries) > 0 {
		h.SendLogStreamToSubscribedClients(nodeName, processName, logStream)
		// 更新偏移量
		h.logOffsetsMu.Lock()
		h.logOffsets[logKey] = logStream.LastOffset
		h.logOffsetsMu.Unlock()
	}
}

// getSubscribedLogStreams 获取所有订阅的日志流
func (h *Hub) getSubscribedLogStreams() map[string]bool {
	subscribedLogs := make(map[string]bool)

	h.clientsMu.RLock()
	for client := range h.clients {
		client.subscribed.Range(func(key, value interface{}) bool {
			if keyStr, ok := key.(string); ok && strings.HasPrefix(keyStr, "logs:") {
				logKey := strings.TrimPrefix(keyStr, "logs:")
				subscribedLogs[logKey] = true
			}
			return true
		})
	}
	h.clientsMu.RUnlock()

	return subscribedLogs
}

// SendLogStreamToSubscribedClients sends log stream messages to clients subscribed to specific process logs
func (h *Hub) SendLogStreamToSubscribedClients(nodeName, processName string, logStream *supervisor.LogStream) {
	logKey := fmt.Sprintf("%s:%s", nodeName, processName)
	subscriptionKey := "logs:" + logKey

	message := Message{
		Type: "log_stream",
		Data: LogStreamMessage{
			NodeName:    nodeName,
			ProcessName: processName,
			LogType:     logStream.LogType,
			Entries:     logStream.Entries,
			Timestamp:   time.Now(),
		},
	}

	data, err := json.Marshal(message)
	if err != nil {
		logger.Error("Error marshaling log stream message",
			zap.String("node_name", nodeName),
			zap.String("process_name", processName),
			zap.Error(err))
		return
	}

	// Use collect-then-modify pattern to avoid race conditions
	h.clientsMu.RLock()
	clientsToRemove := make([]*Client, 0)
	sentCount := 0

	for client := range h.clients {
		if _, subscribed := client.subscribed.Load(subscriptionKey); subscribed {
			// L-13: 即使已订阅,若节点 ACL 已被撤销,跳过发送
			if !client.canAccessNode(nodeName) {
				continue
			}
			select {
			case client.send <- data:
				sentCount++
			default:
				// Client's send channel is full, collect for removal
				clientsToRemove = append(clientsToRemove, client)
			}
		}
	}
	h.clientsMu.RUnlock()

	if sentCount > 0 {
		logger.Debug("Sent log stream to clients",
			zap.String("node", nodeName),
			zap.String("process", processName),
			zap.Int("entries", len(logStream.Entries)),
			zap.Int("clients", sentCount))
	}

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

func (h *Hub) startHeartbeatChecker() {
	defer h.wg.Done()
	ticker := time.NewTicker(h.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.checkHeartbeats()
		}
	}
}

// checkHeartbeats 检查所有客户端的心跳状态
func (h *Hub) checkHeartbeats() {
	h.clientsMu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.clientsMu.RUnlock()

	type clientFailure struct {
		client *Client
		reason string
		err    error
	}

	failedClients := make([]clientFailure, 0)
	now := time.Now()
	heartbeatTimeout := h.config.HeartbeatInterval * 3 // 更严格的超时时间

	for _, client := range clients {
		client.mu.RLock()
		closed := client.closed
		lastPong := client.lastPong
		client.mu.RUnlock()
		if closed {
			continue
		}

		if now.Sub(lastPong) > heartbeatTimeout {
			failedClients = append(failedClients, clientFailure{
				client: client,
				reason: "heartbeat timeout",
			})
			continue
		}

		if err := h.validateSession(client.userID, client.tokenVersion); err != nil {
			if errors.Is(err, auth.ErrSessionUnavailable) {
				failedClients = append(failedClients, clientFailure{
					client: client,
					reason: "session revoked",
					err:    err,
				})
			} else {
				logger.Warn("Session validation transient error, skipping",
					zap.String("user_id", client.userID),
					zap.Error(err))
			}
		} else {
			// L-13: 会话仍然有效,刷新节点 ACL 快照,确保撤销操作实时生效
			client.refreshAllowedNodeIDs(h.db)
		}
	}

	// 收集待断开客户端 (不持有 client.mu)
	toUnregister := make([]*Client, 0, len(failedClients))
	for _, failure := range failedClients {
		client := failure.client
		client.mu.Lock()
		if !client.closed {
			client.closed = true
			fields := []zap.Field{
				zap.String("userID", client.userID),
				zap.String("reason", failure.reason),
				zap.Duration("since_last_pong", now.Sub(client.lastPong)),
			}
			if failure.err != nil {
				fields = append(fields, zap.Error(failure.err))
			}
			logger.Warn("WebSocket client validation failed, disconnecting", fields...)
			toUnregister = append(toUnregister, client)
		}
		client.mu.Unlock()
	}

	// 发送到 unregister 通道 (不持有任何锁)
	for _, client := range toUnregister {
		select {
		case h.unregister <- client:
		default:
			// 如果channel满了,直接关闭连接
			if client.conn != nil {
				client.conn.Close()
			}
		}
	}

	// 发送ping消息给所有活跃客户端
	pingMessage := map[string]interface{}{
		"type":      "ping",
		"timestamp": now.Unix(),
	}

	pingData, err := json.Marshal(pingMessage)
	if err != nil {
		logger.Error("Failed to marshal ping message", zap.Error(err))
		return
	}

	// 使用超时机制发送ping
	select {
	case h.broadcast <- pingData:
	case <-time.After(1 * time.Second):
		logger.Warn("Ping broadcast timeout, channel may be blocked")
	}
}

// canAccessNode 检查客户端是否有权访问指定节点(H-08)。
// allowedNodeIDs 为 nil(超级管理员)时允许访问所有节点。
func (c *Client) canAccessNode(nodeName string) bool {
	c.allowedNodeIDsMu.RLock()
	defer c.allowedNodeIDsMu.RUnlock()
	if c.allowedNodeIDs == nil {
		return true
	}
	return c.allowedNodeIDs[nodeName]
}

func (h *Hub) sendInitialData(client *Client) {
	// Check if client is still registered
	h.clientsMu.RLock()
	_, exists := h.clients[client]
	h.clientsMu.RUnlock()

	if !exists {
		logger.Debug("Client already disconnected, skipping initial data",
			zap.String("user_id", client.userID))
		return
	}

	// Send current nodes data (H-08: 按节点 ACL 过滤)
	nodes := h.service.GetAllNodes()
	nodesData := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		if !client.canAccessNode(node.Name) {
			continue
		}
		nodesData = append(nodesData, node.Serialize())
	}

	message := Message{
		Type: "nodes_update",
		Data: nodesData,
	}

	data, err := json.Marshal(message)
	if err != nil {
		logger.Error("Error marshaling initial data",
			zap.Error(err),
			zap.String("user_id", client.userID))
		return
	}

	// Try to send with timeout, but don't panic if channel is closed(M-35)
	if !trySend(client.send, data) {
		// channel is full or closed — client may have disconnected
		logger.Warn("Failed to send initial data to client, channel full or closed",
			zap.String("user_id", client.userID))
	}
}

// isNodesUpdateMessage 判断广播消息是否为 nodes_update 类型(H-08)。
func isNodesUpdateMessage(message []byte) bool {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(message, &probe); err != nil {
		return false
	}
	return probe.Type == "nodes_update"
}

// filterNodesUpdateForClient 按客户端节点 ACL 过滤 nodes_update 广播(H-08)。
// 返回过滤后的消息字节;若客户端无任何可见节点,返回包含空列表的消息。
func (h *Hub) filterNodesUpdateForClient(message []byte, client *Client) []byte {
	var msg Message
	if err := json.Unmarshal(message, &msg); err != nil {
		return message
	}
	nodesData, ok := msg.Data.([]interface{})
	if !ok {
		return message
	}

	filtered := make([]interface{}, 0, len(nodesData))
	for _, item := range nodesData {
		nodeMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := nodeMap["name"].(string)
		if name != "" && client.canAccessNode(name) {
			filtered = append(filtered, item)
		}
	}
	msg.Data = filtered
	out, err := json.Marshal(msg)
	if err != nil {
		return message
	}
	return out
}

// trySend 安全地向客户端通道发送数据,捕获因通道关闭导致的 panic(H-13/M-35)。
// 通道关闭后向其发送数据会 panic 而非阻塞,因此 select-default 不足以防御。
func trySend(ch chan []byte, data []byte) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	select {
	case ch <- data:
		return true
	default:
		return false
	}
}

// refreshAllowedNodeIDs 从数据库重载客户端的节点 ACL,确保撤销操作实时生效(L-13)。
// 心跳校验时调用,在释放 client.mu 锁后安全更新。
func (c *Client) refreshAllowedNodeIDs(db *gorm.DB) {
	if db == nil {
		return
	}
	var user models.User
	if err := db.Preload("NodeAccess").First(&user, "id = ?", c.userID).Error; err != nil {
		return
	}
	if user.IsSuperAdmin() {
		c.allowedNodeIDsMu.Lock()
		c.allowedNodeIDs = nil
		c.allowedNodeIDsMu.Unlock()
		return
	}
	newAllowed := make(map[string]bool, len(user.NodeAccess))
	for _, access := range user.NodeAccess {
		if access.CanRead {
			var dbNode models.Node
			if err := db.First(&dbNode, "id = ?", access.NodeID).Error; err == nil {
				newAllowed[dbNode.Name] = true
			}
		}
	}
	c.allowedNodeIDsMu.Lock()
	c.allowedNodeIDs = newAllowed
	c.allowedNodeIDsMu.Unlock()
}

func (h *Hub) broadcastNodesUpdate() {
	nodes := h.service.GetAllNodes()
	nodesData := make([]map[string]interface{}, len(nodes))
	for i, node := range nodes {
		nodesData[i] = node.Serialize()
	}

	message := Message{
		Type: "nodes_update",
		Data: nodesData,
	}

	data, err := json.Marshal(message)
	if err != nil {
		logger.Error("Error marshaling nodes update", zap.Error(err))
		return
	}

	select {
	case h.broadcast <- data:
	default:
		logger.Warn("Broadcast channel full, dropping nodes update")
	}
}

func (h *Hub) broadcastSystemStats() {
	nodes := h.service.GetAllNodes()
	totalNodes := len(nodes)
	connectedNodes := 0
	runningProcesses := 0
	stoppedProcesses := 0

	for _, node := range nodes {
		if node.IsConnected {
			connectedNodes++
			for _, process := range node.Processes {
				if process.State == 20 { // RUNNING state in supervisor
					runningProcesses++
				} else {
					stoppedProcesses++
				}
			}
		}
	}

	stats := SystemStatsMessage{
		TotalNodes:       totalNodes,
		ConnectedNodes:   connectedNodes,
		RunningProcesses: runningProcesses,
		StoppedProcesses: stoppedProcesses,
		Timestamp:        time.Now(),
	}

	message := Message{
		Type: "system_stats",
		Data: stats,
	}

	data, err := json.Marshal(message)
	if err != nil {
		logger.Error("Error marshaling system stats", zap.Error(err))
		return
	}

	select {
	case h.broadcast <- data:
	default:
		logger.Warn("Broadcast channel full, dropping system stats")
	}
}

func (h *Hub) BroadcastProcessStatusChange(nodeName, processName, status string) {
	message := Message{
		Type: "process_status_change",
		Data: ProcessStatusMessage{
			NodeName:    nodeName,
			ProcessName: processName,
			Status:      status,
			Timestamp:   time.Now(),
		},
	}

	data, err := json.Marshal(message)
	if err != nil {
		logger.Error("Error marshaling process status change", zap.Error(err))
		return
	}

	select {
	case h.broadcast <- data:
	default:
		logger.Warn("Broadcast channel full, dropping process status change")
	}
}

// handleViolation 处理客户端违规行为
func (c *Client) handleViolation(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.violationCount++
	logger.Warn("Client violation detected",
		zap.String("userID", c.userID),
		zap.String("reason", reason),
		zap.Int("violationCount", c.violationCount),
		zap.Int("maxViolations", c.hub.config.MaxViolations))

	// 如果违规次数超过阈值，强制断开连接
	if c.violationCount >= c.hub.config.MaxViolations {
		c.closed = true
		logger.Error("Client exceeded max violations, force disconnecting",
			zap.String("userID", c.userID),
			zap.Int("violationCount", c.violationCount))

		// 异步关闭连接以避免阻塞
		go func() {
			c.conn.Close()
		}()
	}
}

func (h *Hub) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("WebSocket upgrade error", zap.Error(err))
		return
	}

	// Get user ID from context (set by auth middleware)
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetString("userID")
	}
	if userID == "" {
		userID = "anonymous"
	}
	tokenVersion, _ := c.Get("token_version")
	version, _ := tokenVersion.(uint64)

	// 设置连接超时
	conn.SetReadDeadline(time.Now().Add(h.config.ReadTimeout))
	conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout))

	// 创建速率限制器
	limiter := rate.NewLimiter(rate.Limit(h.config.RateLimit), h.config.RateBurst)

	// H-08: 从鉴权中间件上下文加载用户及其节点 ACL,用于连接级节点过滤。
	// user 由 cmd/main.go 的 /ws 路由通过 auth.AuthenticateToken 设置,
	// 已包含 Roles.Permissions 与 NodeAccess(见 loadUserForSession)。
	var allowedNodeIDs map[string]bool
	if userObj, exists := c.Get("user"); exists {
		if u, ok := userObj.(*models.User); ok {
			if !u.IsSuperAdmin() && len(u.NodeAccess) > 0 && h.db != nil {
				allowedNodeIDs = make(map[string]bool, len(u.NodeAccess))
				for _, access := range u.NodeAccess {
					// 通过 NodeID 查询对应的 DB 节点名,构建 name→true 映射
					var dbNode models.Node
					if err := h.db.First(&dbNode, "id = ?", access.NodeID).Error; err == nil {
						allowedNodeIDs[dbNode.Name] = access.CanRead
					}
				}
			}
		}
	}

	client := &Client{
		hub:            h,
		conn:           conn,
		send:           make(chan []byte, 256),
		userID:         userID,
		tokenVersion:   version,
		limiter:        limiter,
		lastPong:       time.Now(),
		violationCount: 0,
		closed:         false,
		allowedNodeIDs: allowedNodeIDs,
	}

	// 设置pong处理器
	conn.SetPongHandler(func(string) error {
		client.mu.Lock()
		client.lastPong = time.Now()
		client.mu.Unlock()
		conn.SetReadDeadline(time.Now().Add(h.config.ReadTimeout))
		return nil
	})

	// 注册前校验会话:慢速 DB 查询发生在这里,而不是阻塞 Run() 主循环。
	if err := h.validateSession(client.userID, client.tokenVersion); err != nil {
		logger.Warn("WebSocket session rejected during registration",
			zap.String("user_id", client.userID),
			zap.Error(err))
		client.conn.Close()
		return
	}

	// 非阻塞注册:若 Run() 主循环繁忙(register 无缓冲),立即拒绝连接而不是阻塞握手。
	select {
	case client.hub.register <- client:
	default:
		logger.Warn("WebSocket register channel full, rejecting new connection",
			zap.String("user_id", client.userID))
		client.conn.Close()
		return
	}

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(data []byte) {
	select {
	case h.broadcast <- data:
	default:
		logger.Warn("Broadcast channel full, message dropped")
	}
}
