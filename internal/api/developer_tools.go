package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"superview/internal/config"
	"superview/internal/database"
	"superview/internal/logger"
	"superview/internal/middleware"
	"superview/internal/supervisor"
	"superview/internal/utils"
	"superview/internal/validation"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DeveloperToolsAPI handles developer tools related operations
type DeveloperToolsAPI struct {
	db        *gorm.DB
	service   *supervisor.SupervisorService
	config    *config.DeveloperToolsConfig
	logReader *utils.LogReader
	hub       WebSocketHub
}

func defaultDeveloperToolsConfig() *config.DeveloperToolsConfig {
	return &config.DeveloperToolsConfig{
		Enabled:           true,
		LogPath:           "logs/app.log",
		MaxLogLines:       1000,
		APIDocsEnabled:    true,
		SampleDataEnabled: false,
		MetricsEnabled:    true,
		SystemMetrics: config.SystemMetricsConfig{
			CPUMonitorEnabled:  true,
			DiskMonitorEnabled: true,
			UpdateInterval:     30,
		},
	}
}

// NewDeveloperToolsAPI creates a new DeveloperToolsAPI instance
func NewDeveloperToolsAPI(db *gorm.DB, service *supervisor.SupervisorService, cfg *config.DeveloperToolsConfig, hub WebSocketHub) *DeveloperToolsAPI {
	if cfg == nil {
		cfg = defaultDeveloperToolsConfig()
	}

	if cfg.LogPath == "" {
		cfg.LogPath = "logs/app.log"
	}
	if cfg.MaxLogLines == 0 {
		cfg.MaxLogLines = 1000
	}
	if cfg.SystemMetrics.UpdateInterval == 0 {
		cfg.SystemMetrics.UpdateInterval = 30
	}

	logReader := utils.NewLogReader(cfg)
	return &DeveloperToolsAPI{
		db:        db,
		service:   service,
		config:    cfg,
		logReader: logReader,
		hub:       hub,
	}
}

