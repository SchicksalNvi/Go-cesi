package services

import (
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"superview/internal/logger"
	"superview/internal/models"
)

// AlertService 告警服务
type AlertService struct {
	db *gorm.DB
}

// NewAlertService 创建告警服务实例
func NewAlertService(db *gorm.DB) *AlertService {
	return &AlertService{db: db}
}

// CreateAlertRule 创建告警规则
func (s *AlertService) CreateAlertRule(rule *models.AlertRule) error {
	return createAndPreserveBool(s.db, rule, "enabled", rule.Enabled)
}

// GetAlertRules 获取告警规则列表
func (s *AlertService) GetAlertRules(page, pageSize int, filters map[string]interface{}) ([]models.AlertRule, int64, error) {
	var rules []models.AlertRule
	var total int64

	query := s.db.Model(&models.AlertRule{}).Preload("User")

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "enabled":
			query = query.Where("enabled = ?", value)
		case "severity":
			query = query.Where("severity = ?", value)
		case "metric":
			query = query.Where("metric = ?", value)
		case "node_id":
			query = query.Where("node_id = ?", value)
		case "search":
			query = query.Where("name LIKE ? OR description LIKE ?", "%"+value.(string)+"%", "%"+value.(string)+"%")
		}
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&rules).Error

	return rules, total, err
}

// GetAlertRuleByID 根据ID获取告警规则
func (s *AlertService) GetAlertRuleByID(id uint) (*models.AlertRule, error) {
	var rule models.AlertRule
	err := s.db.Preload("User").Preload("Alerts").First(&rule, id).Error
	return &rule, err
}

// UpdateAlertRule 更新告警规则
func (s *AlertService) UpdateAlertRule(id uint, updates map[string]interface{}) error {
	return s.db.Model(&models.AlertRule{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteAlertRule 删除告警规则（级联删除相关告警）
func (s *AlertService) DeleteAlertRule(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 先删除相关的告警记录
		if err := tx.Where("rule_id = ?", id).Delete(&models.Alert{}).Error; err != nil {
			return err
		}
		// 再删除规则
		return tx.Delete(&models.AlertRule{}, id).Error
	})
}

// CreateAlert 创建告警
func (s *AlertService) CreateAlert(alert *models.Alert) error {
	return s.db.Create(alert).Error
}

// GetAlerts 获取告警列表
func (s *AlertService) GetAlerts(page, pageSize int, filters map[string]interface{}) ([]models.Alert, int64, error) {
	var alerts []models.Alert
	var total int64

	query := s.db.Model(&models.Alert{}).
		Preload("Rule").
		Preload("AckedByUser").
		Preload("ResolvedByUser").
		Joins("JOIN alert_rules ON alerts.rule_id = alert_rules.id AND alert_rules.deleted_at IS NULL")

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "status":
			query = query.Where("status = ?", value)
		case "severity":
			query = query.Where("severity = ?", value)
		case "rule_id":
			query = query.Where("rule_id = ?", value)
		case "node_id":
			query = query.Where("node_id = ?", value)
		case "start_time_from":
			query = query.Where("start_time >= ?", value)
		case "start_time_to":
			query = query.Where("start_time <= ?", value)
		case "search":
			query = query.Where("message LIKE ?", "%"+value.(string)+"%")
		}
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("start_time DESC").Find(&alerts).Error

	return alerts, total, err
}

// GetAlertByID 根据ID获取告警
func (s *AlertService) GetAlertByID(id uint) (*models.Alert, error) {
	var alert models.Alert
	err := s.db.Preload("Rule").Preload("AckedByUser").Preload("ResolvedByUser").Preload("Notifications").First(&alert, id).Error
	return &alert, err
}

// AcknowledgeAlert 确认告警
func (s *AlertService) AcknowledgeAlert(id uint, userIDStr string) error {
	now := time.Now()
	return s.db.Model(&models.Alert{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     models.AlertStatusAcknowledged,
			"acked_by":   userIDStr,
			"acked_at":   now,
			"updated_at": now,
		}).Error
}

// ResolveAlert 解决告警
func (s *AlertService) ResolveAlert(id uint, userIDStr string) error {
	now := time.Now()
	return s.db.Model(&models.Alert{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      models.AlertStatusResolved,
			"resolved_by": userIDStr,
			"resolved_at": now,
			"end_time":    now,
			"updated_at":  now,
		}).Error
}

// CreateNotificationChannel 创建通知渠道
func (s *AlertService) CreateNotificationChannel(channel *models.NotificationChannel) error {
	return createAndPreserveBool(s.db, channel, "enabled", channel.Enabled)
}

