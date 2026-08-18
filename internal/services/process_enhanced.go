package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"superview/internal/logger"
	"superview/internal/models"
)

// ProcessEnhancedService 进程增强服务
type ProcessEnhancedService struct {
	db        *gorm.DB
	cronJob   *cron.Cron
	scheduler *TaskScheduler
}

// TaskScheduler 任务调度器
type TaskScheduler struct {
	service *ProcessEnhancedService
	running bool

	// mu 保护 running 字段和 cronEntries 映射,防止并发
	// 重复启动/停止导致 cron 任务重复注册。
	mu sync.Mutex

	// cronEntries 记录 taskID -> cron.EntryID 的映射,用于在
	// 删除/更新任务时精确移除已注册的 cron 条目,避免重复注册。
	cronEntries map[uint]cron.EntryID
}

// NewProcessEnhancedService 创建进程增强服务实例
func NewProcessEnhancedService(db *gorm.DB) *ProcessEnhancedService {
	service := &ProcessEnhancedService{
		db:      db,
		cronJob: cron.New(),
	}
	service.scheduler = &TaskScheduler{
		service:     service,
		running:     false,
		cronEntries: make(map[uint]cron.EntryID),
	}
	return service
}

// StartScheduler 启动任务调度器
func (s *ProcessEnhancedService) StartScheduler() error {
	s.scheduler.mu.Lock()
	defer s.scheduler.mu.Unlock()

	if s.scheduler.running {
		return fmt.Errorf("scheduler is already running")
	}

	// 加载所有启用的定时任务
	err := s.loadScheduledTasks()
	if err != nil {
		return err
	}

	s.cronJob.Start()
	s.scheduler.running = true
	logger.Info("Task scheduler started")
	return nil
}

// StopScheduler 停止任务调度器
func (s *ProcessEnhancedService) StopScheduler() {
	s.scheduler.mu.Lock()
	defer s.scheduler.mu.Unlock()

	if s.scheduler.running {
		s.cronJob.Stop()
		s.scheduler.running = false

		// 清空 cron 条目映射,避免重复注册
		s.scheduler.cronEntries = make(map[uint]cron.EntryID)
		logger.Info("Task scheduler stopped")
	}
}

// removeCronEntry 移除指定任务的 cron 条目(需在持有 scheduler.mu 时调用)
func (s *ProcessEnhancedService) removeCronEntry(taskID uint) {
	entryID, exists := s.scheduler.cronEntries[taskID]
	if !exists {
		return
	}
	s.cronJob.Remove(entryID)
	delete(s.scheduler.cronEntries, taskID)
}

// addCronEntry 注册指定任务的 cron 条目(需在持有 scheduler.mu 时调用)
func (s *ProcessEnhancedService) addCronEntry(task *models.ScheduledTask) error {
	// 先移除旧条目,避免重复注册
	s.removeCronEntry(task.ID)

	entryID, err := s.cronJob.AddFunc(task.CronExpr, func() {
		s.executeScheduledTask(task)
	})
	if err != nil {
		return err
	}
	s.scheduler.cronEntries[task.ID] = entryID
	return nil
}

// loadScheduledTasks 加载定时任务
// 调用方必须持有 s.scheduler.mu(StartScheduler 内已加锁)
func (s *ProcessEnhancedService) loadScheduledTasks() error {
	var tasks []models.ScheduledTask
	err := s.db.Where("enabled = ?", true).Find(&tasks).Error
	if err != nil {
		return err
	}

	for i := range tasks {
		task := &tasks[i]
		err := s.addCronEntry(task)
		if err != nil {
			logger.Error("Failed to add cron job for task", zap.Uint("task_id", task.ID), zap.Error(err))
		}
	}

	return nil
}

// CreateProcessGroup 创建进程分组
func (s *ProcessEnhancedService) CreateProcessGroup(group *models.ProcessGroup) error {
	return createAndPreserveBool(s.db, group, "enabled", group.Enabled)
}