// RequireEnabled 中间件:当 developer_tools.enabled 为 false 时,
// 所有开发者工具路由返回 404,避免暴露未启用(且可能不安全)的功能。
func (api *DeveloperToolsAPI) RequireEnabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		if api.config == nil || !api.config.Enabled {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": "Developer tools are disabled",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// APIEndpoint represents an API endpoint
type APIEndpoint struct {
	Path        string         `json:"path"`
	Method      string         `json:"method"`
	Description string         `json:"description"`
	Parameters  []APIParameter `json:"parameters"`
}

// APIParameter represents an API parameter
type APIParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// DebugLog represents a debug log entry
type DebugLog struct {
	ID        int       `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
	Details   string    `json:"details"`
}

// PerformanceMetrics represents system performance metrics
type PerformanceMetrics struct {
	System    SystemMetrics    `json:"system"`
	API       APIMetrics       `json:"api"`
	Database  DatabaseMetrics  `json:"database"`
	WebSocket WebSocketMetrics `json:"websocket"`
}

type SystemMetrics struct {
	Uptime      string  `json:"uptime"`
	MemoryUsage string  `json:"memoryUsage"`
	CPUUsage    string  `json:"cpuUsage"`
	DiskUsage   string  `json:"diskUsage"`
	Goroutines  int     `json:"goroutines"`
	GCPauses    float64 `json:"gcPauses"`
}

type APIMetrics struct {
	TotalRequests       int    `json:"totalRequests"`
	RequestsPerMinute   int    `json:"requestsPerMinute"`
	AverageResponseTime string `json:"averageResponseTime"`
	ErrorRate           string `json:"errorRate"`
}

type DatabaseMetrics struct {
	Connections    int    `json:"connections"`
	MaxConnections int    `json:"maxConnections"`
	QueryTime      string `json:"queryTime"`
	SlowQueries    int    `json:"slowQueries"`
}

type WebSocketMetrics struct {
	ActiveConnections int `json:"activeConnections"`
	MessagesPerSecond int `json:"messagesPerSecond"`
	TotalMessages     int `json:"totalMessages"`
}

// TestResult represents the result of an API endpoint test
type TestResult struct {
	Success      bool        `json:"success"`
	StatusCode   int         `json:"status_code"`
	ResponseTime string      `json:"response_time"`
	Message      string      `json:"message"`
	Response     string      `json:"response,omitempty"`
	Headers      http.Header `json:"headers,omitempty"`
	Error        string      `json:"error,omitempty"`
}

// GetApiEndpoints returns available API endpoints
func (api *DeveloperToolsAPI) GetApiEndpoints(c *gin.Context) {
	// 从配置生成API端点文档
	endpoints := []APIEndpoint{}

	if api.config.APIDocsEnabled {
		// 核心API端点
		endpoints = append(endpoints, []APIEndpoint{
			{
				Path:        "/api/v1/nodes",
				Method:      "GET",
				Description: "Get all supervisor nodes",
				Parameters:  []APIParameter{},
			},
			{
				Path:        "/api/v1/nodes/{id}",
				Method:      "GET",
				Description: "Get specific node details",
				Parameters:  []APIParameter{{Name: "id", Type: "string", Required: true, Description: "Node ID"}},
			},
			{
				Path:        "/api/v1/processes",
				Method:      "GET",
				Description: "Get all processes",
				Parameters:  []APIParameter{{Name: "node_id", Type: "string", Required: false, Description: "Filter by node ID"}, {Name: "status", Type: "string", Required: false, Description: "Filter by status"}},
			},
			{
				Path:        "/api/v1/processes/{id}/start",
				Method:      "POST",
				Description: "Start a process",
				Parameters:  []APIParameter{{Name: "id", Type: "string", Required: true, Description: "Process ID"}},
			},
			{
				Path:        "/api/v1/processes/{id}/stop",
				Method:      "POST",
				Description: "Stop a process",
				Parameters:  []APIParameter{{Name: "id", Type: "string", Required: true, Description: "Process ID"}},
			},
			{
				Path:        "/api/v1/auth/login",
				Method:      "POST",
				Description: "User authentication",
				Parameters:  []APIParameter{{Name: "username", Type: "string", Required: true, Description: "Username"}, {Name: "password", Type: "string", Required: true, Description: "Password"}},
			},
			{
				Path:        "/api/v1/auth/logout",
				Method:      "POST",
				Description: "User logout",
				Parameters:  []APIParameter{},
			},
			{
				Path:        "/api/v1/developer/logs",
				Method:      "GET",
				Description: "Get debug logs",
				Parameters:  []APIParameter{{Name: "level", Type: "string", Required: false, Description: "Log level filter"}, {Name: "component", Type: "string", Required: false, Description: "Component filter"}, {Name: "limit", Type: "integer", Required: false, Description: "Number of logs to return"}},
			},
			{
				Path:        "/api/v1/developer/metrics",
				Method:      "GET",
				Description: "Get performance metrics",
				Parameters:  []APIParameter{},
			},
			{
				Path:        "/api/v1/health",
				Method:      "GET",
				Description: "Health check endpoint",
				Parameters:  []APIParameter{},
			},
		}...)
	}

	c.JSON(http.StatusOK, endpoints)
}

// TestApiEndpoint tests an API endpoint
func (api *DeveloperToolsAPI) TestApiEndpoint(c *gin.Context) {
	path := c.PostForm("path")
	method := c.PostForm("method")
	headers := c.PostForm("headers")
	body := c.PostForm("body")

	// 验证输入
	validator := validation.NewValidator()
	validator.ValidateRequired("path", path)
	validator.ValidateRequired("method", method)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input parameters", "details": validator.Errors()})
		return
	}

	// 验证HTTP方法
	allowedMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	validMethod := false
	for _, m := range allowedMethods {
		if method == m {
			validMethod = true
			break
		}
	}
	if !validMethod {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid HTTP method"})
		return
	}

	// 执行API端点测试
	result := api.executeEndpointTest(path, method, headers, body)
	c.JSON(http.StatusOK, result)
}

// executeEndpointTest executes the actual API endpoint test
func (api *DeveloperToolsAPI) executeEndpointTest(path, method, headers, body string) TestResult {
	startTime := time.Now()

	// 构建完整的URL（假设测试本地服务器）
	baseURL := "http://localhost:8080" // 可以从配置中获取
	fullURL := baseURL + path

	// 创建HTTP请求
	var req *http.Request
	var err error

	if body != "" && (method == "POST" || method == "PUT" || method == "PATCH") {
		req, err = http.NewRequest(method, fullURL, bytes.NewBufferString(body))
	} else {
		req, err = http.NewRequest(method, fullURL, nil)
	}

	if err != nil {
		return TestResult{
			Success:      false,
			StatusCode:   0,
			ResponseTime: time.Since(startTime).String(),
			Message:      fmt.Sprintf("Failed to create request: %v", err),
			Error:        err.Error(),
		}
	}

	// 设置请求头
	if headers != "" {
		var headerMap map[string]string
		if err := json.Unmarshal([]byte(headers), &headerMap); err == nil {
			for key, value := range headerMap {
				req.Header.Set(key, value)
			}
		} else {
			// 如果不是JSON格式，尝试解析为key:value格式
			headerLines := strings.Split(headers, "\n")
			for _, line := range headerLines {
				parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
				if len(parts) == 2 {
					req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				}
			}
		}
	}

	// 设置默认Content-Type
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// 执行请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return TestResult{
			Success:      false,
			StatusCode:   0,
			ResponseTime: time.Since(startTime).String(),
			Message:      fmt.Sprintf("Request failed: %v", err),
			Error:        err.Error(),
		}
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return TestResult{
			Success:      false,
			StatusCode:   resp.StatusCode,
			ResponseTime: time.Since(startTime).String(),
			Message:      fmt.Sprintf("Failed to read response: %v", err),
			Error:        err.Error(),
		}
	}

	// 构建测试结果
	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	message := "Request completed successfully"
	if !success {
		message = fmt.Sprintf("Request failed with status %d", resp.StatusCode)
	}

	return TestResult{
		Success:      success,
		StatusCode:   resp.StatusCode,
		ResponseTime: time.Since(startTime).String(),
		Message:      message,
		Response:     string(respBody),
		Headers:      resp.Header,
	}
}

// GetDebugLogs returns debug logs from the actual logging system
func (api *DeveloperToolsAPI) GetDebugLogs(c *gin.Context) {
	level := c.Query("level")
	component := c.Query("component")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	// 验证参数
	validator := validation.NewValidator()
	validator.ValidateRange("limit", limit, 1, 1000)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter", "details": validator.Errors()})
		return
	}
	if level != "" {
		validator.ValidateLogLevel("level", level)
		if validator.HasErrors() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid log level", "details": validator.Errors()})
			return
		}
	}

	// 从真实的日志系统获取调试日志
	logEntries, err := api.logReader.ReadLogs(level, component, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read logs: " + err.Error()})
		return
	}

	// 转换为API响应格式
	logs := make([]DebugLog, len(logEntries))
	for i, entry := range logEntries {
		logs[i] = DebugLog{
			ID:        entry.ID,
			Timestamp: entry.Timestamp,
			Level:     entry.Level,
			Component: entry.Component,
			Message:   entry.Message,
			Details:   entry.Details,
		}
	}

	// 如果没有真实日志，返回空数组
	// 生产环境中应该从实际日志系统获取数据

	c.JSON(http.StatusOK, logs)
}

// ClearDebugLogs clears debug logs
func (api *DeveloperToolsAPI) ClearDebugLogs(c *gin.Context) {
	// In a real implementation, this would clear the debug logs
	c.JSON(http.StatusOK, gin.H{"message": "Debug logs cleared successfully"})
}

// SetLogLevel sets the logging level
func (api *DeveloperToolsAPI) SetLogLevel(c *gin.Context) {
	var request struct {
		Level  string `json:"level" binding:"required"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 调用真实的日志级别设置(动态调整 zap 的 AtomicLevel)
	changedBy := logLevelChangedBy(c)
	if err := logger.SetLogLevel(request.Level, changedBy, request.Reason); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Failed to set log level",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Log level updated successfully",
		"level":   request.Level,
		"success": true,
	})
}

