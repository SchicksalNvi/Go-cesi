package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"superview/internal/models"
	"superview/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// ProcessEnhancedHandler 进程增强处理器
type ProcessEnhancedHandler struct {
	service            *services.ProcessEnhancedService
	db                 *gorm.DB
	activityLogService *services.ActivityLogService
}

// NewProcessEnhancedHandler 创建进程增强处理器实例
func NewProcessEnhancedHandler(db *gorm.DB, activityLogService ...*services.ActivityLogService) *ProcessEnhancedHandler {
	h := &ProcessEnhancedHandler{
		service: services.NewProcessEnhancedService(db),
		db:      db,
	}
	if len(activityLogService) > 0 {
		h.activityLogService = activityLogService[0]
	}
	return h
}

// authorizeNodeAccess 检查当前用户对指定节点的访问权限(节点 ACL 与私有资源隔离)
// action: "read" 读取, "write" 写入, "execute" 执行(start/stop/restart)
// 节点在数据库中不存在或用户无权访问时返回 false,并已写入错误响应。
func (h *ProcessEnhancedHandler) authorizeNodeAccess(c *gin.Context, nodeID uint, action string) bool {
	user, ok := getCurrentUser(c)
	if !ok {
		return false
	}

	var node models.Node
	if err := h.db.First(&node, nodeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			handleNotFound(c, "node", strconv.FormatUint(uint64(nodeID), 10))
		} else {
			handleAppError(c, err)
		}
		return false
	}

	if !isNodeActionAllowed(user, node.ID, action) {
		handleForbidden(c, "Access to this node is forbidden")
		return false
	}

	return true
}

// StartScheduler 启动任务调度器
func (h *ProcessEnhancedHandler) StartScheduler(c *gin.Context) {
	err := h.service.StartScheduler()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Task scheduler started successfully"})

	if h.activityLogService != nil {
		h.activityLogService.LogWithContext(c, "INFO", "start_scheduler", "scheduler", "task_scheduler", "Started task scheduler", nil)
	}
}

// StopScheduler 停止任务调度器
func (h *ProcessEnhancedHandler) StopScheduler(c *gin.Context) {
	h.service.StopScheduler()

	if h.activityLogService != nil {
		h.activityLogService.LogWithContext(c, "WARNING", "stop_scheduler", "scheduler", "task_scheduler", "Stopped task scheduler", nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task scheduler stopped successfully"})
}

// CreateProcessGroup 创建进程分组
func (h *ProcessEnhancedHandler) CreateProcessGroup(c *gin.Context) {
	// 权限检查
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
		Enabled     *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	group := &models.ProcessGroup{
		Name:        req.Name,
		Description: req.Description,
		Priority:    req.Priority,
		Enabled:     boolValueOrDefault(req.Enabled, true),
		CreatedBy:   userID,
	}

	err := h.service.CreateProcessGroup(group)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, group)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created process group: %s", group.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_process_group", "process_group", group.Name, msg, nil)
	}
}

// GetProcessGroups 获取进程分组列表
func (h *ProcessEnhancedHandler) GetProcessGroups(c *gin.Context) {
	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 过滤条件
	filters := make(map[string]interface{})
	if enabled := c.Query("enabled"); enabled != "" {
		filters["enabled"] = enabled == "true"
	}
	if createdBy := c.Query("created_by"); createdBy != "" {
		filters["created_by"] = createdBy
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	groups, total, err := h.service.GetProcessGroups(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        groups,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetProcessGroup 获取单个进程分组
func (h *ProcessEnhancedHandler) GetProcessGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}

	group, err := h.service.GetProcessGroupByID(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Process group not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, group)
}

// UpdateProcessGroup 更新进程分组
func (h *ProcessEnhancedHandler) UpdateProcessGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 添加更新时间
	updates["updated_at"] = time.Now()

	err = h.service.UpdateProcessGroup(uint(id), updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Process group updated successfully"})
}

// DeleteProcessGroup 删除进程分组
func (h *ProcessEnhancedHandler) DeleteProcessGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}

	err = h.service.DeleteProcessGroup(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted process group ID: %d", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_process_group", "process_group", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Process group deleted successfully"})
}

