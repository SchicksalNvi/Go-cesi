package models

import (
	"fmt"
	"gorm.io/gorm"
	"time"
)

// AlertRule 告警规则
type AlertRule struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null;size:100;uniqueIndex:idx_alert_rule_name" validate:"required,min=1,max=100"`
	Description string         `json:"description" gorm:"size:500" validate:"omitempty,max=500"`
	Metric      string         `json:"metric" gorm:"not null;size:50;index:idx_metric" validate:"required,oneof=cpu memory disk process_status network_io disk_io"`
	Condition   string         `json:"condition" gorm:"not null;size:20" validate:"required,oneof=> < >= <= == !="`
	Threshold   float64        `json:"threshold" gorm:"not null" validate:"required,gte=0"`
	Duration    int            `json:"duration" gorm:"not null;check:duration > 0" validate:"required,min=1"`
	Severity    string         `json:"severity" gorm:"not null;size:20;index:idx_severity" validate:"required,oneof=low medium high critical"`
	Enabled     bool           `json:"enabled" gorm:"default:true;not null;index:idx_enabled"`
	NodeID      *uint          `json:"node_id,omitempty" gorm:"index:idx_node_id" validate:"omitempty,gt=0"`
	ProcessName *string        `json:"process_name,omitempty" gorm:"size:100;index:idx_process_name" validate:"omitempty,max=100"`
	Tags        string         `json:"tags" gorm:"size:500" validate:"omitempty,max=500,json"`
	CreatedBy   string         `json:"created_by" gorm:"size:36;not null;index:idx_alert_rule_created_by"`
	CreatedAt   time.Time      `json:"created_at" gorm:"not null;index:idx_alert_rule_created_at"`
	UpdatedAt   time.Time      `json:"updated_at" gorm:"not null"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	// 关联
	User   User    `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
	Alerts []Alert `json:"alerts,omitempty" gorm:"foreignKey:RuleID"`
}

// Alert 告警记录
type Alert struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	RuleID      uint           `json:"rule_id" gorm:"not null;index:idx_rule_id" validate:"required,gt=0"`
	NodeName    string         `json:"node_name" gorm:"size:100;index:idx_alert_node_name" validate:"omitempty,max=100"`
	ProcessName *string        `json:"process_name,omitempty" gorm:"size:100;index:idx_alert_process_name" validate:"omitempty,max=100"`
	Message     string         `json:"message" gorm:"not null;size:1000" validate:"required,min=1,max=1000"`
	Severity    string         `json:"severity" gorm:"not null;size:20;index:idx_alert_severity" validate:"required,oneof=low medium high critical"`
	Status      string         `json:"status" gorm:"not null;size:20;default:'active';index:idx_alert_status" validate:"required,oneof=active acknowledged resolved"`
	Value       float64        `json:"value" validate:"gte=0"`
	StartTime   time.Time      `json:"start_time" gorm:"not null;index:idx_start_time" validate:"required"`
	EndTime     *time.Time     `json:"end_time,omitempty" gorm:"index:idx_end_time"`
	AckedBy     *string        `json:"acked_by,omitempty" gorm:"size:36;index:idx_acked_by"`
	AckedAt     *time.Time     `json:"acked_at,omitempty"`
	ResolvedBy  *string        `json:"resolved_by,omitempty" gorm:"size:36;index:idx_resolved_by"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
	Metadata    string         `json:"metadata" gorm:"type:text" validate:"omitempty,json"`
	CreatedAt   time.Time      `json:"created_at" gorm:"not null;index:idx_alert_created_at"`
	UpdatedAt   time.Time      `json:"updated_at" gorm:"not null"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	// 关联
	Rule           AlertRule      `json:"rule,omitempty" gorm:"foreignKey:RuleID"`
	AckedByUser    *User          `json:"acked_by_user,omitempty" gorm:"foreignKey:AckedBy;references:ID"`
	ResolvedByUser *User          `json:"resolved_by_user,omitempty" gorm:"foreignKey:ResolvedBy;references:ID"`
	Notifications  []Notification `json:"notifications,omitempty" gorm:"foreignKey:AlertID"`
}

// TableName 指定表名
func (Alert) TableName() string {
	return "alerts"
}

// NotificationChannel 通知渠道
type NotificationChannel struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null;size:100"`
	Type        string         `json:"type" gorm:"not null;size:20"` // email, slack, webhook, sms, dingtalk
	Config      string         `json:"config" gorm:"type:text"`      // JSON格式的配置信息
	Enabled     bool           `json:"enabled" gorm:"default:true"`
	Description string         `json:"description" gorm:"size:500"`
	CreatedBy   string         `json:"created_by" gorm:"size:36"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	// 关联
	User          User           `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
	Notifications []Notification `json:"notifications,omitempty" gorm:"foreignKey:ChannelID"`
}

// Notification 通知记录
type Notification struct {
	ID         uint       `json:"id" gorm:"primaryKey"`
	AlertID    uint       `json:"alert_id" gorm:"not null"`
	ChannelID  uint       `json:"channel_id" gorm:"not null"`
	Status     string     `json:"status" gorm:"not null;size:20;default:'pending'"` // pending, sent, failed, retry
	Message    string     `json:"message" gorm:"type:text"`
	Error      *string    `json:"error,omitempty" gorm:"type:text"`
	SentAt     *time.Time `json:"sent_at,omitempty"`
	RetryCount int        `json:"retry_count" gorm:"default:0"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`

	// 关联
	Alert   Alert               `json:"alert,omitempty" gorm:"foreignKey:AlertID"`
	Channel NotificationChannel `json:"channel,omitempty" gorm:"foreignKey:ChannelID"`
}

