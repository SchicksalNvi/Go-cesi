package models

import (
	"gorm.io/gorm"
	"time"
)

// ProcessGroup 进程分组
type ProcessGroup struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null;size:100;uniqueIndex"`
	Description string         `json:"description" gorm:"size:500"`
	Color       string         `json:"color" gorm:"size:7;default:'#3B82F6'"` // 十六进制颜色代码
	Icon        string         `json:"icon" gorm:"size:50;default:'folder'"`
	Priority    int            `json:"priority" gorm:"default:0"` // 启动优先级
	Enabled     bool           `json:"enabled" gorm:"default:true"`
	CreatedBy   string         `json:"created_by" gorm:"size:50;index"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	// 关联
	User      User               `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
	Processes []ProcessGroupItem `json:"processes,omitempty" gorm:"foreignKey:GroupID"`
}

// ProcessGroupItem 进程分组项
type ProcessGroupItem struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	GroupID     uint      `json:"group_id" gorm:"not null"`
	ProcessName string    `json:"process_name" gorm:"not null;size:100"`
	NodeID      uint      `json:"node_id" gorm:"not null"`
	Order       int       `json:"order" gorm:"default:0"` // 在组内的排序
	CreatedAt   time.Time `json:"created_at"`

	// 关联
	Group ProcessGroup `json:"group,omitempty" gorm:"foreignKey:GroupID"`
}

// ProcessDependency 进程依赖关系
type ProcessDependency struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	ProcessName      string    `json:"process_name" gorm:"not null;size:100"`
	NodeID           uint      `json:"node_id" gorm:"not null"`
	DependentProcess string    `json:"dependent_process" gorm:"not null;size:100"`
	DependentNodeID  uint      `json:"dependent_node_id" gorm:"not null"`
	DependencyType   string    `json:"dependency_type" gorm:"not null;size:20;default:'start_after'"` // start_after, stop_before, restart_with
	Required         bool      `json:"required" gorm:"default:true"`                                  // 是否为强依赖
	Timeout          int       `json:"timeout" gorm:"default:30"`                                     // 等待超时时间(秒)
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ScheduledTask 定时任务
type ScheduledTask struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null;size:100"`
	Description string         `json:"description" gorm:"size:500"`
	TaskType    string         `json:"task_type" gorm:"not null;size:20"`   // start, stop, restart, custom_command
	TargetType  string         `json:"target_type" gorm:"not null;size:20"` // process, group, node
	TargetID    string         `json:"target_id" gorm:"not null;size:100"`  // 目标ID或名称
	NodeID      *uint          `json:"node_id,omitempty"`
	CronExpr    string         `json:"cron_expr" gorm:"not null;size:100"` // Cron表达式
	Command     *string        `json:"command,omitempty" gorm:"type:text"` // 自定义命令
	Enabled     bool           `json:"enabled" gorm:"default:true"`
	LastRun     *time.Time     `json:"last_run,omitempty"`
	NextRun     *time.Time     `json:"next_run,omitempty"`
	RunCount    int            `json:"run_count" gorm:"default:0"`
	CreatedBy   string         `json:"created_by" gorm:"size:50;index"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	// 关联
	User       User            `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
	Executions []TaskExecution `json:"executions,omitempty" gorm:"foreignKey:TaskID"`
}

// TaskExecution 任务执行记录
type TaskExecution struct {
	ID        uint       `json:"id" gorm:"primaryKey"`
	TaskID    uint       `json:"task_id" gorm:"not null"`
	Status    string     `json:"status" gorm:"not null;size:20"` // pending, running, success, failed, timeout
	StartTime time.Time  `json:"start_time" gorm:"not null"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Duration  int        `json:"duration" gorm:"default:0"` // 执行时长(毫秒)
	Output    *string    `json:"output,omitempty" gorm:"type:text"`
	Error     *string    `json:"error,omitempty" gorm:"type:text"`
	CreatedAt time.Time  `json:"created_at"`

	// 关联
	Task ScheduledTask `json:"task,omitempty" gorm:"foreignKey:TaskID"`
}

// ProcessTemplate 进程模板
type ProcessTemplate struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"not null;size:100;uniqueIndex"`
	Description string         `json:"description" gorm:"size:500"`
	Category    string         `json:"category" gorm:"size:50;default:'general'"`
	Config      string         `json:"config" gorm:"type:text"` // JSON格式的配置模板
	Tags        string         `json:"tags" gorm:"size:500"`    // JSON格式的标签
	IsPublic    bool           `json:"is_public" gorm:"default:false"`
	UsageCount  int            `json:"usage_count" gorm:"default:0"`
	CreatedBy   string         `json:"created_by" gorm:"size:50;index"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	// 关联
	User User `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
}

