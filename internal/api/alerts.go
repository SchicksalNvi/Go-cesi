package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"superview/internal/models"
	"superview/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AlertHandler 告警处理器
type AlertHandler struct {
	alertService       *services.AlertService
	hub                WebSocketHub
	activityLogService *services.ActivityLogService
}

// NewAlertHandler 创建告警处理器实例
func NewAlertHandler(db *gorm.DB, hub WebSocketHub, activityLogService ...*services.ActivityLogService) *AlertHandler {
	h := &AlertHandler{
		alertService: services.NewAlertService(db),
		hub:          hub,
	}
	if len(activityLogService) > 0 {
		h.activityLogService = activityLogService[0]
	}
	return h
}

// CreateAlertRule 创建告警规则
func (h *AlertHandler) CreateAlertRule(c *gin.Context) {
	var req struct {
		Name        string  `json:"name" binding:"required"`
		Description string  `json:"description"`
		Metric      string  `json:"metric" binding:"required"`
		Condition   string  `json:"condition" binding:"required"`
		Threshold   float64 `json:"threshold" binding:"required"`
		Duration    int     `json:"duration" binding:"required"`
		Severity    string  `json:"severity" binding:"required"`
		Enabled     *bool   `json:"enabled"`
		NodeID      *uint   `json:"node_id,omitempty"`
		ProcessName *string `json:"process_name,omitempty"`
		Tags        string  `json:"tags"`
		ChannelIDs  []uint  `json:"channel_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证严重级别
	if req.Severity != models.AlertSeverityLow && req.Severity != models.AlertSeverityMedium &&
		req.Severity != models.AlertSeverityHigh && req.Severity != models.AlertSeverityCritical {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid severity level"})
		return
	}

	// 验证条件
	validConditions := []string{">", ">=", "<", "<=", "==", "!="}
	validCondition := false
	for _, cond := range validConditions {
		if req.Condition == cond {
			validCondition = true
			break
		}
	}
	if !validCondition {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid condition"})
		return
	}

	// 获取当前用户ID
	userIDStr, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	rule := &models.AlertRule{
		Name:        req.Name,
		Description: req.Description,
		Metric:      req.Metric,
		Condition:   req.Condition,
		Threshold:   req.Threshold,
		Duration:    req.Duration,
		Severity:    req.Severity,
		Enabled:     boolValueOrDefault(req.Enabled, true),
		NodeID:      req.NodeID,
		ProcessName: req.ProcessName,
		Tags:        req.Tags,
		CreatedBy:   userIDStr,
	}

	err := h.alertService.CreateAlertRule(rule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 分配通知渠道
	for _, channelID := range req.ChannelIDs {
		h.alertService.AssignChannelToRule(rule.ID, channelID)
	}

	c.JSON(http.StatusCreated, rule)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created alert rule: %s", rule.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_alert_rule", "alert_rule", rule.Name, msg, nil)
	}
}

// GetAlertRules 获取告警规则列表
func (h *AlertHandler) GetAlertRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filters := make(map[string]interface{})
	if enabled := c.Query("enabled"); enabled != "" {
		filters["enabled"] = enabled == "true"
	}
	if severity := c.Query("severity"); severity != "" {
		filters["severity"] = severity
	}
	if metric := c.Query("metric"); metric != "" {
		filters["metric"] = metric
	}
	if nodeID := c.Query("node_id"); nodeID != "" {
		if id, err := strconv.ParseUint(nodeID, 10, 32); err == nil {
			filters["node_id"] = uint(id)
		}
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	rules, total, err := h.alertService.GetAlertRules(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        rules,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetAlertRule 获取单个告警规则
func (h *AlertHandler) GetAlertRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}

	rule, err := h.alertService.GetAlertRuleByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Alert rule not found"})
		return
	}

	// 获取关联的通知渠道
	channels, _ := h.alertService.GetRuleChannels(rule.ID)
	rule.Alerts = nil // 避免返回过多数据

	c.JSON(http.StatusOK, gin.H{
		"rule":     rule,
		"channels": channels,
	})
}

// UpdateAlertRule 更新告警规则
func (h *AlertHandler) UpdateAlertRule(c *gin.Context) {
	id, ok := parseAndValidateID(c, "id", "rule")
	if !ok {
		return
	}

	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Metric      string  `json:"metric"`
		Condition   string  `json:"condition"`
		Threshold   float64 `json:"threshold"`
		Duration    int     `json:"duration"`
		Severity    string  `json:"severity"`
		Enabled     *bool   `json:"enabled"`
		NodeID      *uint   `json:"node_id"`
		ProcessName *string `json:"process_name"`
		Tags        string  `json:"tags"`
		ChannelIDs  []uint  `json:"channel_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Metric != "" {
		updates["metric"] = req.Metric
	}
	if req.Condition != "" {
		updates["condition"] = req.Condition
	}
	if req.Threshold != 0 {
		updates["threshold"] = req.Threshold
	}
	if req.Duration != 0 {
		updates["duration"] = req.Duration
	}
	if req.Severity != "" {
		updates["severity"] = req.Severity
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.NodeID != nil {
		updates["node_id"] = *req.NodeID
	}
	if req.ProcessName != nil {
		updates["process_name"] = *req.ProcessName
	}
	if req.Tags != "" {
		updates["tags"] = req.Tags
	}

	err := h.alertService.UpdateAlertRule(id, updates)
	if err != nil {
		handleInternalError(c, err)
		return
	}

	// 更新通知渠道关联
	if len(req.ChannelIDs) > 0 {
		// 先删除现有关联
		// 这里简化处理，实际应该更精确地管理关联关系
		for _, channelID := range req.ChannelIDs {
			h.alertService.AssignChannelToRule(uint(id), channelID)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert rule updated successfully"})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Updated alert rule ID: %d", id)
		h.activityLogService.LogWithContext(c, "INFO", "update_alert_rule", "alert_rule", fmt.Sprintf("%d", id), msg, nil)
	}
}

// DeleteAlertRule 删除告警规则
func (h *AlertHandler) DeleteAlertRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}

	err = h.alertService.DeleteAlertRule(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted alert rule ID: %d", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_alert_rule", "alert_rule", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Alert rule deleted successfully"})
}