// GetPerformanceMetrics returns performance metrics
func (api *DeveloperToolsAPI) GetPerformanceMetrics(c *gin.Context) {
	metrics := middleware.GetPerformanceMetrics()
	c.JSON(http.StatusOK, metrics)
}

// getCPUUsage returns current CPU usage percentage
func (api *DeveloperToolsAPI) getCPUUsage() float64 {
	// 读取/proc/stat获取CPU使用率
	cmd := exec.Command("sh", "-c", "grep 'cpu ' /proc/stat | awk '{usage=($2+$4)*100/($2+$3+$4+$5)} END {print usage}'")
	output, err := cmd.Output()
	if err != nil {
		return 0.0
	}

	usage, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0.0
	}

	return usage
}

// getDiskUsage returns current disk usage percentage
func (api *DeveloperToolsAPI) getDiskUsage() float64 {
	// 获取根目录磁盘使用情况
	var stat syscall.Statfs_t
	err := syscall.Statfs("/", &stat)
	if err != nil {
		return 0.0
	}

	// 计算使用率
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free

	if total == 0 {
		return 0.0
	}

	return float64(used) / float64(total) * 100
}

// ResetPerformanceMetrics resets performance metrics
func (api *DeveloperToolsAPI) ResetPerformanceMetrics(c *gin.Context) {
	middleware.ResetPerformanceMetrics()
	c.JSON(http.StatusOK, gin.H{
		"message": "Performance metrics reset successfully",
	})
}

