package services

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"superview/internal/models"
)

type ActivityLogService struct {
	db *gorm.DB
}

func contextUserID(c *gin.Context) string {
	for _, key := range []string{"user_id", "userID"} {
		if uid, exists := c.Get(key); exists {
			if uidStr, ok := uid.(string); ok && uidStr != "" {
				return uidStr
			}
		}
	}
	return ""
}

func NewActivityLogService(db *gorm.DB) *ActivityLogService {
	return &ActivityLogService{db: db}
}

// LogActivity 记录活动日志
func (s *ActivityLogService) LogActivity(log *models.ActivityLog) error {
	return s.db.Create(log).Error
}

// LogWithContext 从Gin上下文记录日志
func (s *ActivityLogService) LogWithContext(c *gin.Context, level, action, resource, target, message string, extraInfo interface{}) {
	var userID string
	var username string

	// 从上下文获取用户信息
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(*models.User); ok {
			userID = u.ID
			username = u.Username
		}
	}

	// fallback: 如果 user 对象不存在，尝试用 user_id 查库
	if username == "" {
		if uidStr := contextUserID(c); uidStr != "" {
			userID = uidStr
			var user models.User
			if s.db.Where("id = ?", uidStr).First(&user).Error == nil {
				username = user.Username
			}
		}
	}

	log := &models.ActivityLog{
		Level:     level,
		Message:   message,
		Action:    action,
		Resource:  resource,
		Target:    target,
		UserID:    userID,
		Username:  username,
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		CreatedAt: time.Now(),
	}

	s.db.Create(log)
}

// LogError 记录错误日志
func (s *ActivityLogService) LogError(c *gin.Context, action, resource, target string, err error, extraInfo interface{}) {
	var userID string
	var username string

	// 从上下文获取用户信息
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(*models.User); ok {
			userID = u.ID
			username = u.Username
		}
	}

	// fallback: 如果 user 对象不存在，尝试用 user_id 查库
	if username == "" {
		if uidStr := contextUserID(c); uidStr != "" {
			userID = uidStr
			var user models.User
			if s.db.Where("id = ?", uidStr).First(&user).Error == nil {
				username = user.Username
			}
		}
	}

	log := &models.ActivityLog{
		Level:     "ERROR",
		Message:   err.Error(),
		Action:    action,
		Resource:  resource,
		Target:    target,
		UserID:    userID,
		Username:  username,
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		CreatedAt: time.Now(),
	}

	s.db.Create(log)
}

