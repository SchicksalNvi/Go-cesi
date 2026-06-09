package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"superview/internal/logger"
	"superview/internal/models"
	"superview/internal/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type LogsAPI struct {
	logger *services.ActivityLogService
	db     *gorm.DB
}

func NewLogsAPI(logger *services.ActivityLogService, db *gorm.DB) *LogsAPI {
	return &LogsAPI{logger: logger, db: db}
}

func (a *LogsAPI) GetLogs(c *gin.Context) {
	// Check authentication
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "Authentication required",
		})
		return
	}

	// Get user from database
	var user models.User
	if err := a.db.Preload("Roles.Permissions").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "User not found",
		})
		return
	}

	if !user.IsSuperAdmin() && !user.HasPermission(models.PermissionLogRead) {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": "Log read permission required",
		})
		return
	}

	// Get query parameters
	level := c.Query("level")
	search := c.Query("search")
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 100
	}

	// Open log file
	file, err := os.Open("logs/activity.log")
	if err != nil {
		logger.Error("Failed to open log file", zap.String("username", user.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to open log file",
		})
		return
	}
	defer file.Close()

	// Read logs line by line
	var logs []map[string]interface{}
	scanner := bufio.NewScanner(file)
	count := 0

	for scanner.Scan() {
		if limitStr != "all" && count >= limit {
			break
		}

		line := scanner.Text()
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			continue
		}

		// Apply filters
		if level != "" && logEntry["level"] != level {
			continue
		}

		if search != "" {
			match := false
			for _, v := range logEntry {
				if strVal, ok := v.(string); ok {
					if strings.Contains(strings.ToLower(strVal), strings.ToLower(search)) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}

		logs = append(logs, logEntry)
		count++
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Failed to read log file", zap.String("username", user.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to read log file",
		})
		return
	}

	logger.Debug("Logs accessed", zap.String("username", user.Username), zap.Int("count", len(logs)))

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"logs":   logs,
		"count":  len(logs),
	})
}