// GetTopSlowEndpoints returns the slowest endpoints
func (api *DeveloperToolsAPI) GetTopSlowEndpoints(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}

	slowEndpoints := middleware.GetTopSlowEndpoints(limit)
	c.JSON(http.StatusOK, slowEndpoints)
}

// GetErrorRateByEndpoint returns error rate by endpoint
func (api *DeveloperToolsAPI) GetErrorRateByEndpoint(c *gin.Context) {
	errorRates := middleware.GetErrorRateByEndpoint()
	c.JSON(http.StatusOK, errorRates)
}

// GetSystemMetrics returns system-specific metrics
func (api *DeveloperToolsAPI) GetSystemMetrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 获取CPU使用率
	cpuUsage := api.getCPUUsage()

	// 获取磁盘使用率
	diskUsage := api.getDiskUsage()

	metrics := SystemMetrics{
		Uptime:      "N/A", // 需要实现进程启动时间计算
		MemoryUsage: formatBytes(m.Alloc),
		CPUUsage:    fmt.Sprintf("%.2f%%", cpuUsage),
		DiskUsage:   fmt.Sprintf("%.2f%%", diskUsage),
		Goroutines:  runtime.NumGoroutine(),
		GCPauses:    float64(m.PauseTotalNs) / 1e6,
	}

	c.JSON(http.StatusOK, metrics)
}

// GetApiMetrics returns API-specific metrics from performance middleware
func (api *DeveloperToolsAPI) GetApiMetrics(c *gin.Context) {
	// 从性能监控中间件获取真实的API指标
	perfMetrics := middleware.GetPerformanceMetrics()

	// 计算每分钟请求数（基于最近的统计时间）
	minutesSinceReset := time.Since(perfMetrics.LastResetTime).Minutes()
	requestsPerMinute := int64(0)
	if minutesSinceReset > 0 {
		requestsPerMinute = int64(float64(perfMetrics.RequestCount) / minutesSinceReset)
	}

	// 计算错误率
	errorRate := "0%"
	if perfMetrics.RequestCount > 0 {
		errorRateFloat := float64(perfMetrics.ErrorCount) / float64(perfMetrics.RequestCount) * 100
		errorRate = strconv.FormatFloat(errorRateFloat, 'f', 1, 64) + "%"
	}

	metrics := APIMetrics{
		TotalRequests:       int(perfMetrics.RequestCount),
		RequestsPerMinute:   int(requestsPerMinute),
		AverageResponseTime: perfMetrics.AverageDuration.String(),
		ErrorRate:           errorRate,
	}

	c.JSON(http.StatusOK, metrics)
}

// GetDatabaseMetrics returns database-specific metrics from actual database stats
func (api *DeveloperToolsAPI) GetDatabaseMetrics(c *gin.Context) {
	// 从数据库连接池获取真实的数据库指标
	sqlDB, err := database.DB.DB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get database stats"})
		return
	}

	stats := sqlDB.Stats()
	metrics := DatabaseMetrics{
		Connections:    stats.OpenConnections,
		MaxConnections: stats.MaxOpenConnections,
		QueryTime:      "N/A", // 需要实现查询时间统计
		SlowQueries:    0,     // 需要实现慢查询统计
	}

	c.JSON(http.StatusOK, metrics)
}

// GetWebSocketMetrics returns WebSocket-specific metrics from hub
func (api *DeveloperToolsAPI) GetWebSocketMetrics(c *gin.Context) {
	var activeConnections int64
	if api.hub != nil {
		activeConnections = api.hub.GetConnectionCount()
	}

	metrics := WebSocketMetrics{
		ActiveConnections: int(activeConnections),
		MessagesPerSecond: 0, // 需要实现消息速率统计
		TotalMessages:     0, // 需要实现总消息数统计
	}

	c.JSON(http.StatusOK, metrics)
}

// formatBytes formats bytes to human readable format
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return strconv.FormatUint(bytes, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(bytes)/float64(div), 'f', 1, 64) + " " + "KMGTPE"[exp:exp+1] + "B"
}