// GetNotificationChannels 获取通知渠道列表
func (s *AlertService) GetNotificationChannels(page, pageSize int, filters map[string]interface{}) ([]models.NotificationChannel, int64, error) {
	var channels []models.NotificationChannel
	var total int64

	query := s.db.Model(&models.NotificationChannel{}).Preload("User")

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "enabled":
			query = query.Where("enabled = ?", value)
		case "type":
			query = query.Where("type = ?", value)
		case "search":
			query = query.Where("name LIKE ? OR description LIKE ?", "%"+value.(string)+"%", "%"+value.(string)+"%")
		}
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&channels).Error

	return channels, total, err
}

// GetNotificationChannelByID 根据ID获取通知渠道
func (s *AlertService) GetNotificationChannelByID(id uint) (*models.NotificationChannel, error) {
	var channel models.NotificationChannel
	err := s.db.Preload("User").First(&channel, id).Error
	return &channel, err
}

// UpdateNotificationChannel 更新通知渠道
func (s *AlertService) UpdateNotificationChannel(id uint, updates map[string]interface{}) error {
	return s.db.Model(&models.NotificationChannel{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteNotificationChannel 删除通知渠道
func (s *AlertService) DeleteNotificationChannel(id uint) error {
	return s.db.Delete(&models.NotificationChannel{}, id).Error
}

// AssignChannelToRule 为告警规则分配通知渠道
func (s *AlertService) AssignChannelToRule(ruleID, channelID uint) error {
	assignment := &models.AlertRuleNotificationChannel{
		RuleID:    ruleID,
		ChannelID: channelID,
	}
	return s.db.Create(assignment).Error
}

// RemoveChannelFromRule 从告警规则移除通知渠道
func (s *AlertService) RemoveChannelFromRule(ruleID, channelID uint) error {
	return s.db.Where("rule_id = ? AND channel_id = ?", ruleID, channelID).Delete(&models.AlertRuleNotificationChannel{}).Error
}

// GetRuleChannels 获取告警规则的通知渠道
func (s *AlertService) GetRuleChannels(ruleID uint) ([]models.NotificationChannel, error) {
	var channels []models.NotificationChannel
	err := s.db.Table("notification_channels").
		Joins("JOIN alert_rule_notification_channels ON notification_channels.id = alert_rule_notification_channels.channel_id").
		Where("alert_rule_notification_channels.rule_id = ? AND notification_channels.enabled = ?", ruleID, true).
		Find(&channels).Error
	return channels, err
}

// RecordSystemMetric 记录系统指标
func (s *AlertService) RecordSystemMetric(metric *models.SystemMetric) error {
	return s.db.Create(metric).Error
}

// GetSystemMetrics 获取系统指标
func (s *AlertService) GetSystemMetrics(filters map[string]interface{}, limit int) ([]models.SystemMetric, error) {
	var metrics []models.SystemMetric

	query := s.db.Model(&models.SystemMetric{})

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "node_id":
			query = query.Where("node_id = ?", value)
		case "process_name":
			query = query.Where("process_name = ?", value)
		case "metric_type":
			query = query.Where("metric_type = ?", value)
		case "metric_name":
			query = query.Where("metric_name = ?", value)
		case "timestamp_from":
			query = query.Where("timestamp >= ?", value)
		case "timestamp_to":
			query = query.Where("timestamp <= ?", value)
		}
	}

	err := query.Order("timestamp DESC").Limit(limit).Find(&metrics).Error
	return metrics, err
}

// CheckAlertRules 检查告警规则并触发告警
func (s *AlertService) CheckAlertRules() error {
	// 获取所有启用的告警规则
	var rules []models.AlertRule
	err := s.db.Where("enabled = ?", true).Find(&rules).Error
	if err != nil {
		return err
	}

	for _, rule := range rules {
		err := s.checkSingleRule(&rule)
		if err != nil {
			logger.Error("Error checking rule", zap.Uint("rule_id", rule.ID), zap.Error(err))
		}
	}

	return nil
}

// checkSingleRule 检查单个告警规则
func (s *AlertService) checkSingleRule(rule *models.AlertRule) error {
	// 获取最新的指标数据
	filters := map[string]interface{}{
		"metric_name":    rule.Metric,
		"timestamp_from": time.Now().Add(-time.Duration(rule.Duration) * time.Second),
	}

	if rule.NodeID != nil {
		filters["node_id"] = *rule.NodeID
	}

	if rule.ProcessName != nil {
		filters["process_name"] = *rule.ProcessName
	}

	metrics, err := s.GetSystemMetrics(filters, 1)
	if err != nil {
		return err
	}

	if len(metrics) == 0 {
		return nil // 没有数据，跳过检查
	}

	latestMetric := metrics[0]

	// 检查是否应该触发告警
	if rule.ShouldTrigger(latestMetric.Value) {
		// 检查是否已经有活跃的告警
		var existingAlert models.Alert
		err := s.db.Where("rule_id = ? AND status = ?", rule.ID, models.AlertStatusActive).First(&existingAlert).Error
		if err == gorm.ErrRecordNotFound {
			// 创建新告警
			alert := &models.Alert{
				RuleID:      rule.ID,
				ProcessName: rule.ProcessName,
				Message:     s.generateAlertMessage(rule, latestMetric.Value),
				Severity:    rule.Severity,
				Status:      models.AlertStatusActive,
				Value:       latestMetric.Value,
				StartTime:   time.Now(),
			}

			err = s.CreateAlert(alert)
			if err != nil {
				return err
			}

			// 发送通知
			return s.sendAlertNotifications(alert)
		}
	} else {
		// 检查是否有活跃的告警需要自动解决
		var activeAlert models.Alert
		err := s.db.Where("rule_id = ? AND status = ?", rule.ID, models.AlertStatusActive).First(&activeAlert).Error
		if err == nil {
			// 自动解决告警
			activeAlert.Status = models.AlertStatusResolved
			now := time.Now()
			activeAlert.EndTime = &now
			activeAlert.ResolvedAt = &now
			return s.db.Save(&activeAlert).Error
		}
	}

	return nil
}

// generateAlertMessage 生成告警消息
func (s *AlertService) generateAlertMessage(rule *models.AlertRule, value float64) string {
	return fmt.Sprintf("告警: %s - %s 当前值: %.2f, 阈值: %s %.2f",
		rule.Name, rule.Description, value, rule.Condition, rule.Threshold)
}

// sendAlertNotifications 发送告警通知
func (s *AlertService) sendAlertNotifications(alert *models.Alert) error {
	// 获取告警规则的通知渠道
	channels, err := s.GetRuleChannels(alert.RuleID)
	if err != nil {
		return err
	}

	for _, channel := range channels {
		notification := &models.Notification{
			AlertID:   alert.ID,
			ChannelID: channel.ID,
			Status:    models.NotificationStatusPending,
			Message:   s.formatNotificationMessage(alert, &channel),
		}

		err = s.db.Create(notification).Error
		if err != nil {
			logger.Error("Error creating notification", zap.Error(err))
			continue
		}

		// 异步发送通知
		go s.sendNotification(notification, &channel)
	}

	return nil
}

// formatNotificationMessage 格式化通知消息
func (s *AlertService) formatNotificationMessage(alert *models.Alert, channel *models.NotificationChannel) string {
	switch channel.Type {
	case models.ChannelTypeEmail:
		return fmt.Sprintf("告警通知\n\n%s\n\n严重级别: %s\n开始时间: %s",
			alert.Message, alert.Severity, alert.StartTime.Format("2006-01-02 15:04:05"))
	case models.ChannelTypeSlack:
		return fmt.Sprintf(":warning: *告警通知*\n%s\n*严重级别:* %s\n*开始时间:* %s",
			alert.Message, alert.Severity, alert.StartTime.Format("2006-01-02 15:04:05"))
	default:
		return alert.Message
	}
}

// sendNotification 发送通知
func (s *AlertService) sendNotification(notification *models.Notification, channel *models.NotificationChannel) {
	var err error

	switch channel.Type {
	case models.ChannelTypeEmail:
		err = s.sendEmailNotification(notification, channel)
	case models.ChannelTypeSlack:
		err = s.sendSlackNotification(notification, channel)
	case models.ChannelTypeWebhook:
		err = s.sendWebhookNotification(notification, channel)
	case models.ChannelTypeDingTalk:
		err = s.sendDingTalkNotification(notification, channel)
	default:
		err = fmt.Errorf("unsupported notification channel type: %s", channel.Type)
	}

	if err != nil {
		notification.MarkAsFailed(err)
		logger.Error("Failed to send notification", zap.Uint("notification_id", notification.ID), zap.Error(err))
	} else {
		notification.MarkAsSent()
	}

	// 更新通知状态
	s.db.Save(notification)
}

// sendEmailNotification 发送邮件通知
func (s *AlertService) sendEmailNotification(notification *models.Notification, channel *models.NotificationChannel) error {
	// 解析邮件配置
	var config map[string]interface{}
	err := json.Unmarshal([]byte(channel.Config), &config)
	if err != nil {
		return err
	}

	// TODO: 实现实际的邮件发送逻辑
	// 占位实现:仅记录日志并返回错误,告知调用方该功能尚未实现
	logger.Info("Email notification placeholder (not implemented)", zap.String("message", notification.Message))
	return fmt.Errorf("email notification is not implemented yet")
}

// sendSlackNotification 发送Slack通知
func (s *AlertService) sendSlackNotification(notification *models.Notification, channel *models.NotificationChannel) error {
	// 解析Slack配置
	var config map[string]interface{}
	err := json.Unmarshal([]byte(channel.Config), &config)
	if err != nil {
		return err
	}

	// TODO: 实现实际的Slack通知发送逻辑
	// 占位实现:仅记录日志并返回错误,告知调用方该功能尚未实现
	logger.Info("Slack notification placeholder (not implemented)", zap.String("message", notification.Message))
	return fmt.Errorf("slack notification is not implemented yet")
}

// sendWebhookNotification 发送Webhook通知
func (s *AlertService) sendWebhookNotification(notification *models.Notification, channel *models.NotificationChannel) error {
	// 解析Webhook配置
	var config map[string]interface{}
	err := json.Unmarshal([]byte(channel.Config), &config)
	if err != nil {
		return err
	}

	// TODO: 实现实际的Webhook发送逻辑
	// 占位实现:仅记录日志并返回错误,告知调用方该功能尚未实现
	logger.Info("Webhook notification placeholder (not implemented)", zap.String("message", notification.Message))
	return fmt.Errorf("webhook notification is not implemented yet")
}

// sendDingTalkNotification 发送钉钉通知
func (s *AlertService) sendDingTalkNotification(notification *models.Notification, channel *models.NotificationChannel) error {
	// 解析钉钉配置
	var config map[string]interface{}
	err := json.Unmarshal([]byte(channel.Config), &config)
	if err != nil {
		return err
	}

	// TODO: 实现实际的钉钉通知发送逻辑
	// 占位实现:仅记录日志并返回错误,告知调用方该功能尚未实现
	logger.Info("DingTalk notification placeholder (not implemented)", zap.String("message", notification.Message))
	return fmt.Errorf("dingtalk notification is not implemented yet")
}

// GetAlertStatistics 获取告警统计信息
func (s *AlertService) GetAlertStatistics(timeRange string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 计算时间范围
	var startTime time.Time
	switch timeRange {
	case "24h":
		startTime = time.Now().Add(-24 * time.Hour)
	case "7d":
		startTime = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		startTime = time.Now().Add(-30 * 24 * time.Hour)
	default:
		startTime = time.Now().Add(-24 * time.Hour)
	}

	// 总告警数
	var totalAlerts int64
	s.db.Model(&models.Alert{}).Where("start_time >= ?", startTime).Count(&totalAlerts)
	stats["total_alerts"] = totalAlerts

	// 活跃告警数
	var activeAlerts int64
	s.db.Model(&models.Alert{}).Where("status = ? AND start_time >= ?", models.AlertStatusActive, startTime).Count(&activeAlerts)
	stats["active_alerts"] = activeAlerts

	// 已解决告警数
	var resolvedAlerts int64
	s.db.Model(&models.Alert{}).Where("status = ? AND start_time >= ?", models.AlertStatusResolved, startTime).Count(&resolvedAlerts)
	stats["resolved_alerts"] = resolvedAlerts

	// 按严重级别统计
	severityStats := make(map[string]int64)
	for _, severity := range []string{models.AlertSeverityLow, models.AlertSeverityMedium, models.AlertSeverityHigh, models.AlertSeverityCritical} {
		var count int64
		s.db.Model(&models.Alert{}).Where("severity = ? AND start_time >= ?", severity, startTime).Count(&count)
		severityStats[severity] = count
	}
	stats["severity_stats"] = severityStats

	// 按状态统计
	statusStats := make(map[string]int64)
	for _, status := range []string{models.AlertStatusActive, models.AlertStatusAcknowledged, models.AlertStatusResolved} {
		var count int64
		s.db.Model(&models.Alert{}).Where("status = ? AND start_time >= ?", status, startTime).Count(&count)
		statusStats[status] = count
	}
	stats["status_stats"] = statusStats

	return stats, nil
}

// CleanupOldMetrics 清理旧的指标数据
func (s *AlertService) CleanupOldMetrics(retentionDays int) error {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	return s.db.Where("timestamp < ?", cutoffTime).Delete(&models.SystemMetric{}).Error
}

// CleanupOldAlerts 清理旧的告警数据
func (s *AlertService) CleanupOldAlerts(retentionDays int) error {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	return s.db.Where("start_time < ? AND status = ?", cutoffTime, models.AlertStatusResolved).Delete(&models.Alert{}).Error
}
