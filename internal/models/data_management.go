package models

import (
	"crypto/rand"
	"fmt"
	"gorm.io/gorm"
	"time"
)

// generateID 生成唯一ID
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// DataExportRecord 数据导出记录模型
type DataExportRecord struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:100;not null" json:"name"`
	ExportType  string         `gorm:"size:50;not null" json:"export_type"` // users, logs, configs, processes, all
	Format      string         `gorm:"size:20;not null" json:"format"`      // json, csv, xlsx
	FilePath    string         `gorm:"size:500" json:"file_path"`
	FileSize    int64          `json:"file_size"`
	RecordCount int            `json:"record_count"`
	Status      string         `gorm:"size:50;default:'pending'" json:"status"` // pending, running, completed, failed
	ErrorMsg    string         `gorm:"size:1000" json:"error_msg"`
	CreatedBy   string         `gorm:"size:50;not null" json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	CompletedAt *time.Time     `json:"completed_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index:idx_data_export_record_deleted_at" json:"-"`

	// 关联关系
	Creator User `gorm:"foreignKey:CreatedBy;references:ID" json:"creator,omitempty"`
}

// DataImportRecord 数据导入记录模型
type DataImportRecord struct {
	ID            string         `gorm:"primaryKey" json:"id"`
	Name          string         `gorm:"size:100;not null" json:"name"`
	ImportType    string         `gorm:"size:50;not null" json:"import_type"` // configs
	SourceFile    string         `gorm:"size:500;not null" json:"source_file"`
	FileSize      int64          `json:"file_size"`
	TotalRecords  int            `json:"total_records"`
	SuccessCount  int            `json:"success_count"`
	FailureCount  int            `json:"failure_count"`
	Status        string         `gorm:"size:50;default:'pending'" json:"status"` // pending, running, completed, failed, partial
	ErrorMsg      string         `gorm:"size:1000" json:"error_msg"`
	ValidationLog string         `gorm:"type:text" json:"validation_log"`
	CreatedBy     string         `gorm:"size:50;not null" json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index:idx_data_import_record_deleted_at" json:"-"`

	// 关联关系
	Creator User `gorm:"foreignKey:CreatedBy;references:ID" json:"creator,omitempty"`
}

// SystemSettings 系统设置模型
type SystemSettings struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	Category    string         `gorm:"size:50;default:'general'" json:"category"` // theme, language, email, security
	Key         string         `gorm:"size:100;not null;uniqueIndex:idx_category_key" json:"key"`
	Value       string         `gorm:"type:text" json:"value"`
	ValueType   string         `gorm:"size:20;default:'string'" json:"value_type"` // string, number, boolean, json
	Description string         `gorm:"size:500" json:"description"`
	IsPublic    bool           `gorm:"default:false" json:"is_public"` // 是否允许普通用户查看
	UpdatedBy   *string        `gorm:"size:50" json:"updated_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index:idx_system_settings_deleted_at" json:"-"`

	// 关联关系
	Updater *User `gorm:"foreignKey:UpdatedBy;references:ID" json:"updater,omitempty"`
}

// GetCategory 获取分类，如果为空则返回默认值
func (s *SystemSettings) GetCategory() string {
	if s.Category == "" {
		return "general"
	}
	return s.Category
}

// UserPreferences 用户个人偏好设置模型
type UserPreferences struct {
	ID                   string         `gorm:"primaryKey" json:"id"`
	UserID               string         `gorm:"size:50;not null;uniqueIndex" json:"user_id"`
	Theme                string         `gorm:"size:20;default:'light'" json:"theme"` // light, dark, auto
	Language             string         `gorm:"size:10;default:'en'" json:"language"` // en, zh, zh-CN
	Timezone             string         `gorm:"size:50;default:'UTC'" json:"timezone"`
	DateFormat           string         `gorm:"size:20;default:'YYYY-MM-DD'" json:"date_format"`
	TimeFormat           string         `gorm:"size:20;default:'HH:mm:ss'" json:"time_format"`
	PageSize             int            `gorm:"default:20" json:"page_size"`
	AutoRefresh          bool           `gorm:"default:true" json:"auto_refresh"`
	RefreshInterval      int            `gorm:"default:30" json:"refresh_interval"` // 秒
	
	// 通知设置
	EmailNotifications   bool           `gorm:"default:true" json:"email_notifications"`
	ProcessAlerts        bool           `gorm:"default:true" json:"process_alerts"`
	SystemAlerts         bool           `gorm:"default:true" json:"system_alerts"`
	NodeStatusChanges    bool           `gorm:"default:false" json:"node_status_changes"`
	WeeklyReport         bool           `gorm:"default:false" json:"weekly_report"`
	
	// 其他设置
	Notifications       string         `gorm:"type:text" json:"notifications"`     // JSON格式的额外通知设置
	DashboardLayout     string         `gorm:"type:text" json:"dashboard_layout"`  // JSON格式的仪表板布局
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index:idx_user_preferences_deleted_at" json:"-"`

	// 关联关系
	User User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
}