// GetAlerts 获取告警列表
func (h *AlertHandler) GetAlerts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filters := make(map[string]interface{})
	if status := c.Query("status"); status != "" {
		filters["status"] = status
	}
	if severity := c.Query("severity"); severity != "" {
		filters["severity"] = severity
	}
	if ruleID := c.Query("rule_id"); ruleID != "" {
		if id, err := strconv.ParseUint(ruleID, 10, 32); err == nil {
			filters["rule_id"] = uint(id)
		}
	}
	if nodeID := c.Query("node_id"); nodeID != "" {
		if id, err := strconv.ParseUint(nodeID, 10, 32); err == nil {
			filters["node_id"] = uint(id)
		}
	}
	if startTimeFrom := c.Query("start_time_from"); startTimeFrom != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", startTimeFrom); err == nil {
			filters["start_time_from"] = t
		}
	}
	if startTimeTo := c.Query("start_time_to"); startTimeTo != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", startTimeTo); err == nil {
			filters["start_time_to"] = t
		}
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	alerts, total, err := h.alertService.GetAlerts(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        alerts,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetAlert 获取单个告警
func (h *AlertHandler) GetAlert(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	alert, err := h.alertService.GetAlertByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Alert not found"})
		return
	}

	c.JSON(http.StatusOK, alert)
}

// AcknowledgeAlert 确认告警
func (h *AlertHandler) AcknowledgeAlert(c *gin.Context) {
	id, ok := parseAndValidateID(c, "id", "alert")
	if !ok {
		return
	}

	userID, ok := validateUserAuthString(c)
	if !ok {
		return
	}

	err := h.alertService.AcknowledgeAlert(id, userID)
	if err != nil {
		handleInternalError(c, err)
		return
	}

	// Broadcast alert update event
	h.broadcastAlertEvent("alert_updated", id)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Acknowledged alert ID: %d", id)
		h.activityLogService.LogWithContext(c, "INFO", "acknowledge_alert", "alert", fmt.Sprintf("%d", id), msg, nil)
	}

	handleSuccess(c, "Alert acknowledged", nil)
}

// ResolveAlert 解决告警
func (h *AlertHandler) ResolveAlert(c *gin.Context) {
	id, ok := parseAndValidateID(c, "id", "alert")
	if !ok {
		return
	}

	userID, ok := validateUserAuthString(c)
	if !ok {
		return
	}

	err := h.alertService.ResolveAlert(id, userID)
	if err != nil {
		handleInternalError(c, err)
		return
	}

	// Broadcast alert resolved event
	h.broadcastAlertEvent("alert_resolved", id)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Resolved alert ID: %d", id)
		h.activityLogService.LogWithContext(c, "INFO", "resolve_alert", "alert", fmt.Sprintf("%d", id), msg, nil)
	}

	handleSuccess(c, "Alert resolved successfully", nil)
}

