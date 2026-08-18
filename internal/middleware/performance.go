package middleware

import (
	"runtime"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"superview/internal/logger"
)

// PerformanceMetrics 性能指标结构
type PerformanceMetrics struct {
	mu               sync.RWMutex
	RequestCount     int64                    `json:"request_count"`
	TotalDuration    time.Duration            `json:"total_duration_ms"`
	AverageDuration  time.Duration            `json:"average_duration_ms"`
	SlowRequests     int64                    `json:"slow_requests"`
	ErrorCount       int64                    `json:"error_count"`
	EndpointMetrics  map[string]*EndpointStat `json:"endpoint_metrics"`
	StatusCodeCounts map[int]int64            `json:"status_code_counts"`
	LastResetTime    time.Time                `json:"last_reset_time"`
	// 内存监控指标
	MemoryMetrics *MemoryMetrics `json:"memory_metrics"`
}

// MemoryMetrics 内存监控指标
type MemoryMetrics struct {
	AllocBytes      uint64    `json:"alloc_bytes"`       // 当前分配的字节数
	TotalAllocBytes uint64    `json:"total_alloc_bytes"` // 累计分配的字节数
	SysBytes        uint64    `json:"sys_bytes"`         // 系统内存字节数
	NumGC           uint32    `json:"num_gc"`            // GC次数
	GCCPUFraction   float64   `json:"gc_cpu_fraction"`   // GC CPU占用比例
	HeapObjects     uint64    `json:"heap_objects"`      // 堆对象数量
	StackInUse      uint64    `json:"stack_in_use"`      // 栈使用量
	LastUpdated     time.Time `json:"last_updated"`      // 最后更新时间
}

// EndpointStat 端点统计信息
type EndpointStat struct {
	Count           int64         `json:"count"`
	TotalDuration   time.Duration `json:"total_duration_ms"`
	AverageDuration time.Duration `json:"average_duration_ms"`
	MinDuration     time.Duration `json:"min_duration_ms"`
	MaxDuration     time.Duration `json:"max_duration_ms"`
	ErrorCount      int64         `json:"error_count"`
	LastAccess      time.Time     `json:"last_access"`
}

// 全局性能指标实例
var globalMetrics = &PerformanceMetrics{
	EndpointMetrics:  make(map[string]*EndpointStat),
	StatusCodeCounts: make(map[int]int64),
	LastResetTime:    time.Now(),
	MemoryMetrics:    &MemoryMetrics{LastUpdated: time.Now()},
}

// 内存监控定时器
var memoryUpdateTicker *time.Ticker
var memoryUpdateStop chan bool

// PerformanceMiddleware 性能监控中间件
func PerformanceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		method := c.Request.Method
		endpoint := method + " " + path

		// 处理请求
		c.Next()

		// 计算执行时间
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		// 记录性能指标
		recordMetrics(endpoint, duration, statusCode)

		// 记录慢请求日志（超过1秒）
		if duration > time.Second {
			logger.Warn("Slow request detected",
				zap.String("endpoint", endpoint),
				zap.Duration("duration", duration),
				zap.Int("status_code", statusCode),
				zap.String("client_ip", c.ClientIP()),
			)
		}

		// 记录错误请求日志
		if statusCode >= 400 {
			logger.Error("Request error",
				zap.String("endpoint", endpoint),
				zap.Duration("duration", duration),
				zap.Int("status_code", statusCode),
				zap.String("client_ip", c.ClientIP()),
				zap.String("user_agent", c.GetHeader("User-Agent")),
			)
		}

		// 记录调试信息（仅在debug级别）
		logger.Debug("Request completed",
			zap.String("endpoint", endpoint),
			zap.Duration("duration", duration),
			zap.Int("status_code", statusCode),
		)
	}
}

// collectMemoryMetrics reads the runtime counters without taking the global
// metrics lock. Callers can then publish the snapshot in their own critical
// section without recursively locking globalMetrics.mu.
func collectMemoryMetrics() *MemoryMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &MemoryMetrics{
		AllocBytes:      m.Alloc,
		TotalAllocBytes: m.TotalAlloc,
		SysBytes:        m.Sys,
		NumGC:           m.NumGC,
		GCCPUFraction:   m.GCCPUFraction,
		HeapObjects:     m.HeapObjects,
		StackInUse:      m.StackInuse,
		LastUpdated:     time.Now(),
	}
}

// updateMemoryMetrics 更新内存指标
func updateMemoryMetrics() {
	memoryMetrics := collectMemoryMetrics()

	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()

	globalMetrics.MemoryMetrics = memoryMetrics
}

