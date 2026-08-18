package api

import (
	"errors"
	"net/http"
	"strconv"

	"superview/internal/services"
	"superview/internal/validation"

	"github.com/gin-gonic/gin"
)

type ActivityLogsAPI struct {
	service *services.ActivityLogService
}

func NewActivityLogsAPI(service *services.ActivityLogService) *ActivityLogsAPI {
	return &ActivityLogsAPI{service: service}
}

// GetActivityLogs 获取活动日志列表
func (a *ActivityLogsAPI) GetActivityLogs(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	// 验证分页参数
	validator := validation.NewValidator()
	page, pageSize := validator.ValidatePagination(pageStr, pageSizeStr)

	// 检查验证错误
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Validation failed",
			"errors":  validator.Errors(),
		})
		return
	}

	// 获取过滤参数
	filters := map[string]interface{}{
		"level":      c.Query("level"),
		"action":     c.Query("action"),
		"resource":   c.Query("resource"),
		"username":   c.Query("username"),
		"search":     c.Query("search"),
		"start_time": c.Query("start_time"),
		"end_time":   c.Query("end_time"),
	}

	logs, total, err := a.service.GetActivityLogs(page, pageSize, filters)
	if err != nil {
		if errors.Is(err, services.ErrInvalidActivityLogTimeFilter) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get activity logs: " + err.Error(),
		})
		return
	}

	// 计算分页信息
	totalPages := (int(total) + pageSize - 1) / pageSize
	hasNext := page < totalPages
	hasPrev := page > 1

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"logs": logs,
			"pagination": gin.H{
				"page":        page,
				"page_size":   pageSize,
				"total":       total,
				"total_pages": totalPages,
				"has_next":    hasNext,
				"has_prev":    hasPrev,
			},
		},
	})
}

// GetRecentLogs 获取最近的日志
func (a *ActivityLogsAPI) GetRecentLogs(c *gin.Context) {
	validator := validation.NewValidator()

	// 验证限制参数
	limitStr := c.DefaultQuery("limit", "20")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		validator.AddError("limit", "must be a valid number")
	} else {
		validator.ValidateRange("limit", limit, 1, 100)
	}

	// 检查验证错误
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Validation failed",
			"errors":  validator.Errors(),
		})
		return
	}

	logs, err := a.service.GetRecentLogs(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get recent logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"logs":   logs,
	})
}

// GetLogStatistics 获取日志统计信息
func (a *ActivityLogsAPI) GetLogStatistics(c *gin.Context) {
	validator := validation.NewValidator()

	// 验证天数参数
	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		validator.AddError("days", "must be a valid number")
	} else {
		validator.ValidateRange("days", days, 1, 365)
	}

	// 检查验证错误
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Validation failed",
			"errors":  validator.Errors(),
		})
		return
	}

	stats, err := a.service.GetLogStatistics(days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get log statistics: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// CleanOldLogs 清理旧日志（管理员功能）
func (a *ActivityLogsAPI) CleanOldLogs(c *gin.Context) {
	if _, ok := getCurrentUser(c); !ok {
		return
	}

	validator := validation.NewValidator()
	daysStr := c.DefaultQuery("days", "90")
	days, err := strconv.Atoi(daysStr)
	if err != nil {
		validator.AddError("days", "must be a valid number")
	} else {
		validator.ValidateRange("days", days, 1, 3650)
	}

	// 验证保留天数参数
	retentionDaysStr := c.Query("retention_days")
	if retentionDaysStr != "" {
		retentionDays, err := strconv.Atoi(retentionDaysStr)
		if err != nil {
			validator.AddError("retention_days", "must be a valid number")
		} else {
			validator.ValidateRetentionDays("retention_days", retentionDays)
		}
	}

	// 检查验证错误
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Validation failed",
			"errors":  validator.Errors(),
		})
		return
	}

	err = a.service.CleanOldLogs(days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to clean old logs: " + err.Error(),
		})
		return
	}

	// 记录清理操作
	a.service.LogWithContext(c, "INFO", "clean_logs", "system", "",
		"Cleaned logs older than "+strconv.Itoa(days)+" days",
		map[string]int{"days": days})

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Old logs cleaned successfully",
	})
}

// DeleteLogs 按条件删除日志（管理员功能）
func (a *ActivityLogsAPI) DeleteLogs(c *gin.Context) {
	if _, ok := getCurrentUser(c); !ok {
		return
	}

	// 获取过滤参数
	filters := map[string]interface{}{
		"level":      c.Query("level"),
		"action":     c.Query("action"),
		"resource":   c.Query("resource"),
		"username":   c.Query("username"),
		"search":     c.Query("search"),
		"start_time": c.Query("start_time"),
		"end_time":   c.Query("end_time"),
	}

	// 检查是否有任何过滤条件
	hasFilter := false
	for _, v := range filters {
		if v != nil && v != "" {
			hasFilter = true
			break
		}
	}

	// 执行删除
	deleted, err := a.service.DeleteLogs(filters)
	if err != nil {
		if errors.Is(err, services.ErrInvalidActivityLogTimeFilter) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to delete logs: " + err.Error(),
		})
		return
	}

	// 记录删除操作
	message := "Deleted all logs"
	if hasFilter {
		message = "Deleted logs matching filters"
	}
	a.service.LogWithContext(c, "INFO", "delete_logs", "system", "",
		message+" ("+strconv.FormatInt(deleted, 10)+" records)",
		filters)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": message,
		"deleted": deleted,
	})
}

// ExportLogs 导出日志为 CSV
func (a *ActivityLogsAPI) ExportLogs(c *gin.Context) {
	// 获取过滤参数
	filters := map[string]interface{}{
		"level":      c.Query("level"),
		"action":     c.Query("action"),
		"resource":   c.Query("resource"),
		"username":   c.Query("username"),
		"search":     c.Query("search"),
		"start_time": c.Query("start_time"),
		"end_time":   c.Query("end_time"),
	}

	csvData, err := a.service.ExportLogs(filters)
	if err != nil {
		if errors.Is(err, services.ErrInvalidActivityLogTimeFilter) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to export logs: " + err.Error(),
		})
		return
	}

	// 设置响应头
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=activity-logs.csv")
	c.Data(http.StatusOK, "text/csv", csvData)
}