// GetProcessGroups 获取进程分组列表
func (s *ProcessEnhancedService) GetProcessGroups(page, pageSize int, filters map[string]interface{}) ([]models.ProcessGroup, int64, error) {
	var groups []models.ProcessGroup
	var total int64

	query := s.db.Model(&models.ProcessGroup{}).Preload("User").Preload("Processes")

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "enabled":
			query = query.Where("enabled = ?", value)
		case "created_by":
			query = query.Where("created_by = ?", value)
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
	err = query.Offset(offset).Limit(pageSize).Order("priority DESC, created_at DESC").Find(&groups).Error

	return groups, total, err
}

// GetProcessGroupByID 根据ID获取进程分组
func (s *ProcessEnhancedService) GetProcessGroupByID(id uint) (*models.ProcessGroup, error) {
	var group models.ProcessGroup
	err := s.db.Preload("User").Preload("Processes").First(&group, id).Error
	return &group, err
}

// UpdateProcessGroup 更新进程分组
func (s *ProcessEnhancedService) UpdateProcessGroup(id uint, updates map[string]interface{}) error {
	return s.db.Model(&models.ProcessGroup{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteProcessGroup 删除进程分组
func (s *ProcessEnhancedService) DeleteProcessGroup(id uint) error {
	// 先删除分组中的进程项
	err := s.db.Where("group_id = ?", id).Delete(&models.ProcessGroupItem{}).Error
	if err != nil {
		return err
	}
	// 删除分组
	return s.db.Delete(&models.ProcessGroup{}, id).Error
}

// AddProcessToGroup 添加进程到分组
func (s *ProcessEnhancedService) AddProcessToGroup(groupID uint, processName string, nodeID uint) error {
	// 检查是否已存在
	var existing models.ProcessGroupItem
	err := s.db.Where("group_id = ? AND process_name = ? AND node_id = ?", groupID, processName, nodeID).First(&existing).Error
	if err == nil {
		return fmt.Errorf("process already exists in group")
	}

	// 获取下一个排序号
	var maxOrder int
	s.db.Model(&models.ProcessGroupItem{}).Where("group_id = ?", groupID).Select("COALESCE(MAX(order), 0)").Scan(&maxOrder)

	item := &models.ProcessGroupItem{
		GroupID:     groupID,
		ProcessName: processName,
		NodeID:      nodeID,
		Order:       maxOrder + 1,
	}

	return s.db.Create(item).Error
}

// RemoveProcessFromGroup 从分组移除进程
func (s *ProcessEnhancedService) RemoveProcessFromGroup(groupID uint, processName string, nodeID uint) error {
	return s.db.Where("group_id = ? AND process_name = ? AND node_id = ?", groupID, processName, nodeID).Delete(&models.ProcessGroupItem{}).Error
}

// ReorderProcessesInGroup 重新排序分组中的进程
func (s *ProcessEnhancedService) ReorderProcessesInGroup(groupID uint, processOrders []map[string]interface{}) error {
	for _, order := range processOrders {
		processName := order["process_name"].(string)
		nodeID := uint(order["node_id"].(float64))
		newOrder := int(order["order"].(float64))

		err := s.db.Model(&models.ProcessGroupItem{}).
			Where("group_id = ? AND process_name = ? AND node_id = ?", groupID, processName, nodeID).
			Update("order", newOrder).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateProcessDependency 创建进程依赖
func (s *ProcessEnhancedService) CreateProcessDependency(dependency *models.ProcessDependency) error {
	// 检查是否会形成循环依赖
	if s.wouldCreateCircularDependency(dependency) {
		return fmt.Errorf("would create circular dependency")
	}
	return s.db.Create(dependency).Error
}

// wouldCreateCircularDependency 检查是否会形成循环依赖
func (s *ProcessEnhancedService) wouldCreateCircularDependency(newDep *models.ProcessDependency) bool {
	// 简化的循环依赖检查
	// 实际实现应该使用图算法进行深度检查
	var existingDep models.ProcessDependency
	err := s.db.Where("process_name = ? AND node_id = ? AND dependent_process = ? AND dependent_node_id = ?",
		newDep.DependentProcess, newDep.DependentNodeID, newDep.ProcessName, newDep.NodeID).First(&existingDep).Error
	return err == nil
}

// GetProcessDependencies 获取进程依赖列表
func (s *ProcessEnhancedService) GetProcessDependencies(processName string, nodeID uint) ([]models.ProcessDependency, error) {
	var dependencies []models.ProcessDependency
	err := s.db.Where("process_name = ? AND node_id = ?", processName, nodeID).Find(&dependencies).Error
	return dependencies, err
}

// GetDependentProcesses 获取依赖当前进程的其他进程
func (s *ProcessEnhancedService) GetDependentProcesses(processName string, nodeID uint) ([]models.ProcessDependency, error) {
	var dependencies []models.ProcessDependency
	err := s.db.Where("dependent_process = ? AND dependent_node_id = ?", processName, nodeID).Find(&dependencies).Error
	return dependencies, err
}

// DeleteProcessDependency 删除进程依赖
func (s *ProcessEnhancedService) DeleteProcessDependency(id uint) error {
	return s.db.Delete(&models.ProcessDependency{}, id).Error
}

// GetStartupOrder 获取进程启动顺序
func (s *ProcessEnhancedService) GetStartupOrder(processes []string, nodeID uint) ([]string, error) {
	// 构建依赖图
	dependencyMap := make(map[string][]string)
	for _, process := range processes {
		deps, err := s.GetProcessDependencies(process, nodeID)
		if err != nil {
			return nil, err
		}
		for _, dep := range deps {
			if dep.DependencyType == models.DependencyTypeStartAfter {
				dependencyMap[process] = append(dependencyMap[process], dep.DependentProcess)
			}
		}
	}

	// 拓扑排序
	return s.topologicalSort(processes, dependencyMap), nil
}

// topologicalSort 拓扑排序
func (s *ProcessEnhancedService) topologicalSort(processes []string, dependencies map[string][]string) []string {
	// 简化的拓扑排序实现
	visited := make(map[string]bool)
	result := make([]string, 0)

	var visit func(string)
	visit = func(process string) {
		if visited[process] {
			return
		}
		visited[process] = true

		// 先访问依赖的进程
		for _, dep := range dependencies[process] {
			visit(dep)
		}

		result = append(result, process)
	}

	for _, process := range processes {
		visit(process)
	}

	return result
}

// CreateScheduledTask 创建定时任务
func (s *ProcessEnhancedService) CreateScheduledTask(task *models.ScheduledTask) error {
	// 验证Cron表达式
	_, err := cron.ParseStandard(task.CronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %v", err)
	}

	// 计算下次运行时间
	nextRun := s.calculateNextRun(task.CronExpr)
	task.NextRun = &nextRun

	err = createAndPreserveBool(s.db, task, "enabled", task.Enabled)
	if err != nil {
		return err
	}

	// 如果调度器正在运行且任务启用，添加到cron
	s.scheduler.mu.Lock()
	defer s.scheduler.mu.Unlock()
	if s.scheduler.running && task.Enabled {
		err = s.addCronEntry(task)
	}

	return err
}

// calculateNextRun 计算下次运行时间
func (s *ProcessEnhancedService) calculateNextRun(cronExpr string) time.Time {
	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return time.Now().Add(time.Hour) // 默认1小时后
	}
	return schedule.Next(time.Now())
}

// GetScheduledTasks 获取定时任务列表
func (s *ProcessEnhancedService) GetScheduledTasks(page, pageSize int, filters map[string]interface{}) ([]models.ScheduledTask, int64, error) {
	var tasks []models.ScheduledTask
	var total int64

	query := s.db.Model(&models.ScheduledTask{}).Preload("User")

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "enabled":
			query = query.Where("enabled = ?", value)
		case "task_type":
			query = query.Where("task_type = ?", value)
		case "target_type":
			query = query.Where("target_type = ?", value)
		case "created_by":
			query = query.Where("created_by = ?", value)
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
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&tasks).Error

	return tasks, total, err
}

// GetScheduledTaskByID 根据ID获取定时任务
func (s *ProcessEnhancedService) GetScheduledTaskByID(id uint) (*models.ScheduledTask, error) {
	var task models.ScheduledTask
	err := s.db.Preload("User").Preload("Executions").First(&task, id).Error
	return &task, err
}

// UpdateScheduledTask 更新定时任务
func (s *ProcessEnhancedService) UpdateScheduledTask(id uint, updates map[string]interface{}) error {
	// 如果更新了cron表达式，重新计算下次运行时间
	if cronExpr, exists := updates["cron_expr"]; exists {
		cronExprStr, ok := cronExpr.(string)
		if !ok {
			return fmt.Errorf("invalid cron expression")
		}
		if _, err := cron.ParseStandard(cronExprStr); err != nil {
			return fmt.Errorf("invalid cron expression: %v", err)
		}
		nextRun := s.calculateNextRun(cronExprStr)
		updates["next_run"] = nextRun
	}

	if err := s.db.Model(&models.ScheduledTask{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return err
	}

	// 若调度器正在运行,重新注册该任务的 cron 条目(先移除旧条目再按新配置注册),
	// 避免 cron 表达式/启用状态变更后仍执行旧调度。
	s.scheduler.mu.Lock()
	defer s.scheduler.mu.Unlock()
	if s.scheduler.running {
		var task models.ScheduledTask
		if err := s.db.First(&task, id).Error; err == nil {
			if task.Enabled {
				err = s.addCronEntry(&task)
			} else {
				s.removeCronEntry(task.ID)
			}
		}
	}

	return nil
}

// DeleteScheduledTask 删除定时任务
func (s *ProcessEnhancedService) DeleteScheduledTask(id uint) error {
	// 先删除执行记录
	err := s.db.Where("task_id = ?", id).Delete(&models.TaskExecution{}).Error
	if err != nil {
		return err
	}

	// 从 cron 中移除该任务的注册条目,防止删除后仍被调度执行
	s.scheduler.mu.Lock()
	s.removeCronEntry(id)
	s.scheduler.mu.Unlock()

	// 删除任务
	return s.db.Delete(&models.ScheduledTask{}, id).Error
}

// executeScheduledTask 执行定时任务
func (s *ProcessEnhancedService) executeScheduledTask(task *models.ScheduledTask) {
	// 创建执行记录
	execution := &models.TaskExecution{
		TaskID:    task.ID,
		Status:    models.ExecutionStatusRunning,
		StartTime: time.Now(),
	}

	err := s.db.Create(execution).Error
	if err != nil {
		logger.Error("Failed to create task execution record", zap.Error(err))
		return
	}

	// 执行任务
	var output string
	var execErr error

	switch task.TaskType {
	case models.TaskTypeStart:
		output, execErr = s.executeStartTask(task)
	case models.TaskTypeStop:
		output, execErr = s.executeStopTask(task)
	case models.TaskTypeRestart:
		output, execErr = s.executeRestartTask(task)
	case models.TaskTypeCustomCommand:
		output, execErr = s.executeCustomCommand(task)
	default:
		execErr = fmt.Errorf("unknown task type: %s", task.TaskType)
	}

	// 更新执行记录
	status := models.ExecutionStatusSuccess
	var errorMsg *string
	if execErr != nil {
		status = models.ExecutionStatusFailed
		errStr := execErr.Error()
		errorMsg = &errStr
	}

	execution.MarkAsCompleted(status, &output, errorMsg)
	s.db.Save(execution)

	// 更新任务统计
	task.IncrementRunCount()
	nextRun := s.calculateNextRun(task.CronExpr)
	task.NextRun = &nextRun
	s.db.Save(task)

	logger.Info("Task executed", zap.Uint("task_id", task.ID), zap.String("task_name", task.Name), zap.String("status", status))
}

// executeStartTask 执行启动任务
func (s *ProcessEnhancedService) executeStartTask(task *models.ScheduledTask) (string, error) {
	// 这里应该调用实际的进程启动逻辑
	// 为了演示，返回模拟结果
	return fmt.Sprintf("Started %s %s", task.TargetType, task.TargetID), nil
}

// executeStopTask 执行停止任务
func (s *ProcessEnhancedService) executeStopTask(task *models.ScheduledTask) (string, error) {
	// 这里应该调用实际的进程停止逻辑
	return fmt.Sprintf("Stopped %s %s", task.TargetType, task.TargetID), nil
}

// executeRestartTask 执行重启任务
func (s *ProcessEnhancedService) executeRestartTask(task *models.ScheduledTask) (string, error) {
	// 这里应该调用实际的进程重启逻辑
	return fmt.Sprintf("Restarted %s %s", task.TargetType, task.TargetID), nil
}

// executeCustomCommand 执行自定义命令
func (s *ProcessEnhancedService) executeCustomCommand(task *models.ScheduledTask) (string, error) {
	if task.Command == nil {
		return "", fmt.Errorf("custom command is empty")
	}
	// 这里应该执行实际的自定义命令
	return fmt.Sprintf("Executed custom command: %s", *task.Command), nil
}

// GetTaskExecutions 获取任务执行记录
func (s *ProcessEnhancedService) GetTaskExecutions(taskID uint, page, pageSize int) ([]models.TaskExecution, int64, error) {
	var executions []models.TaskExecution
	var total int64

	query := s.db.Model(&models.TaskExecution{}).Where("task_id = ?", taskID)

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("start_time DESC").Find(&executions).Error

	return executions, total, err
}

// CreateProcessTemplate 创建进程模板
func (s *ProcessEnhancedService) CreateProcessTemplate(template *models.ProcessTemplate) error {
	return s.db.Create(template).Error
}

// GetProcessTemplates 获取进程模板列表
func (s *ProcessEnhancedService) GetProcessTemplates(page, pageSize int, filters map[string]interface{}) ([]models.ProcessTemplate, int64, error) {
	var templates []models.ProcessTemplate
	var total int64

	query := s.db.Model(&models.ProcessTemplate{}).Preload("User")

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "category":
			query = query.Where("category = ?", value)
		case "is_public":
			query = query.Where("is_public = ?", value)
		case "created_by":
			query = query.Where("created_by = ?", value)
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
	err = query.Offset(offset).Limit(pageSize).Order("usage_count DESC, created_at DESC").Find(&templates).Error

	return templates, total, err
}

// GetProcessTemplateByID 根据ID获取进程模板
func (s *ProcessEnhancedService) GetProcessTemplateByID(id uint) (*models.ProcessTemplate, error) {
	var template models.ProcessTemplate
	err := s.db.Preload("User").First(&template, id).Error
	return &template, err
}

// UpdateProcessTemplate 更新进程模板
func (s *ProcessEnhancedService) UpdateProcessTemplate(id uint, updates map[string]interface{}) error {
	return s.db.Model(&models.ProcessTemplate{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteProcessTemplate 删除进程模板
func (s *ProcessEnhancedService) DeleteProcessTemplate(id uint) error {
	return s.db.Delete(&models.ProcessTemplate{}, id).Error
}

// UseTemplate 使用模板
func (s *ProcessEnhancedService) UseTemplate(id uint) error {
	var template models.ProcessTemplate
	err := s.db.First(&template, id).Error
	if err != nil {
		return err
	}

	template.IncrementUsage()
	return s.db.Save(&template).Error
}

// CreateProcessBackup 创建进程配置备份
func (s *ProcessEnhancedService) CreateProcessBackup(backup *models.ProcessBackup) error {
	// 获取下一个版本号
	var maxVersion int
	s.db.Model(&models.ProcessBackup{}).
		Where("process_name = ? AND node_id = ?", backup.ProcessName, backup.NodeID).
		Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)

	backup.Version = maxVersion + 1
	return s.db.Create(backup).Error
}

// GetProcessBackups 获取进程配置备份列表
func (s *ProcessEnhancedService) GetProcessBackups(processName string, nodeID uint) ([]models.ProcessBackup, error) {
	var backups []models.ProcessBackup
	err := s.db.Where("process_name = ? AND node_id = ?", processName, nodeID).
		Preload("User").Order("version DESC").Find(&backups).Error
	return backups, err
}

// RestoreProcessBackup 恢复进程配置备份
func (s *ProcessEnhancedService) RestoreProcessBackup(id uint) (*models.ProcessBackup, error) {
	var backup models.ProcessBackup
	err := s.db.First(&backup, id).Error
	if err != nil {
		return nil, err
	}

	// 这里应该实现实际的配置恢复逻辑
	// 为了演示，只返回备份信息
	return &backup, nil
}

// RecordProcessMetrics 记录进程性能指标
func (s *ProcessEnhancedService) RecordProcessMetrics(metrics *models.ProcessMetrics) error {
	return s.db.Create(metrics).Error
}

// GetProcessMetrics 获取进程性能指标
func (s *ProcessEnhancedService) GetProcessMetrics(processName string, nodeID uint, timeRange string, limit int) ([]models.ProcessMetrics, error) {
	var metrics []models.ProcessMetrics

	// 计算时间范围
	var startTime time.Time
	switch timeRange {
	case "1h":
		startTime = time.Now().Add(-time.Hour)
	case "6h":
		startTime = time.Now().Add(-6 * time.Hour)
	case "24h":
		startTime = time.Now().Add(-24 * time.Hour)
	case "7d":
		startTime = time.Now().Add(-7 * 24 * time.Hour)
	default:
		startTime = time.Now().Add(-time.Hour)
	}

	err := s.db.Where("process_name = ? AND node_id = ? AND timestamp >= ?", processName, nodeID, startTime).
		Order("timestamp DESC").Limit(limit).Find(&metrics).Error

	return metrics, err
}

// GetProcessMetricsStatistics 获取进程性能统计
func (s *ProcessEnhancedService) GetProcessMetricsStatistics(processName string, nodeID uint, timeRange string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 计算时间范围
	var startTime time.Time
	switch timeRange {
	case "1h":
		startTime = time.Now().Add(-time.Hour)
	case "6h":
		startTime = time.Now().Add(-6 * time.Hour)
	case "24h":
		startTime = time.Now().Add(-24 * time.Hour)
	case "7d":
		startTime = time.Now().Add(-7 * 24 * time.Hour)
	default:
		startTime = time.Now().Add(-time.Hour)
	}

	// 获取统计数据
	var result struct {
		AvgCPU    float64 `json:"avg_cpu"`
		MaxCPU    float64 `json:"max_cpu"`
		AvgMemory float64 `json:"avg_memory"`
		MaxMemory float64 `json:"max_memory"`
		Count     int64   `json:"count"`
	}

	err := s.db.Model(&models.ProcessMetrics{}).
		Where("process_name = ? AND node_id = ? AND timestamp >= ?", processName, nodeID, startTime).
		Select("AVG(cpu_percent) as avg_cpu, MAX(cpu_percent) as max_cpu, AVG(memory_percent) as avg_memory, MAX(memory_percent) as max_memory, COUNT(*) as count").
		Scan(&result).Error

	if err != nil {
		return nil, err
	}

	stats["avg_cpu"] = result.AvgCPU
	stats["max_cpu"] = result.MaxCPU
	stats["avg_memory"] = result.AvgMemory
	stats["max_memory"] = result.MaxMemory
	stats["data_points"] = result.Count
	stats["time_range"] = timeRange

	return stats, nil
}

// CleanupOldMetrics 清理旧的性能指标数据
func (s *ProcessEnhancedService) CleanupOldMetrics(retentionDays int) error {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	return s.db.Where("timestamp < ?", cutoffTime).Delete(&models.ProcessMetrics{}).Error
}

// CleanupOldExecutions 清理旧的任务执行记录
func (s *ProcessEnhancedService) CleanupOldExecutions(retentionDays int) error {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	return s.db.Where("start_time < ?", cutoffTime).Delete(&models.TaskExecution{}).Error
}