// StartMemoryMonitoring 启动内存监控
func StartMemoryMonitoring(interval time.Duration) {
	if memoryUpdateTicker != nil {
		return // 已经启动
	}

	memoryUpdateTicker = time.NewTicker(interval)
	memoryUpdateStop = make(chan bool)

	// 立即更新一次
	updateMemoryMetrics()

	go func() {
		for {
			select {
			case <-memoryUpdateTicker.C:
				updateMemoryMetrics()
			case <-memoryUpdateStop:
				return
			}
		}
	}()

	logger.Info("Memory monitoring started", zap.Duration("interval", interval))
}

// StopMemoryMonitoring 停止内存监控
func StopMemoryMonitoring() {
	if memoryUpdateTicker != nil {
		memoryUpdateTicker.Stop()
		memoryUpdateTicker = nil
	}
	if memoryUpdateStop != nil {
		close(memoryUpdateStop)
		memoryUpdateStop = nil
	}
	logger.Info("Memory monitoring stopped")
}

// recordMetrics 记录性能指标
func recordMetrics(endpoint string, duration time.Duration, statusCode int) {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()

	// 更新全局指标
	globalMetrics.RequestCount++
	globalMetrics.TotalDuration += duration
	globalMetrics.AverageDuration = time.Duration(int64(globalMetrics.TotalDuration) / globalMetrics.RequestCount)

	// 记录慢请求
	if duration > time.Second {
		globalMetrics.SlowRequests++
	}

	// 记录错误请求
	if statusCode >= 400 {
		globalMetrics.ErrorCount++
	}

	// 更新状态码统计
	globalMetrics.StatusCodeCounts[statusCode]++

	// 更新端点指标
	if globalMetrics.EndpointMetrics[endpoint] == nil {
		globalMetrics.EndpointMetrics[endpoint] = &EndpointStat{
			MinDuration: duration,
			MaxDuration: duration,
		}
	}

	stat := globalMetrics.EndpointMetrics[endpoint]
	stat.Count++
	stat.TotalDuration += duration
	stat.AverageDuration = time.Duration(int64(stat.TotalDuration) / stat.Count)
	stat.LastAccess = time.Now()

	// 更新最小和最大执行时间
	if duration < stat.MinDuration {
		stat.MinDuration = duration
	}
	if duration > stat.MaxDuration {
		stat.MaxDuration = duration
	}

	// 记录端点错误
	if statusCode >= 400 {
		stat.ErrorCount++
	}
}

// GetPerformanceMetrics 获取性能指标
func GetPerformanceMetrics() *PerformanceMetrics {
	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	// 创建副本以避免并发问题
	metrics := &PerformanceMetrics{
		RequestCount:     globalMetrics.RequestCount,
		TotalDuration:    globalMetrics.TotalDuration,
		AverageDuration:  globalMetrics.AverageDuration,
		SlowRequests:     globalMetrics.SlowRequests,
		ErrorCount:       globalMetrics.ErrorCount,
		EndpointMetrics:  make(map[string]*EndpointStat),
		StatusCodeCounts: make(map[int]int64),
		LastResetTime:    globalMetrics.LastResetTime,
		// 复制内存指标
		MemoryMetrics: &MemoryMetrics{
			AllocBytes:      globalMetrics.MemoryMetrics.AllocBytes,
			TotalAllocBytes: globalMetrics.MemoryMetrics.TotalAllocBytes,
			SysBytes:        globalMetrics.MemoryMetrics.SysBytes,
			NumGC:           globalMetrics.MemoryMetrics.NumGC,
			GCCPUFraction:   globalMetrics.MemoryMetrics.GCCPUFraction,
			HeapObjects:     globalMetrics.MemoryMetrics.HeapObjects,
			StackInUse:      globalMetrics.MemoryMetrics.StackInUse,
			LastUpdated:     globalMetrics.MemoryMetrics.LastUpdated,
		},
	}

	// 复制端点指标
	for k, v := range globalMetrics.EndpointMetrics {
		metrics.EndpointMetrics[k] = &EndpointStat{
			Count:           v.Count,
			TotalDuration:   v.TotalDuration,
			AverageDuration: v.AverageDuration,
			MinDuration:     v.MinDuration,
			MaxDuration:     v.MaxDuration,
			ErrorCount:      v.ErrorCount,
			LastAccess:      v.LastAccess,
		}
	}

	// 复制状态码统计
	for k, v := range globalMetrics.StatusCodeCounts {
		metrics.StatusCodeCounts[k] = v
	}

	return metrics
}