// ProcessBackup 进程配置备份
type ProcessBackup struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ProcessName string    `json:"process_name" gorm:"not null;size:100"`
	NodeID      uint      `json:"node_id" gorm:"not null"`
	Config      string    `json:"config" gorm:"type:text"` // JSON格式的配置备份
	Version     int       `json:"version" gorm:"not null;default:1"`
	Comment     string    `json:"comment" gorm:"size:500"`
	CreatedBy   string    `json:"created_by" gorm:"size:50;index"`
	CreatedAt   time.Time `json:"created_at"`

	// 关联
	User User `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
}

// ProcessMetrics 进程性能指标
type ProcessMetrics struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	ProcessName   string    `json:"process_name" gorm:"not null;size:100;index"`
	NodeID        uint      `json:"node_id" gorm:"not null;index"`
	PID           int       `json:"pid"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryMB      float64   `json:"memory_mb"`
	MemoryPercent float64   `json:"memory_percent"`
	OpenFiles     int       `json:"open_files"`
	Connections   int       `json:"connections"`
	Uptime        int       `json:"uptime"`   // 运行时间(秒)
	Restarts      int       `json:"restarts"` // 重启次数
	Timestamp     time.Time `json:"timestamp" gorm:"not null;index"`
	CreatedAt     time.Time `json:"created_at"`
}

// 常量定义
const (
	// 依赖类型
	DependencyTypeStartAfter  = "start_after"  // 在依赖进程启动后启动
	DependencyTypeStopBefore  = "stop_before"  // 在依赖进程停止前停止
	DependencyTypeRestartWith = "restart_with" // 与依赖进程一起重启

	// 任务类型
	TaskTypeStart         = "start"
	TaskTypeStop          = "stop"
	TaskTypeRestart       = "restart"
	TaskTypeCustomCommand = "custom_command"

	// 目标类型
	TargetTypeProcess = "process"
	TargetTypeGroup   = "group"
	TargetTypeNode    = "node"

	// 执行状态
	ExecutionStatusPending = "pending"
	ExecutionStatusRunning = "running"
	ExecutionStatusSuccess = "success"
	ExecutionStatusFailed  = "failed"
	ExecutionStatusTimeout = "timeout"

	// 模板分类
	TemplateCategoryGeneral    = "general"
	TemplateCategoryWebServer  = "web_server"
	TemplateCategoryDatabase   = "database"
	TemplateCategoryQueue      = "queue"
	TemplateCategoryMonitoring = "monitoring"
	TemplateCategoryCustom     = "custom"
)

// GetDuration 获取任务执行时长
func (te *TaskExecution) GetDuration() time.Duration {
	if te.EndTime != nil {
		return te.EndTime.Sub(te.StartTime)
	}
	return time.Since(te.StartTime)
}

// IsRunning 检查任务是否正在运行
func (te *TaskExecution) IsRunning() bool {
	return te.Status == ExecutionStatusRunning
}

// IsCompleted 检查任务是否已完成
func (te *TaskExecution) IsCompleted() bool {
	return te.Status == ExecutionStatusSuccess || te.Status == ExecutionStatusFailed || te.Status == ExecutionStatusTimeout
}

// MarkAsCompleted 标记任务为完成
func (te *TaskExecution) MarkAsCompleted(status string, output, errorMsg *string) {
	now := time.Now()
	te.EndTime = &now
	te.Status = status
	te.Duration = int(te.GetDuration().Milliseconds())
	if output != nil {
		te.Output = output
	}
	if errorMsg != nil {
		te.Error = errorMsg
	}
}

// GetProcessCount 获取分组中的进程数量
func (pg *ProcessGroup) GetProcessCount() int {
	return len(pg.Processes)
}

// IsEnabled 检查分组是否启用
func (pg *ProcessGroup) IsEnabled() bool {
	return pg.Enabled
}

// GetNextRunTime 计算下次运行时间
func (st *ScheduledTask) GetNextRunTime() *time.Time {
	// 这里应该根据Cron表达式计算下次运行时间
	// 为了简化，这里返回当前的NextRun字段
	return st.NextRun
}

// IsEnabled 检查任务是否启用
func (st *ScheduledTask) IsEnabled() bool {
	return st.Enabled
}

// ShouldRun 检查任务是否应该运行
func (st *ScheduledTask) ShouldRun() bool {
	if !st.Enabled {
		return false
	}
	if st.NextRun == nil {
		return false
	}
	return time.Now().After(*st.NextRun)
}

// IncrementRunCount 增加运行次数
func (st *ScheduledTask) IncrementRunCount() {
	st.RunCount++
	now := time.Now()
	st.LastRun = &now
}

// IsStrongDependency 检查是否为强依赖
func (pd *ProcessDependency) IsStrongDependency() bool {
	return pd.Required
}

// GetTimeoutDuration 获取超时时长
func (pd *ProcessDependency) GetTimeoutDuration() time.Duration {
	return time.Duration(pd.Timeout) * time.Second
}

// IncrementUsage 增加模板使用次数
func (pt *ProcessTemplate) IncrementUsage() {
	pt.UsageCount++
}

// IsPublicTemplate 检查是否为公共模板
func (pt *ProcessTemplate) IsPublicTemplate() bool {
	return pt.IsPublic
}

// GetLatestVersion 获取最新版本号
func (pb *ProcessBackup) GetLatestVersion() int {
	return pb.Version
}

// GetCPUUsage 获取CPU使用率
func (pm *ProcessMetrics) GetCPUUsage() float64 {
	return pm.CPUPercent
}

// GetMemoryUsage 获取内存使用率
func (pm *ProcessMetrics) GetMemoryUsage() float64 {
	return pm.MemoryPercent
}

// GetUptimeHours 获取运行时间(小时)
func (pm *ProcessMetrics) GetUptimeHours() float64 {
	return float64(pm.Uptime) / 3600.0
}