// BeforeCreate 在创建记录前生成 ID
func (up *UserPreferences) BeforeCreate(tx *gorm.DB) error {
	if up.ID == "" {
		up.ID = generateID()
	}
	return nil
}

// WebhookConfig Webhook配置模型
type WebhookConfig struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:100;not null" json:"name"`
	URL         string         `gorm:"size:500;not null" json:"url"`
	Method      string         `gorm:"size:10;default:'POST'" json:"method"` // POST, PUT
	Headers     string         `gorm:"type:text" json:"headers"`             // JSON格式的请求头
	Events      string         `gorm:"type:text;not null" json:"events"`     // JSON数组，支持的事件类型
	Secret      string         `gorm:"size:100" json:"secret"`               // 用于签名验证
	Timeout     int            `gorm:"default:30" json:"timeout"`            // 超时时间（秒）
	RetryCount  int            `gorm:"default:3" json:"retry_count"`         // 重试次数
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	LastTrigger *time.Time     `json:"last_trigger"`
	CreatedBy   string         `gorm:"size:50;not null" json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index:idx_webhook_config_deleted_at" json:"-"`

	// 关联关系
	Creator User `gorm:"foreignKey:CreatedBy;references:ID" json:"creator,omitempty"`
}

// WebhookLog Webhook执行日志模型
type WebhookLog struct {
	ID         string         `gorm:"primaryKey" json:"id"`
	WebhookID  string         `gorm:"size:50;not null;index" json:"webhook_id"`
	Event      string         `gorm:"size:50;not null" json:"event"`
	Payload    string         `gorm:"type:text" json:"payload"`
	Response   string         `gorm:"type:text" json:"response"`
	StatusCode int            `json:"status_code"`
	Duration   int            `json:"duration"` // 执行时间（毫秒）
	Success    bool           `json:"success"`
	ErrorMsg   string         `gorm:"size:1000" json:"error_msg"`
	RetryCount int            `gorm:"default:0" json:"retry_count"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index:idx_webhook_log_deleted_at" json:"-"`

	// 关联关系
	Webhook WebhookConfig `gorm:"foreignKey:WebhookID;references:ID" json:"webhook,omitempty"`
}

// 常量定义
const (
	// 导出类型
	ExportTypeUsers     = "users"
	ExportTypeLogs      = "logs"
	ExportTypeConfigs   = "configs"
	ExportTypeProcesses = "processes"
	ExportTypeAll       = "all"

	// 导出格式
	ExportFormatJSON = "json"
	ExportFormatCSV  = "csv"
	ExportFormatXLSX = "xlsx"

	// 导入类型
	ImportTypeUsers      = "users"
	ImportTypeConfigs    = "configs"

	// 状态
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusPartial   = "partial"

	// 系统设置分类
	SettingsCategoryTheme    = "theme"
	SettingsCategoryLanguage = "language"
	SettingsCategoryEmail    = "email"
	SettingsCategorySecurity = "security"

	// 主题
	ThemeLight = "light"
	ThemeDark  = "dark"
	ThemeAuto  = "auto"

	// 语言
	LanguageEN   = "en"
	LanguageZH   = "zh"
	LanguageZHCN = "zh-CN"

	// Webhook事件
	WebhookEventProcessStart   = "process.start"
	WebhookEventProcessStop    = "process.stop"
	WebhookEventProcessRestart = "process.restart"
	WebhookEventProcessFailed  = "process.failed"
	WebhookEventUserLogin      = "user.login"
	WebhookEventUserLogout     = "user.logout"
	WebhookEventSystemAlert    = "system.alert"
)