// AddProcessToGroup 添加进程到分组
func (h *ProcessEnhancedHandler) AddProcessToGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}

	var req struct {
		ProcessName string `json:"process_name" binding:"required"`
		NodeID      uint   `json:"node_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, req.NodeID, "write") {
		return
	}

	err = h.service.AddProcessToGroup(uint(id), req.ProcessName, req.NodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Process added to group successfully"})
}

// RemoveProcessFromGroup 从分组移除进程
func (h *ProcessEnhancedHandler) RemoveProcessFromGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}

	var req struct {
		ProcessName string `json:"process_name" binding:"required"`
		NodeID      uint   `json:"node_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, req.NodeID, "write") {
		return
	}

	err = h.service.RemoveProcessFromGroup(uint(id), req.ProcessName, req.NodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Process removed from group successfully"})
}

// ReorderProcessesInGroup 重新排序分组中的进程
func (h *ProcessEnhancedHandler) ReorderProcessesInGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}

	var req struct {
		ProcessOrders []map[string]interface{} `json:"process_orders" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.service.ReorderProcessesInGroup(uint(id), req.ProcessOrders)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Process order updated successfully"})
}

// CreateProcessDependency 创建进程依赖
func (h *ProcessEnhancedHandler) CreateProcessDependency(c *gin.Context) {
	_, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		ProcessName      string `json:"process_name" binding:"required"`
		NodeID           uint   `json:"node_id" binding:"required"`
		DependentProcess string `json:"dependent_process" binding:"required"`
		DependentNodeID  uint   `json:"dependent_node_id" binding:"required"`
		DependencyType   string `json:"dependency_type" binding:"required"`
		Description      string `json:"description"`
		Enabled          bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dependency := &models.ProcessDependency{
		ProcessName:      req.ProcessName,
		NodeID:           req.NodeID,
		DependentProcess: req.DependentProcess,
		DependentNodeID:  req.DependentNodeID,
		DependencyType:   req.DependencyType,
		Required:         true,
		Timeout:          30,
	}

	// 节点 ACL 检查(依赖涉及两个节点,均需访问权限)
	if !h.authorizeNodeAccess(c, req.NodeID, "write") || !h.authorizeNodeAccess(c, req.DependentNodeID, "write") {
		return
	}

	err := h.service.CreateProcessDependency(dependency)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, dependency)
}

// GetProcessDependencies 获取进程依赖列表
func (h *ProcessEnhancedHandler) GetProcessDependencies(c *gin.Context) {
	processName := c.Query("process_name")
	nodeIDStr := c.Query("node_id")

	if processName == "" || nodeIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "process_name and node_id are required"})
		return
	}

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node_id"})
		return
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, uint(nodeID), "read") {
		return
	}

	dependencies, err := h.service.GetProcessDependencies(processName, uint(nodeID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": dependencies})
}

// GetDependentProcesses 获取依赖当前进程的其他进程
func (h *ProcessEnhancedHandler) GetDependentProcesses(c *gin.Context) {
	processName := c.Query("process_name")
	nodeIDStr := c.Query("node_id")

	if processName == "" || nodeIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "process_name and node_id are required"})
		return
	}

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node_id"})
		return
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, uint(nodeID), "read") {
		return
	}

	dependencies, err := h.service.GetDependentProcesses(processName, uint(nodeID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": dependencies})
}

// DeleteProcessDependency 删除进程依赖
func (h *ProcessEnhancedHandler) DeleteProcessDependency(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid dependency ID"})
		return
	}

	err = h.service.DeleteProcessDependency(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Process dependency deleted successfully"})
}

// GetStartupOrder 获取进程启动顺序
func (h *ProcessEnhancedHandler) GetStartupOrder(c *gin.Context) {
	var req struct {
		Processes []string `json:"processes" binding:"required"`
		NodeID    uint     `json:"node_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, req.NodeID, "read") {
		return
	}

	order, err := h.service.GetStartupOrder(req.Processes, req.NodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"startup_order": order})
}

// CreateScheduledTask 创建定时任务
func (h *ProcessEnhancedHandler) CreateScheduledTask(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        string  `json:"name" binding:"required"`
		Description string  `json:"description"`
		CronExpr    string  `json:"cron_expr" binding:"required"`
		TaskType    string  `json:"task_type" binding:"required"`
		TargetType  string  `json:"target_type" binding:"required"`
		TargetID    string  `json:"target_id" binding:"required"`
		Command     *string `json:"command"`
		Enabled     *bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := cron.ParseStandard(req.CronExpr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cron expression"})
		return
	}

	task := &models.ScheduledTask{
		Name:        req.Name,
		Description: req.Description,
		CronExpr:    req.CronExpr,
		TaskType:    req.TaskType,
		TargetType:  req.TargetType,
		TargetID:    req.TargetID,
		Command:     req.Command,
		Enabled:     boolValueOrDefault(req.Enabled, true),
		CreatedBy:   userID,
	}

	err := h.service.CreateScheduledTask(task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, task)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created scheduled task: %s", task.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_scheduled_task", "scheduled_task", task.Name, msg, nil)
	}
}

