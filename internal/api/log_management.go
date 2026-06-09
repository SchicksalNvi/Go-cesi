package api

import (
	"net/http"
	"time"

	"superview/internal/logger"
	"superview/internal/validation"

	"github.com/gin-gonic/gin"
)

type LogManagementAPI struct{}

type SetLogLevelRequest struct {
	Level     string `json:"level" binding:"required"`
	Reason    string `json:"reason,omitempty"`
	ChangedBy string `json:"changed_by,omitempty"`
}

type SetTemporaryLogLevelRequest struct {
	Level     string `json:"level" binding:"required"`
	Duration  int    `json:"duration" binding:"required"` // 持续时间（秒）
	Reason    string `json:"reason,omitempty"`
	ChangedBy string `json:"changed_by,omitempty"`
}

func NewLogManagementAPI() *LogManagementAPI {
	return &LogManagementAPI{}
}

func logLevelChangedBy(c *gin.Context) string {
	userID, exists := getUserIDString(c)
	if !exists || userID == "" {
		return "unknown"
	}
	return "user_" + userID
}

// GetLogLevel 获取当前日志级别
func (api *LogManagementAPI) GetLogLevel(c *gin.Context) {
	levelInfo := logger.GetLogLevelInfo()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    levelInfo,
	})
}

// SetLogLevel 设置日志级别
func (api *LogManagementAPI) SetLogLevel(c *gin.Context) {
	var req SetLogLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	// 验证日志级别
	validator := validation.NewValidator()
	validator.ValidateLogLevel("level", req.Level)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid log level",
			"details": validator.Errors(),
		})
		return
	}

	// 获取用户信息
	changedBy := logLevelChangedBy(c)
	if req.ChangedBy != "" {
		changedBy = req.ChangedBy
	}

	// 设置日志级别
	if err := logger.SetLogLevel(req.Level, changedBy, req.Reason); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Failed to set log level",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Log level updated successfully",
		"data": gin.H{
			"new_level":  req.Level,
			"changed_by": changedBy,
			"reason":     req.Reason,
		},
	})
}

// SetTemporaryLogLevel 设置临时日志级别
func (api *LogManagementAPI) SetTemporaryLogLevel(c *gin.Context) {
	var req SetTemporaryLogLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	// 验证日志级别和超时时间
	validator := validation.NewValidator()
	validator.ValidateLogLevel("level", req.Level)
	validator.ValidateTimeout("duration", req.Duration)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid parameters",
			"details": validator.Errors(),
		})
		return
	}

	// 获取用户信息
	changedBy := logLevelChangedBy(c)
	if req.ChangedBy != "" {
		changedBy = req.ChangedBy
	}

	duration := time.Duration(req.Duration) * time.Second

	// 设置临时日志级别
	if err := logger.SetTemporaryLogLevel(req.Level, duration, changedBy, req.Reason); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Failed to set temporary log level",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Temporary log level set successfully",
		"data": gin.H{
			"new_level":  req.Level,
			"duration":   req.Duration,
			"changed_by": changedBy,
			"reason":     req.Reason,
			"expires_at": time.Now().Add(duration),
		},
	})
}

// ResetLogLevel 重置日志级别到默认值
func (api *LogManagementAPI) ResetLogLevel(c *gin.Context) {
	// 获取用户信息
	changedBy := logLevelChangedBy(c)

	// 重置日志级别
	if err := logger.ResetLogLevel(changedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to reset log level",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Log level reset to default (info)",
		"data": gin.H{
			"new_level":  "info",
			"changed_by": changedBy,
		},
	})
}

// GetAvailableLogLevels 获取可用的日志级别
func (api *LogManagementAPI) GetAvailableLogLevels(c *gin.Context) {
	levels := logger.GetAvailableLogLevels()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    levels,
	})
}

// ClearLogLevelHistory 清空日志级别变更历史
func (api *LogManagementAPI) ClearLogLevelHistory(c *gin.Context) {
	// 获取用户信息
	changedBy := logLevelChangedBy(c)

	logger.ClearLevelHistory(changedBy)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Log level history cleared",
		"data": gin.H{
			"cleared_by": changedBy,
		},
	})
}
