package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
	"superview/internal/config"
	"superview/internal/logger"
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
	userID         string
	subscribed     sync.Map      // map[string]bool - thread-safe
	limiter        *rate.Limiter // 速率限制器
	lastPong       time.Time     // 最后一次pong时间
	mu             sync.RWMutex
	violationCount int  // 违规计数
	closed         bool // 连接是否已关闭
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
				close(client.send)
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
				select {
				case client.send <- message:
					// 发送成功
				default:
					// 客户端阻塞，收集待移除的客户端
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
			if _, ok := h.clients[client]; ok {
				close(client.send)
				delete(h.clients, client)
				atomic.AddInt64(&h.connectionCount, -1)
				logger.Debug("Client cleaned up",
					zap.String("user_id", client.userID))
			}
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

	for logKey := range subscribedLogs {
		parts := strings.Split(logKey, ":")
		if len(parts) != 2 {
			continue
		}

		nodeName, processName := parts[0], parts[1]

		// 获取节点
		node, err := h.service.GetNode(nodeName)
		if err != nil {
			continue
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
				continue
			}
			h.logOffsetsMu.Lock()
			h.logOffsets[logKey] = fileSize
			h.logOffsetsMu.Unlock()
			continue // 不发送任何日志，等待下次轮询
		}

		// 从当前偏移量读取新日志
		logStream, err := node.GetProcessLogStream(processName, currentOffset, 50)
		if err != nil {
			logger.Debug("Failed to get log stream",
				zap.String("node", nodeName),
				zap.String("process", processName),
				zap.Error(err))
			continue
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
	deadClients := make([]*Client, 0)
	now := time.Now()
	heartbeatTimeout := h.config.HeartbeatInterval * 3 // 更严格的超时时间

	for client := range h.clients {
		client.mu.RLock()
		// 检查客户端是否已关闭或超时
		if client.closed || now.Sub(client.lastPong) > heartbeatTimeout {
			if !client.closed {
				deadClients = append(deadClients, client)
			}
		}
		client.mu.RUnlock()
	}
	h.clientsMu.RUnlock()

	// 安全地断开超时的客户端
	for _, client := range deadClients {
		client.mu.Lock()
		if !client.closed {
			client.closed = true
			logger.Warn("Client heartbeat timeout, disconnecting",
				zap.String("userID", client.userID),
				zap.Duration("timeout", now.Sub(client.lastPong)))

			// 通过unregister channel安全地移除客户端
			select {
			case h.unregister <- client:
			default:
				// 如果channel满了，直接关闭连接
				client.conn.Close()
			}
		}
		client.mu.Unlock()
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

	// Send current nodes data
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
		logger.Error("Error marshaling initial data",
			zap.Error(err),
			zap.String("user_id", client.userID))
		return
	}

	// Try to send with timeout, but don't panic if channel is closed
	select {
	case client.send <- data:
		// Successfully sent
	case <-time.After(1 * time.Second):
		// Timeout - client may have disconnected
		logger.Warn("Failed to send initial data to client, timeout",
			zap.String("user_id", client.userID))
	}
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

	// 设置连接超时
	conn.SetReadDeadline(time.Now().Add(h.config.ReadTimeout))
	conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout))

	// 创建速率限制器
	limiter := rate.NewLimiter(rate.Limit(h.config.RateLimit), h.config.RateBurst)

	client := &Client{
		hub:            h,
		conn:           conn,
		send:           make(chan []byte, 256),
		userID:         userID,
		limiter:        limiter,
		lastPong:       time.Now(),
		violationCount: 0,
		closed:         false,
	}

	// 设置pong处理器
	conn.SetPongHandler(func(string) error {
		client.mu.Lock()
		client.lastPong = time.Now()
		client.mu.Unlock()
		conn.SetReadDeadline(time.Now().Add(h.config.ReadTimeout))
		return nil
	})

	client.hub.register <- client

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