// GetActivityLogs 获取活动日志列表
func (s *ActivityLogService) GetActivityLogs(page, pageSize int, filters map[string]interface{}) ([]*models.ActivityLog, int64, error) {
	var logs []*models.ActivityLog
	var total int64

	query := s.db.Model(&models.ActivityLog{})

	// 应用过滤器
	for key, value := range filters {
		if value != nil && value != "" {
			switch key {
			case "level":
				query = query.Where("level = ?", value)
			case "action":
				query = query.Where("action = ?", value)
			case "resource":
				if str, ok := value.(string); ok {
					query = query.Where("resource LIKE ?", "%"+str+"%")
				}
			case "username":
				if str, ok := value.(string); ok {
					query = query.Where("username LIKE ?", "%"+str+"%")
				}
			case "start_time":
				if str, ok := value.(string); ok && str != "" {
					query = query.Where("created_at >= ?", str)
				}
			case "end_time":
				if str, ok := value.(string); ok && str != "" {
					query = query.Where("created_at <= ?", str)
				}
			}
		}
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// GetRecentLogs 获取最近的日志
func (s *ActivityLogService) GetRecentLogs(limit int) ([]models.ActivityLog, error) {
	var logs []models.ActivityLog
	err := s.db.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// GetLogStatistics 获取日志统计信息
func (s *ActivityLogService) GetLogStatistics(days int) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 计算时间范围
	startTime := time.Now().AddDate(0, 0, -days)

	// 总日志数
	var totalLogs int64
	if err := s.db.Model(&models.ActivityLog{}).Where("created_at >= ?", startTime).Count(&totalLogs).Error; err != nil {
		return nil, err
	}
	stats["total_logs"] = totalLogs

	// 按级别统计
	var levelStats []struct {
		Level string `json:"level"`
		Count int64  `json:"count"`
	}
	if err := s.db.Model(&models.ActivityLog{}).
		Select("level, COUNT(*) as count").
		Where("created_at >= ?", startTime).
		Group("level").
		Scan(&levelStats).Error; err != nil {
		return nil, err
	}

	// 初始化级别计数
	stats["info_count"] = int64(0)
	stats["warning_count"] = int64(0)
	stats["error_count"] = int64(0)
	stats["debug_count"] = int64(0)

	// 填充级别统计
	for _, stat := range levelStats {
		switch stat.Level {
		case "INFO":
			stats["info_count"] = stat.Count
		case "WARNING":
			stats["warning_count"] = stat.Count
		case "ERROR":
			stats["error_count"] = stat.Count
		case "DEBUG":
			stats["debug_count"] = stat.Count
		}
	}

	// 按操作统计
	var actionStats []struct {
		Action string `json:"action"`
		Count  int64  `json:"count"`
	}
	if err := s.db.Model(&models.ActivityLog{}).
		Select("action, COUNT(*) as count").
		Where("created_at >= ?", startTime).
		Group("action").
		Order("count DESC").
		Limit(10).
		Scan(&actionStats).Error; err != nil {
		return nil, err
	}
	stats["top_actions"] = actionStats

	// 按用户统计
	var userStats []struct {
		Username string `json:"username"`
		Count    int64  `json:"count"`
	}
	if err := s.db.Model(&models.ActivityLog{}).
		Select("username, COUNT(*) as count").
		Where("created_at >= ? AND username != '' AND username IS NOT NULL", startTime).
		Group("username").
		Order("count DESC").
		Limit(10).
		Scan(&userStats).Error; err != nil {
		return nil, err
	}
	stats["top_users"] = userStats

	return stats, nil
}

// CleanOldLogs 清理旧日志
func (s *ActivityLogService) CleanOldLogs(days int) error {
	cutoffTime := time.Now().AddDate(0, 0, -days)
	return s.db.Where("created_at < ?", cutoffTime).Delete(&models.ActivityLog{}).Error
}

// DeleteLogs 按条件删除日志
func (s *ActivityLogService) DeleteLogs(filters map[string]interface{}) (int64, error) {
	query := s.db.Model(&models.ActivityLog{})
	hasFilter := false

	// 应用过滤器
	for key, value := range filters {
		if value != nil && value != "" {
			hasFilter = true
			switch key {
			case "level":
				query = query.Where("level = ?", value)
			case "action":
				query = query.Where("action = ?", value)
			case "resource":
				if str, ok := value.(string); ok {
					query = query.Where("resource LIKE ?", "%"+str+"%")
				}
			case "username":
				if str, ok := value.(string); ok {
					query = query.Where("username LIKE ?", "%"+str+"%")
				}
			case "start_time":
				if str, ok := value.(string); ok && str != "" {
					query = query.Where("created_at >= ?", str)
				}
			case "end_time":
				if str, ok := value.(string); ok && str != "" {
					query = query.Where("created_at <= ?", str)
				}
			}
		}
	}

	// 执行删除
	result := query.Delete(&models.ActivityLog{})
	if result.Error != nil {
		return 0, result.Error
	}

	// 如果没有任何过滤条件，返回特殊标记
	if !hasFilter {
		return result.RowsAffected, nil
	}

	return result.RowsAffected, nil
}

// 便捷方法
func (s *ActivityLogService) LogLogin(c *gin.Context, username string) {
	message := fmt.Sprintf("User %s logged in", username)
	s.LogWithContext(c, "INFO", "login", "auth", username, message, nil)
}

func (s *ActivityLogService) LogLogout(c *gin.Context, username string) {
	message := fmt.Sprintf("User %s logged out", username)
	s.LogWithContext(c, "INFO", "logout", "auth", username, message, nil)
}

func (s *ActivityLogService) LogProcessAction(c *gin.Context, action, nodeName, processName string) {
	message := fmt.Sprintf("Process %s %s on node %s", processName, action, nodeName)
	target := fmt.Sprintf("%s:%s", nodeName, processName)
	s.LogWithContext(c, "INFO", action, "process", target, message, map[string]string{
		"node":    nodeName,
		"process": processName,
	})
}

func (s *ActivityLogService) LogGroupAction(c *gin.Context, action, groupName, environment string) {
	message := fmt.Sprintf("Group %s %s", groupName, action)
	if environment != "" {
		message += fmt.Sprintf(" in environment %s", environment)
	}
	s.LogWithContext(c, "INFO", action, "group", groupName, message, map[string]string{
		"group":       groupName,
		"environment": environment,
	})
}

func (s *ActivityLogService) LogUserAction(c *gin.Context, action, targetUsername string) {
	message := fmt.Sprintf("User %s %s", targetUsername, action)
	s.LogWithContext(c, "INFO", action, "user", targetUsername, message, nil)
}

// ExportLogs 导出日志为 CSV 格式
func (s *ActivityLogService) ExportLogs(filters map[string]interface{}) ([]byte, error) {
	var logs []*models.ActivityLog

	query := s.db.Model(&models.ActivityLog{})

	// 应用过滤器
	for key, value := range filters {
		if value != nil && value != "" {
			switch key {
			case "level":
				query = query.Where("level = ?", value)
			case "action":
				query = query.Where("action = ?", value)
			case "resource":
				if str, ok := value.(string); ok {
					query = query.Where("resource LIKE ?", "%"+str+"%")
				}
			case "username":
				if str, ok := value.(string); ok {
					query = query.Where("username LIKE ?", "%"+str+"%")
				}
			case "start_time":
				if str, ok := value.(string); ok && str != "" {
					query = query.Where("created_at >= ?", str)
				}
			case "end_time":
				if str, ok := value.(string); ok && str != "" {
					query = query.Where("created_at <= ?", str)
				}
			}
		}
	}

	// 查询所有符合条件的日志
	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, err
	}

	// 生成 CSV 内容
	csv := "ID,Created At,Level,Username,Action,Resource,Target,Message,IP Address,Status,Duration\n"

	for _, log := range logs {
		username := log.Username
		if username == "" {
			username = "system"
		}

		csv += fmt.Sprintf("%d,%s,%s,%s,%s,%s,%s,\"%s\",%s,%s,%d\n",
			log.ID,
			log.CreatedAt.Format("2006-01-02 15:04:05"),
			log.Level,
			username,
			log.Action,
			log.Resource,
			log.Target,
			log.Message,
			log.IPAddress,
			log.Status,
			log.Duration,
		)
	}

	return []byte(csv), nil
}

// LogSystemEvent 记录系统事件（无用户操作）
func (s *ActivityLogService) LogSystemEvent(level, action, resource, target, message string, extraInfo interface{}) error {
	log := &models.ActivityLog{
		Level:     level,
		Message:   message,
		Action:    action,
		Resource:  resource,
		Target:    target,
		UserID:    "",
		Username:  "system",
		IPAddress: "",
		UserAgent: "",
		CreatedAt: time.Now(),
		Status:    models.StatusSuccess,
	}

	return s.db.Create(log).Error
}
