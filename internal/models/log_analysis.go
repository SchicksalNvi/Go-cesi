package models

import (
	"time"

	"gorm.io/gorm"
)

// LogEntry 日志条目
type LogEntry struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Timestamp   time.Time      `json:"timestamp" gorm:"index"`
	Level       string         `json:"level" gorm:"index;size:20"`
	Source      string         `json:"source" gorm:"index;size:100"`
	ProcessName string         `json:"process_name" gorm:"index;size:100"`
	NodeID      *uint          `json:"node_id" gorm:"index"`
	Message     string         `json:"message" gorm:"type:text"`
	RawLog      string         `json:"raw_log" gorm:"type:longtext"`
	Metadata    *string        `json:"metadata" gorm:"type:json"`
	Tags        *string        `json:"tags" gorm:"type:json"`
	Severity    int            `json:"severity" gorm:"index;default:0"`
	Category    string         `json:"category" gorm:"index;size:50"`
	Parsed      bool           `json:"parsed" gorm:"index;default:false"`
	Archived    bool           `json:"archived" gorm:"index;default:false"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index:idx_log_entry_deleted_at"`

	// 关联
	Node *Node `json:"node,omitempty" gorm:"foreignKey:NodeID"`
}

// LogAnalysisRule 日志分析规则
type LogAnalysisRule struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"uniqueIndex;size:100"`
	Description string         `json:"description" gorm:"type:text"`
	Pattern     string         `json:"pattern" gorm:"type:text"`
	PatternType string         `json:"pattern_type" gorm:"size:20;default:'regex'"`
	Conditions  *string        `json:"conditions" gorm:"type:json"`
	Actions     *string        `json:"actions" gorm:"type:json"`
	Priority    int            `json:"priority" gorm:"default:0"`
	IsActive    bool           `json:"is_active" gorm:"default:true"`
	Category    string         `json:"category" gorm:"size:50"`
	Tags        *string        `json:"tags" gorm:"type:json"`
	MatchCount  int64          `json:"match_count" gorm:"default:0"`
	LastMatch   *time.Time     `json:"last_match"`
	CreatedBy   string         `json:"created_by" gorm:"size:50;index"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index:idx_log_analysis_rule_deleted_at"`

	// 关联
	Creator *User `json:"creator,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
}

// LogStatistics 日志统计信息
type LogStatistics struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	Date         time.Time      `json:"date" gorm:"index"`
	Hour         int            `json:"hour" gorm:"index"`
	Level        string         `json:"level" gorm:"index;size:20"`
	Source       string         `json:"source" gorm:"index;size:100"`
	ProcessName  string         `json:"process_name" gorm:"index;size:100"`
	NodeID       *uint          `json:"node_id" gorm:"index"`
	Category     string         `json:"category" gorm:"index;size:50"`
	Count        int64          `json:"count" gorm:"default:0"`
	TotalSize    int64          `json:"total_size" gorm:"default:0"`
	ErrorCount   int64          `json:"error_count" gorm:"default:0"`
	WarningCount int64          `json:"warning_count" gorm:"default:0"`
	InfoCount    int64          `json:"info_count" gorm:"default:0"`
	DebugCount   int64          `json:"debug_count" gorm:"default:0"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index:idx_log_statistics_deleted_at"`

	// 关联
	Node *Node `json:"node,omitempty" gorm:"foreignKey:NodeID"`
}

// LogAlert 日志告警
type LogAlert struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	RuleID         uint           `json:"rule_id" gorm:"index"`
	LogEntryID     uint           `json:"log_entry_id" gorm:"index"`
	Level          string         `json:"level" gorm:"index;size:20"`
	Title          string         `json:"title" gorm:"size:200"`
	Message        string         `json:"message" gorm:"type:text"`
	Metadata       *string        `json:"metadata" gorm:"type:json"`
	Status         string         `json:"status" gorm:"index;size:20;default:'active'"`
	Severity       int            `json:"severity" gorm:"index;default:0"`
	Count          int            `json:"count" gorm:"default:1"`
	FirstSeen      time.Time      `json:"first_seen"`
	LastSeen       time.Time      `json:"last_seen"`
	Acknowledged   bool           `json:"acknowledged" gorm:"default:false"`
	AcknowledgedBy *string        `json:"acknowledged_by" gorm:"size:50;index"`
	AcknowledgedAt *time.Time     `json:"acknowledged_at"`
	Resolved       bool           `json:"resolved" gorm:"default:false"`
	ResolvedBy     *string        `json:"resolved_by" gorm:"size:50;index"`
	ResolvedAt     *time.Time     `json:"resolved_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index:idx_log_alert_deleted_at"`

	// 关联
	Rule         *LogAnalysisRule `json:"rule,omitempty" gorm:"foreignKey:RuleID"`
	LogEntry     *LogEntry        `json:"log_entry,omitempty" gorm:"foreignKey:LogEntryID"`
	Acknowledger *User            `json:"acknowledger,omitempty" gorm:"foreignKey:AcknowledgedBy;references:ID"`
	Resolver     *User            `json:"resolver,omitempty" gorm:"foreignKey:ResolvedBy;references:ID"`
}