// CreateNotificationChannel 创建通知渠道
func (h *AlertHandler) CreateNotificationChannel(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Type        string `json:"type" binding:"required"`
		Config      string `json:"config" binding:"required"`
		Enabled     *bool  `json:"enabled"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证通知渠道类型
	validTypes := []string{models.ChannelTypeEmail, models.ChannelTypeSlack, models.ChannelTypeWebhook, models.ChannelTypeSMS, models.ChannelTypeDingTalk, models.ChannelTypeWeChat}
	validType := false
	for _, t := range validTypes {
		if req.Type == t {
			validType = true
			break
		}
	}
	if !validType {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel type"})
		return
	}

	userIDStr, ok := validateUserAuthString(c)
	if !ok {
		return
	}

	channel := &models.NotificationChannel{
		Name:        req.Name,
		Type:        req.Type,
		Config:      req.Config,
		Enabled:     boolValueOrDefault(req.Enabled, true),
		Description: req.Description,
		CreatedBy:   userIDStr,
	}

	err := h.alertService.CreateNotificationChannel(channel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, channel)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created notification channel: %s (%s)", channel.Name, channel.Type)
		h.activityLogService.LogWithContext(c, "INFO", "create_notification_channel", "notification_channel", channel.Name, msg, nil)
	}
}

// GetNotificationChannels 获取通知渠道列表
func (h *AlertHandler) GetNotificationChannels(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filters := make(map[string]interface{})
	if enabled := c.Query("enabled"); enabled != "" {
		filters["enabled"] = enabled == "true"
	}
	if channelType := c.Query("type"); channelType != "" {
		filters["type"] = channelType
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	channels, total, err := h.alertService.GetNotificationChannels(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        channels,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetNotificationChannel 获取单个通知渠道
func (h *AlertHandler) GetNotificationChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
		return
	}

	channel, err := h.alertService.GetNotificationChannelByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification channel not found"})
		return
	}

	c.JSON(http.StatusOK, channel)
}

// UpdateNotificationChannel 更新通知渠道
func (h *AlertHandler) UpdateNotificationChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
		return
	}

	var req struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Config      string `json:"config"`
		Enabled     *bool  `json:"enabled"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Type != "" {
		updates["type"] = req.Type
	}
	if req.Config != "" {
		updates["config"] = req.Config
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}

	err = h.alertService.UpdateNotificationChannel(uint(id), updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Updated notification channel ID: %d", id)
		h.activityLogService.LogWithContext(c, "INFO", "update_notification_channel", "notification_channel", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification channel updated successfully"})
}

// DeleteNotificationChannel 删除通知渠道
func (h *AlertHandler) DeleteNotificationChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
		return
	}

	err = h.alertService.DeleteNotificationChannel(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted notification channel ID: %d", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_notification_channel", "notification_channel", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification channel deleted successfully"})
}

// GetAlertStatistics 获取告警统计信息
func (h *AlertHandler) GetAlertStatistics(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "24h")

	stats, err := h.alertService.GetAlertStatistics(timeRange)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// RecordSystemMetric 记录系统指标
func (h *AlertHandler) RecordSystemMetric(c *gin.Context) {
	var req struct {
		NodeID      *uint      `json:"node_id,omitempty"`
		ProcessName *string    `json:"process_name,omitempty"`
		MetricType  string     `json:"metric_type" binding:"required"`
		MetricName  string     `json:"metric_name" binding:"required"`
		Value       float64    `json:"value" binding:"required"`
		Unit        string     `json:"unit"`
		Timestamp   *time.Time `json:"timestamp,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	timestamp := time.Now()
	if req.Timestamp != nil {
		timestamp = *req.Timestamp
	}

	metric := &models.SystemMetric{
		NodeID:      req.NodeID,
		ProcessName: req.ProcessName,
		MetricType:  req.MetricType,
		MetricName:  req.MetricName,
		Value:       req.Value,
		Unit:        req.Unit,
		Timestamp:   timestamp,
	}

	err := h.alertService.RecordSystemMetric(metric)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, metric)
}

// GetSystemMetrics 获取系统指标
func (h *AlertHandler) GetSystemMetrics(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	filters := make(map[string]interface{})
	if nodeID := c.Query("node_id"); nodeID != "" {
		if id, err := strconv.ParseUint(nodeID, 10, 32); err == nil {
			filters["node_id"] = uint(id)
		}
	}
	if processName := c.Query("process_name"); processName != "" {
		filters["process_name"] = processName
	}
	if metricType := c.Query("metric_type"); metricType != "" {
		filters["metric_type"] = metricType
	}
	if metricName := c.Query("metric_name"); metricName != "" {
		filters["metric_name"] = metricName
	}
	if timestampFrom := c.Query("timestamp_from"); timestampFrom != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", timestampFrom); err == nil {
			filters["timestamp_from"] = t
		}
	}
	if timestampTo := c.Query("timestamp_to"); timestampTo != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", timestampTo); err == nil {
			filters["timestamp_to"] = t
		}
	}

	metrics, err := h.alertService.GetSystemMetrics(filters, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  metrics,
		"count": len(metrics),
	})
}

// TestNotificationChannel 测试通知渠道
func (h *AlertHandler) TestNotificationChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid channel ID"})
		return
	}

	channel, err := h.alertService.GetNotificationChannelByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification channel not found"})
		return
	}

	// 这里应该调用实际的通知发送逻辑
	// 为了演示，我们只是返回成功
	c.JSON(http.StatusOK, gin.H{
		"message": "Test notification sent successfully",
		"channel": channel.Name,
		"type":    channel.Type,
	})
}

// broadcastAlertEvent 广播告警事件
func (h *AlertHandler) broadcastAlertEvent(eventType string, alertID uint) {
	if h.hub == nil {
		return
	}

	event := map[string]interface{}{
		"type":      eventType,
		"alert_id":  alertID,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.hub.Broadcast(data)
}