// ResetPerformanceMetrics 重置性能指标
func ResetPerformanceMetrics() {
	memoryMetrics := collectMemoryMetrics()

	globalMetrics.mu.Lock()
	globalMetrics.RequestCount = 0
	globalMetrics.TotalDuration = 0
	globalMetrics.AverageDuration = 0
	globalMetrics.SlowRequests = 0
	globalMetrics.ErrorCount = 0
	globalMetrics.EndpointMetrics = make(map[string]*EndpointStat)
	globalMetrics.StatusCodeCounts = make(map[int]int64)
	globalMetrics.LastResetTime = time.Now()
	globalMetrics.MemoryMetrics = memoryMetrics
	globalMetrics.mu.Unlock()

	logger.Info("Performance metrics reset")
}

// 定时清理相关变量
var cleanupTicker *time.Ticker
var cleanupStop chan bool

// StartPerformanceCleanup 启动性能指标定时清理
func StartPerformanceCleanup(resetInterval time.Duration, endpointCleanupThreshold time.Duration) {
	if cleanupTicker != nil {
		return // 已经启动
	}

	cleanupTicker = time.NewTicker(resetInterval)
	cleanupStop = make(chan bool)

	go func() {
		for {
			select {
			case <-cleanupTicker.C:
				// 清理长时间未访问的端点指标
				cleanupOldEndpoints(endpointCleanupThreshold)

				// 如果距离上次重置时间超过重置间隔，则重置指标
				globalMetrics.mu.RLock()
				lastReset := globalMetrics.LastResetTime
				globalMetrics.mu.RUnlock()

				if time.Since(lastReset) >= resetInterval {
					logger.Info("Auto-resetting performance metrics",
						zap.Duration("interval", resetInterval))
					ResetPerformanceMetrics()
				}
			case <-cleanupStop:
				return
			}
		}
	}()

	logger.Info("Performance cleanup started",
		zap.Duration("reset_interval", resetInterval),
		zap.Duration("endpoint_cleanup_threshold", endpointCleanupThreshold))
}

// StopPerformanceCleanup 停止性能指标定时清理
func StopPerformanceCleanup() {
	if cleanupTicker != nil {
		cleanupTicker.Stop()
		cleanupTicker = nil
	}
	if cleanupStop != nil {
		close(cleanupStop)
		cleanupStop = nil
	}
	logger.Info("Performance cleanup stopped")
}

// cleanupOldEndpoints 清理长时间未访问的端点指标
func cleanupOldEndpoints(threshold time.Duration) {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()

	now := time.Now()
	cleanedCount := 0

	for endpoint, stat := range globalMetrics.EndpointMetrics {
		if now.Sub(stat.LastAccess) > threshold {
			delete(globalMetrics.EndpointMetrics, endpoint)
			cleanedCount++
		}
	}

	if cleanedCount > 0 {
		logger.Info("Cleaned up old endpoint metrics",
			zap.Int("cleaned_count", cleanedCount),
			zap.Duration("threshold", threshold))
	}
}

// GetTopSlowEndpoints 获取最慢的端点
func GetTopSlowEndpoints(limit int) []struct {
	Endpoint string        `json:"endpoint"`
	AvgTime  time.Duration `json:"avg_duration_ms"`
	Count    int64         `json:"count"`
} {
	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	type endpointTime struct {
		Endpoint string
		AvgTime  time.Duration
		Count    int64
	}

	var endpoints []endpointTime
	for endpoint, stat := range globalMetrics.EndpointMetrics {
		endpoints = append(endpoints, endpointTime{
			Endpoint: endpoint,
			AvgTime:  stat.AverageDuration,
			Count:    stat.Count,
		})
	}

	// 按平均执行时间排序
	for i := 0; i < len(endpoints)-1; i++ {
		for j := i + 1; j < len(endpoints); j++ {
			if endpoints[i].AvgTime < endpoints[j].AvgTime {
				endpoints[i], endpoints[j] = endpoints[j], endpoints[i]
			}
		}
	}

	// 限制返回数量
	if limit > 0 && limit < len(endpoints) {
		endpoints = endpoints[:limit]
	}

	result := make([]struct {
		Endpoint string        `json:"endpoint"`
		AvgTime  time.Duration `json:"avg_duration_ms"`
		Count    int64         `json:"count"`
	}, len(endpoints))

	for i, ep := range endpoints {
		result[i] = struct {
			Endpoint string        `json:"endpoint"`
			AvgTime  time.Duration `json:"avg_duration_ms"`
			Count    int64         `json:"count"`
		}{
			Endpoint: ep.Endpoint,
			AvgTime:  ep.AvgTime,
			Count:    ep.Count,
		}
	}

	return result
}

// GetErrorRateByEndpoint 获取端点错误率
func GetErrorRateByEndpoint() map[string]float64 {
	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	errorRates := make(map[string]float64)
	for endpoint, stat := range globalMetrics.EndpointMetrics {
		if stat.Count > 0 {
			errorRates[endpoint] = float64(stat.ErrorCount) / float64(stat.Count) * 100
		}
	}

	return errorRates
}