// LogFilter 日志过滤器
type LogFilter struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"uniqueIndex;size:100"`
	Description string         `json:"description" gorm:"type:text"`
	Filters     string         `json:"filters" gorm:"type:json"`
	IsPublic    bool           `json:"is_public" gorm:"default:false"`
	IsDefault   bool           `json:"is_default" gorm:"default:false"`
	UsageCount  int64          `json:"usage_count" gorm:"default:0"`
	LastUsed    *time.Time     `json:"last_used"`
	CreatedBy   string         `json:"created_by" gorm:"size:50;index"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index:idx_log_filter_deleted_at"`

	// 关联
	Creator *User `json:"creator,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
}

// LogExport 日志导出任务
type LogExport struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	Name             string         `json:"name" gorm:"size:100"`
	Description      string         `json:"description" gorm:"type:text"`
	Filters          string         `json:"filters" gorm:"type:json"`
	Format           string         `json:"format" gorm:"size:20;default:'json'"`
	Status           string         `json:"status" gorm:"index;size:20;default:'pending'"`
	Progress         int            `json:"progress" gorm:"default:0"`
	TotalRecords     int64          `json:"total_records" gorm:"default:0"`
	ProcessedRecords int64          `json:"processed_records" gorm:"default:0"`
	FilePath         *string        `json:"file_path"`
	FileSize         *int64         `json:"file_size"`
	DownloadURL      *string        `json:"download_url"`
	ExpiresAt        *time.Time     `json:"expires_at"`
	Error            *string        `json:"error" gorm:"type:text"`
	StartedAt        *time.Time     `json:"started_at"`
	CompletedAt      *time.Time     `json:"completed_at"`
	CreatedBy        string         `json:"created_by" gorm:"size:50;index"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index:idx_log_export_deleted_at"`

	// 关联
	Creator *User `json:"creator,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
}

// LogRetentionPolicy 日志保留策略
type LogRetentionPolicy struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	Name             string         `json:"name" gorm:"uniqueIndex;size:100"`
	Description      string         `json:"description" gorm:"type:text"`
	Conditions       string         `json:"conditions" gorm:"type:json"`
	RetentionDays    int            `json:"retention_days" gorm:"default:30"`
	ArchiveAfterDays *int           `json:"archive_after_days"`
	CompressionType  *string        `json:"compression_type" gorm:"size:20"`
	IsActive         bool           `json:"is_active" gorm:"default:true"`
	Priority         int            `json:"priority" gorm:"default:0"`
	LastExecuted     *time.Time     `json:"last_executed"`
	ProcessedCount   int64          `json:"processed_count" gorm:"default:0"`
	DeletedCount     int64          `json:"deleted_count" gorm:"default:0"`
	ArchivedCount    int64          `json:"archived_count" gorm:"default:0"`
	CreatedBy        string         `json:"created_by" gorm:"size:50;index"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index:idx_log_retention_policy_deleted_at"`

	// 关联
	Creator *User `json:"creator,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
}

// 常量定义
const (
	// 日志级别（使用activity_log.go中已定义的常量）
	// LogLevelInfo, LogLevelWarning, LogLevelError 已在activity_log.go中定义
	LogLevelDebug = "debug"
	LogLevelFatal = "fatal"

	// 模式类型
	PatternTypeRegex      = "regex"
	PatternTypeGlob       = "glob"
	PatternTypeContains   = "contains"
	PatternTypeEquals     = "equals"
	PatternTypeStartsWith = "starts_with"
	PatternTypeEndsWith   = "ends_with"

	// 告警状态（使用alert.go中已定义的常量）
	// AlertStatusActive, AlertStatusResolved 已在alert.go中定义
	AlertStatusSuppressed = "suppressed"

	// 导出状态
	ExportStatusPending   = "pending"
	ExportStatusRunning   = "running"
	ExportStatusCompleted = "completed"
	ExportStatusFailed    = "failed"
	ExportStatusCancelled = "cancelled"

	// 导出格式（特定于日志分析）
	ExportFormatTXT  = "txt"
	ExportFormatXML  = "xml"
	// ExportFormatJSON 复用 data_management.go 中的定义,避免重复声明
)

// GetSeverityLevel 根据日志级别获取严重程度数值
func GetSeverityLevel(level string) int {
	switch level {
	case LogLevelDebug:
		return 0
	case LogLevelInfo:
		return 1
	case LogLevelWarning:
		return 2
	case LogLevelError:
		return 3
	case LogLevelFatal:
		return 4
	default:
		return 0
	}
}

// GetLevelFromSeverity 根据严重程度数值获取日志级别
func GetLevelFromSeverity(severity int) string {
	switch severity {
	case 0:
		return LogLevelDebug
	case 1:
		return LogLevelInfo
	case 2:
		return LogLevelWarning
	case 3:
		return LogLevelError
	case 4:
		return LogLevelFatal
	default:
		return LogLevelInfo
	}
}

// IsValidLogLevel 检查日志级别是否有效
func IsValidLogLevel(level string) bool {
	validLevels := []string{LogLevelDebug, LogLevelInfo, LogLevelWarning, LogLevelError, LogLevelFatal}
	for _, validLevel := range validLevels {
		if level == validLevel {
			return true
		}
	}
	return false
}

// IsValidPatternType 检查模式类型是否有效
func IsValidPatternType(patternType string) bool {
	validTypes := []string{PatternTypeRegex, PatternTypeGlob, PatternTypeContains, PatternTypeEquals, PatternTypeStartsWith, PatternTypeEndsWith}
	for _, validType := range validTypes {
		if patternType == validType {
			return true
		}
	}
	return false
}

// IsValidExportFormat 检查导出格式是否有效
func IsValidExportFormat(format string) bool {
	validFormats := []string{ExportFormatTXT, ExportFormatXML, ExportFormatJSON}
	for _, validFormat := range validFormats {
		if format == validFormat {
			return true
		}
	}
	return false
}