// GetScheduledTasks 获取定时任务列表
func (h *ProcessEnhancedHandler) GetScheduledTasks(c *gin.Context) {
	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 过滤条件
	filters := make(map[string]interface{})
	if enabled := c.Query("enabled"); enabled != "" {
		filters["enabled"] = enabled == "true"
	}
	if taskType := c.Query("task_type"); taskType != "" {
		filters["task_type"] = taskType
	}
	if targetType := c.Query("target_type"); targetType != "" {
		filters["target_type"] = targetType
	}
	if createdBy := c.Query("created_by"); createdBy != "" {
		filters["created_by"] = createdBy
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	tasks, total, err := h.service.GetScheduledTasks(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        tasks,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetScheduledTask 获取单个定时任务
func (h *ProcessEnhancedHandler) GetScheduledTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	task, err := h.service.GetScheduledTaskByID(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Scheduled task not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, task)
}

// UpdateScheduledTask 更新定时任务
func (h *ProcessEnhancedHandler) UpdateScheduledTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if cronExpr, exists := updates["cron_expr"]; exists {
		cronExprStr, ok := cronExpr.(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cron expression"})
			return
		}
		if _, err := cron.ParseStandard(cronExprStr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cron expression"})
			return
		}
	}

	// 添加更新时间
	updates["updated_at"] = time.Now()

	err = h.service.UpdateScheduledTask(uint(id), updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Scheduled task updated successfully"})
}

// DeleteScheduledTask 删除定时任务
func (h *ProcessEnhancedHandler) DeleteScheduledTask(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	err = h.service.DeleteScheduledTask(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted scheduled task ID: %d", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_scheduled_task", "scheduled_task", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Scheduled task deleted successfully"})
}

// GetTaskExecutions 获取任务执行记录
func (h *ProcessEnhancedHandler) GetTaskExecutions(c *gin.Context) {
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	executions, total, err := h.service.GetTaskExecutions(uint(taskID), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        executions,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// CreateProcessTemplate 创建进程模板
func (h *ProcessEnhancedHandler) CreateProcessTemplate(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Config      string `json:"config" binding:"required"`
		IsPublic    bool   `json:"is_public"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	template := &models.ProcessTemplate{
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Config:      req.Config,
		IsPublic:    req.IsPublic,
		CreatedBy:   userID,
	}

	err := h.service.CreateProcessTemplate(template)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, template)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created process template: %s", template.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_process_template", "process_template", template.Name, msg, nil)
	}
}

// GetProcessTemplates 获取进程模板列表
func (h *ProcessEnhancedHandler) GetProcessTemplates(c *gin.Context) {
	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 过滤条件
	filters := make(map[string]interface{})
	if category := c.Query("category"); category != "" {
		filters["category"] = category
	}
	if isPublic := c.Query("is_public"); isPublic != "" {
		filters["is_public"] = isPublic == "true"
	}
	if createdBy := c.Query("created_by"); createdBy != "" {
		filters["created_by"] = createdBy
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	templates, total, err := h.service.GetProcessTemplates(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        templates,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetProcessTemplate 获取单个进程模板
func (h *ProcessEnhancedHandler) GetProcessTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	template, err := h.service.GetProcessTemplateByID(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Process template not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, template)
}

// UpdateProcessTemplate 更新进程模板
func (h *ProcessEnhancedHandler) UpdateProcessTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 添加更新时间
	updates["updated_at"] = time.Now()

	err = h.service.UpdateProcessTemplate(uint(id), updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Process template updated successfully"})
}

// DeleteProcessTemplate 删除进程模板
func (h *ProcessEnhancedHandler) DeleteProcessTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template ID"})
		return
	}

	err = h.service.DeleteProcessTemplate(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted process template ID: %d", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_process_template", "process_template", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Process template deleted successfully"})
}

// UseTemplate 使用模板
func (h *ProcessEnhancedHandler) UseTemplate(c *gin.Context) {
	id, ok := parseAndValidateID(c, "id", "template")
	if !ok {
		return
	}

	err := h.service.UseTemplate(id)
	if err != nil {
		handleInternalError(c, err)
		return
	}

	handleSuccess(c, "Template usage recorded successfully", nil)
}

// CreateProcessBackup 创建进程配置备份
func (h *ProcessEnhancedHandler) CreateProcessBackup(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		ProcessName string `json:"process_name" binding:"required"`
		NodeID      uint   `json:"node_id" binding:"required"`
		Config      string `json:"config" binding:"required"`
		Comment     string `json:"comment"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	backup := &models.ProcessBackup{
		ProcessName: req.ProcessName,
		NodeID:      req.NodeID,
		Config:      req.Config,
		Comment:     req.Comment,
		CreatedBy:   userID,
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, req.NodeID, "write") {
		return
	}

	err := h.service.CreateProcessBackup(backup)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, backup)
}

// GetProcessBackups 获取进程配置备份列表
func (h *ProcessEnhancedHandler) GetProcessBackups(c *gin.Context) {
	processName := c.Query("process_name")
	nodeIDStr := c.Query("node_id")

	if processName == "" || nodeIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "process_name and node_id are required"})
		return
	}

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node_id"})
		return
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, uint(nodeID), "read") {
		return
	}

	backups, err := h.service.GetProcessBackups(processName, uint(nodeID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": backups})
}

// RestoreProcessBackup 恢复进程配置备份
func (h *ProcessEnhancedHandler) RestoreProcessBackup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid backup ID"})
		return
	}

	backup, err := h.service.RestoreProcessBackup(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Process configuration restored successfully",
		"backup":  backup,
	})
}