// AlertRuleNotificationChannel 告警规则与通知渠道的关联
type AlertRuleNotificationChannel struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	RuleID    uint      `json:"rule_id" gorm:"not null"`
	ChannelID uint      `json:"channel_id" gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`

	// 关联
	Rule    AlertRule           `json:"rule,omitempty" gorm:"foreignKey:RuleID"`
	Channel NotificationChannel `json:"channel,omitempty" gorm:"foreignKey:ChannelID"`
}

// SystemMetric 系统指标
type SystemMetric struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	NodeID      *uint     `json:"node_id,omitempty"`
	ProcessName *string   `json:"process_name,omitempty" gorm:"size:100"`
	MetricType  string    `json:"metric_type" gorm:"not null;size:50"` // cpu, memory, disk, network, process
	MetricName  string    `json:"metric_name" gorm:"not null;size:100"`
	Value       float64   `json:"value" gorm:"not null"`
	Unit        string    `json:"unit" gorm:"size:20"`
	Timestamp   time.Time `json:"timestamp" gorm:"not null;index"`
	CreatedAt   time.Time `json:"created_at"`
}

// AlertSeverity 告警严重级别常量
const (
	AlertSeverityLow      = "low"
	AlertSeverityMedium   = "medium"
	AlertSeverityHigh     = "high"
	AlertSeverityCritical = "critical"
)

// AlertStatus 告警状态常量
const (
	AlertStatusActive       = "active"
	AlertStatusAcknowledged = "acknowledged"
	AlertStatusResolved     = "resolved"
	AlertStatusFiring       = "firing"
)

// NotificationStatus 通知状态常量
const (
	NotificationStatusPending = "pending"
	NotificationStatusSent    = "sent"
	NotificationStatusFailed  = "failed"
	NotificationStatusRetry   = "retry"
)

// NotificationChannelType 通知渠道类型常量
const (
	ChannelTypeEmail    = "email"
	ChannelTypeSlack    = "slack"
	ChannelTypeWebhook  = "webhook"
	ChannelTypeSMS      = "sms"
	ChannelTypeDingTalk = "dingtalk"
	ChannelTypeWeChat   = "wechat"
)

// MetricType 指标类型常量
const (
	MetricTypeCPU     = "cpu"
	MetricTypeMemory  = "memory"
	MetricTypeDisk    = "disk"
	MetricTypeNetwork = "network"
	MetricTypeProcess = "process"
)

// GetSeverityLevel 获取严重级别的数值表示
func (a *Alert) GetSeverityLevel() int {
	switch a.Severity {
	case AlertSeverityLow:
		return 1
	case AlertSeverityMedium:
		return 2
	case AlertSeverityHigh:
		return 3
	case AlertSeverityCritical:
		return 4
	default:
		return 0
	}
}

// IsActive 检查告警是否处于活跃状态
func (a *Alert) IsActive() bool {
	return a.Status == AlertStatusActive
}

// IsAcknowledged 检查告警是否已确认
func (a *Alert) IsAcknowledged() bool {
	return a.Status == AlertStatusAcknowledged
}

// IsResolved 检查告警是否已解决
func (a *Alert) IsResolved() bool {
	return a.Status == AlertStatusResolved
}

// GetDuration 获取告警持续时间
func (a *Alert) GetDuration() time.Duration {
	if a.EndTime != nil {
		return a.EndTime.Sub(a.StartTime)
	}
	return time.Since(a.StartTime)
}

// Acknowledge 确认告警
func (a *Alert) Acknowledge(userID string) {
	a.Status = AlertStatusAcknowledged
	a.AckedBy = &userID
	now := time.Now()
	a.AckedAt = &now
}

// Resolve 解决告警
func (a *Alert) Resolve(userID string) {
	a.Status = AlertStatusResolved
	a.ResolvedBy = &userID
	now := time.Now()
	a.ResolvedAt = &now
	a.EndTime = &now
}

// ShouldTrigger 检查规则是否应该触发告警
func (r *AlertRule) ShouldTrigger(value float64) bool {
	if !r.Enabled {
		return false
	}

	switch r.Condition {
	case ">":
		return value > r.Threshold
	case ">=":
		return value >= r.Threshold
	case "<":
		return value < r.Threshold
	case "<=":
		return value <= r.Threshold
	case "==":
		return value == r.Threshold
	case "!=":
		return value != r.Threshold
	default:
		return false
	}
}

// GetConditionText 获取条件的文本描述
func (r *AlertRule) GetConditionText() string {
	return fmt.Sprintf("%s %s %.2f", r.Metric, r.Condition, r.Threshold)
}

// IsChannelEnabled 检查通知渠道是否启用
func (nc *NotificationChannel) IsChannelEnabled() bool {
	return nc.Enabled
}

// CanRetry 检查通知是否可以重试
func (n *Notification) CanRetry() bool {
	return n.Status == NotificationStatusFailed && n.RetryCount < 3
}

// MarkAsSent 标记通知为已发送
func (n *Notification) MarkAsSent() {
	n.Status = NotificationStatusSent
	now := time.Now()
	n.SentAt = &now
}

// MarkAsFailed 标记通知为失败
func (n *Notification) MarkAsFailed(err error) {
	n.Status = NotificationStatusFailed
	n.RetryCount++
	errorMsg := err.Error()
	n.Error = &errorMsg
}