// RecordProcessMetrics 记录进程性能指标
func (h *ProcessEnhancedHandler) RecordProcessMetrics(c *gin.Context) {
	var req struct {
		ProcessName   string  `json:"process_name" binding:"required"`
		NodeID        uint    `json:"node_id" binding:"required"`
		CPUPercent    float64 `json:"cpu_percent"`
		MemoryPercent float64 `json:"memory_percent"`
		MemoryUsage   int64   `json:"memory_usage"`
		ThreadCount   int     `json:"thread_count"`
		FileCount     int     `json:"file_count"`
		NetworkIO     int64   `json:"network_io"`
		DiskIO        int64   `json:"disk_io"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	metrics := &models.ProcessMetrics{
		ProcessName:   req.ProcessName,
		NodeID:        req.NodeID,
		CPUPercent:    req.CPUPercent,
		MemoryPercent: req.MemoryPercent,
		MemoryMB:      float64(req.MemoryUsage) / 1024 / 1024,
		OpenFiles:     req.FileCount,
		Connections:   0,
		Uptime:        0,
		Restarts:      0,
		Timestamp:     time.Now(),
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, req.NodeID, "write") {
		return
	}

	err := h.service.RecordProcessMetrics(metrics)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Process metrics recorded successfully"})
}

// GetProcessMetrics 获取进程性能指标
func (h *ProcessEnhancedHandler) GetProcessMetrics(c *gin.Context) {
	processName := c.Query("process_name")
	nodeIDStr := c.Query("node_id")
	timeRange := c.DefaultQuery("time_range", "1h")
	limitStr := c.DefaultQuery("limit", "100")

	if processName == "" || nodeIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "process_name and node_id are required"})
		return
	}

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node_id"})
		return
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, uint(nodeID), "read") {
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 100
	}

	metrics, err := h.service.GetProcessMetrics(processName, uint(nodeID), timeRange, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": metrics})
}

// GetProcessMetricsStatistics 获取进程性能统计
func (h *ProcessEnhancedHandler) GetProcessMetricsStatistics(c *gin.Context) {
	processName := c.Query("process_name")
	nodeIDStr := c.Query("node_id")
	timeRange := c.DefaultQuery("time_range", "1h")

	if processName == "" || nodeIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "process_name and node_id are required"})
		return
	}

	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid node_id"})
		return
	}

	// 节点 ACL 检查
	if !h.authorizeNodeAccess(c, uint(nodeID), "read") {
		return
	}

	stats, err := h.service.GetProcessMetricsStatistics(processName, uint(nodeID), timeRange)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// CleanupOldData 清理旧数据
func (h *ProcessEnhancedHandler) CleanupOldData(c *gin.Context) {
	var req struct {
		MetricsRetentionDays    int `json:"metrics_retention_days"`
		ExecutionsRetentionDays int `json:"executions_retention_days"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 清理性能指标数据
	if req.MetricsRetentionDays > 0 {
		err := h.service.CleanupOldMetrics(req.MetricsRetentionDays)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup metrics: " + err.Error()})
			return
		}
	}

	// 清理任务执行记录
	if req.ExecutionsRetentionDays > 0 {
		err := h.service.CleanupOldExecutions(req.ExecutionsRetentionDays)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup executions: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Old data cleaned up successfully"})
}
