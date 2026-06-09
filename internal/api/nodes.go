package api

import (
	"fmt"
	"net/http"
	"strconv"

	appErrors "superview/internal/errors"
	"superview/internal/models"
	"superview/internal/services"
	"superview/internal/supervisor"
	"superview/internal/validation"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NodesAPI struct {
	service            *supervisor.SupervisorService
	db                 *gorm.DB
	activityLogService *services.ActivityLogService
}

func NewNodesAPI(service *supervisor.SupervisorService, db *gorm.DB, activityLogService ...*services.ActivityLogService) *NodesAPI {
	api := &NodesAPI{service: service, db: db}
	if len(activityLogService) > 0 {
		api.activityLogService = activityLogService[0]
	}
	return api
}

func getCurrentUser(c *gin.Context) (*models.User, bool) {
	userValue, exists := c.Get("user")
	if !exists {
		handleUnauthorized(c)
		return nil, false
	}

	user, ok := userValue.(*models.User)
	if !ok || user == nil {
		handleUnauthorized(c)
		return nil, false
	}

	return user, true
}

func isNodeActionAllowed(user *models.User, nodeID uint, action string) bool {
	return user != nil && user.CanAccessNode(nodeID, action)
}

func filterNodesByAccess(nodes []*supervisor.Node, nodeIDsByName map[string]uint, user *models.User) []*supervisor.Node {
	if user == nil || user.IsSuperAdmin() || len(user.NodeAccess) == 0 {
		return nodes
	}

	filtered := make([]*supervisor.Node, 0, len(nodes))
	for _, node := range nodes {
		nodeID, exists := nodeIDsByName[node.Name]
		if !exists {
			continue
		}
		if isNodeActionAllowed(user, nodeID, "read") {
			filtered = append(filtered, node)
		}
	}

	return filtered
}

func loadNodeIDsByName(db *gorm.DB, nodes []*supervisor.Node) (map[string]uint, error) {
	nodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	var dbNodes []models.Node
	if len(nodeNames) > 0 {
		if err := db.Select("id", "name").Where("name IN ?", nodeNames).Find(&dbNodes).Error; err != nil {
			return nil, err
		}
	}

	nodeIDsByName := make(map[string]uint, len(dbNodes))
	for _, node := range dbNodes {
		nodeIDsByName[node.Name] = node.ID
	}

	return nodeIDsByName, nil
}

func filterNodesByAction(db *gorm.DB, nodes []*supervisor.Node, user *models.User, action string) ([]*supervisor.Node, error) {
	if user == nil || user.IsSuperAdmin() || len(user.NodeAccess) == 0 {
		return nodes, nil
	}

	nodeIDsByName, err := loadNodeIDsByName(db, nodes)
	if err != nil {
		return nil, err
	}

	filtered := make([]*supervisor.Node, 0, len(nodes))
	for _, node := range nodes {
		nodeID, exists := nodeIDsByName[node.Name]
		if !exists {
			continue
		}
		if isNodeActionAllowed(user, nodeID, action) {
			filtered = append(filtered, node)
		}
	}

	return filtered, nil
}

func (api *NodesAPI) authorizeNodeAccess(c *gin.Context, action string) (*models.Node, bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		return nil, false
	}

	nodeName := c.Param("node_name")
	var node models.Node
	if err := api.db.Where("name = ?", nodeName).First(&node).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			handleNotFound(c, "node", nodeName)
			return nil, false
		}
		handleAppError(c, err)
		return nil, false
	}

	if !isNodeActionAllowed(user, node.ID, action) {
		handleForbidden(c, "Access to this node is forbidden")
		return nil, false
	}

	return &node, true
}

func (api *NodesAPI) GetNodes(c *gin.Context) {
	nodes := api.service.GetAllNodes()
	user, ok := getCurrentUser(c)
	if !ok {
		return
	}

	nodes, err := filterNodesByAction(api.db, nodes, user, "read")
	if err != nil {
		handleAppError(c, err)
		return
	}

	response := make([]map[string]interface{}, len(nodes))
	for i, node := range nodes {
		response[i] = node.Serialize()
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"nodes":  response,
	})
}

func (api *NodesAPI) GetNode(c *gin.Context) {
	nodeName := c.Param("node_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateNoSQLInjection("node_name", nodeName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "read"); !ok {
		return
	}

	node, err := api.service.GetNode(nodeName)
	if err != nil {
		handleNotFound(c, "node", nodeName)
		return
	}
	c.JSON(http.StatusOK, node.Serialize())
}

func (api *NodesAPI) GetNodeProcesses(c *gin.Context) {
	nodeName := c.Param("node_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateNoSQLInjection("node_name", nodeName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "read"); !ok {
		return
	}

	node, err := api.service.GetNode(nodeName)
	if err != nil {
		handleAppError(c, err)
		return
	}

	if err := node.RefreshProcesses(); err != nil {
		handleAppError(c, err)
		return
	}

	// 使用SerializeProcesses方法返回格式化的数据
	processes := node.SerializeProcesses()

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"processes": processes,
	})
}

func (api *NodesAPI) StartProcess(c *gin.Context) {
	nodeName := c.Param("node_name")
	processName := c.Param("process_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateProcessName("process_name", processName)
	validator.ValidateNoSQLInjection("node_name", nodeName)
	validator.ValidateNoSQLInjection("process_name", processName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "execute"); !ok {
		return
	}

	if err := api.service.StartProcess(nodeName, processName); err != nil {
		handleAppError(c, err)
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Started process %s on node %s", processName, nodeName)
		api.activityLogService.LogWithContext(c, "INFO", "start_process", "process", processName, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (api *NodesAPI) StopProcess(c *gin.Context) {
	nodeName := c.Param("node_name")
	processName := c.Param("process_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateProcessName("process_name", processName)
	validator.ValidateNoSQLInjection("node_name", nodeName)
	validator.ValidateNoSQLInjection("process_name", processName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "execute"); !ok {
		return
	}

	if err := api.service.StopProcess(nodeName, processName); err != nil {
		handleAppError(c, err)
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Stopped process %s on node %s", processName, nodeName)
		api.activityLogService.LogWithContext(c, "INFO", "stop_process", "process", processName, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (api *NodesAPI) RestartProcess(c *gin.Context) {
	nodeName := c.Param("node_name")
	processName := c.Param("process_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateProcessName("process_name", processName)
	validator.ValidateNoSQLInjection("node_name", nodeName)
	validator.ValidateNoSQLInjection("process_name", processName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "execute"); !ok {
		return
	}

	if err := api.service.StopProcess(nodeName, processName); err != nil {
		handleInternalError(c, err)
		return
	}
	if err := api.service.StartProcess(nodeName, processName); err != nil {
		handleInternalError(c, err)
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Restarted process %s on node %s", processName, nodeName)
		api.activityLogService.LogWithContext(c, "INFO", "restart_process", "process", processName, msg, nil)
	}

	handleSuccess(c, "Process restarted successfully", nil)
}

func (api *NodesAPI) GetProcessLogs(c *gin.Context) {
	nodeName := c.Param("node_name")
	processName := c.Param("process_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateProcessName("process_name", processName)
	validator.ValidateNoSQLInjection("node_name", nodeName)
	validator.ValidateNoSQLInjection("process_name", processName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "log"); !ok {
		return
	}

	logs, err := api.service.GetProcessLogs(nodeName, processName)
	if err != nil {
		handleAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, logs)
}

// GetProcessLogStream 获取结构化的日志流
func (api *NodesAPI) GetProcessLogStream(c *gin.Context) {
	nodeName := c.Param("node_name")
	processName := c.Param("process_name")

	// 获取查询参数
	offset := -1 // -1 表示从文件末尾读取
	maxLines := 100

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	if maxLinesStr := c.Query("max_lines"); maxLinesStr != "" {
		if m, err := strconv.Atoi(maxLinesStr); err == nil && m > 0 && m <= 1000 {
			maxLines = m
		}
	}

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateProcessName("process_name", processName)
	validator.ValidateNoSQLInjection("node_name", nodeName)
	validator.ValidateNoSQLInjection("process_name", processName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "log"); !ok {
		return
	}

	node, err := api.service.GetNode(nodeName)
	if err != nil {
		handleAppError(c, err)
		return
	}

	// 如果 offset < 0，从文件末尾读取最新日志
	if offset < 0 {
		logStream, err := node.GetProcessLogStreamTail(processName, maxLines)
		if err != nil {
			handleAppError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   logStream,
		})
		return
	}

	// 从指定偏移量读取
	logStream, err := node.GetProcessLogStream(processName, offset, maxLines)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   logStream,
	})
}

// StartAllProcesses starts all processes on a specific node
func (api *NodesAPI) StartAllProcesses(c *gin.Context) {
	nodeName := c.Param("node_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateNoSQLInjection("node_name", nodeName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "execute"); !ok {
		return
	}

	if err := api.service.StartAllProcesses(nodeName); err != nil {
		handleAppError(c, err)
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Started all processes on node %s", nodeName)
		api.activityLogService.LogWithContext(c, "INFO", "start_process", "node", nodeName, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "All processes started"})
}

// StopAllProcesses stops all processes on a specific node
func (api *NodesAPI) StopAllProcesses(c *gin.Context) {
	nodeName := c.Param("node_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateNoSQLInjection("node_name", nodeName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "execute"); !ok {
		return
	}

	if err := api.service.StopAllProcesses(nodeName); err != nil {
		handleAppError(c, err)
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Stopped all processes on node %s", nodeName)
		api.activityLogService.LogWithContext(c, "INFO", "stop_process", "node", nodeName, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "All processes stopped"})
}

// RestartAllProcesses restarts all processes on a specific node
func (api *NodesAPI) RestartAllProcesses(c *gin.Context) {
	nodeName := c.Param("node_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateNodeName("node_name", nodeName)
	validator.ValidateNoSQLInjection("node_name", nodeName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	if _, ok := api.authorizeNodeAccess(c, "execute"); !ok {
		return
	}

	if err := api.service.RestartAllProcesses(nodeName); err != nil {
		handleInternalError(c, err)
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Restarted all processes on node %s", nodeName)
		api.activityLogService.LogWithContext(c, "INFO", "restart_process", "node", nodeName, msg, nil)
	}

	handleSuccess(c, "All processes restarted", nil)
}

// UpdateNode updates a node's name and environment
func (api *NodesAPI) UpdateNode(c *gin.Context) {
	nodeName := c.Param("node_name")

	var req struct {
		Name        string `json:"name" binding:"required,min=1,max=100"`
		Environment string `json:"environment" binding:"max=50"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBadRequest(c, err)
		return
	}

	authorizedNode, ok := api.authorizeNodeAccess(c, "write")
	if !ok {
		return
	}

	tx := api.db.Begin()
	if tx.Error != nil {
		handleInternalError(c, tx.Error)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// 更新数据库（按 old name 查找）
	var node models.Node
	if err := tx.Where("id = ?", authorizedNode.ID).First(&node).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			handleAppError(c, appErrors.NewNotFoundError("node", nodeName))
			tx.Rollback()
			return
		}
		tx.Rollback()
		handleAppError(c, err)
		return
	}

	// 检查新名称是否冲突
	if req.Name != nodeName {
		var count int64
		tx.Model(&models.Node{}).Where("name = ? AND id != ?", req.Name, node.ID).Count(&count)
		if count > 0 {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"status": "error", "message": "node name already exists"})
			return
		}
	}

	oldName := node.Name
	oldEnvironment := node.Environment
	node.Name = req.Name
	node.Environment = req.Environment
	if err := tx.Save(&node).Error; err != nil {
		tx.Rollback()
		handleInternalError(c, err)
		return
	}

	// Keep runtime state and database state in sync. Roll the transaction back
	// if the in-memory service cannot apply the same rename/update.
	if err := api.service.UpdateNodeInfo(nodeName, req.Name, req.Environment); err != nil {
		tx.Rollback()
		handleAppError(c, err)
		return
	}

	if err := tx.Commit().Error; err != nil {
		// Best-effort rollback of the in-memory rename/update when the DB commit fails.
		if revertErr := api.service.UpdateNodeInfo(req.Name, oldName, oldEnvironment); revertErr != nil {
			fmt.Printf("Warning: failed to rollback node update in memory after DB commit failure: %v\n", revertErr)
		}
		handleInternalError(c, err)
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Updated node %s -> name=%s, environment=%s", nodeName, req.Name, req.Environment)
		api.activityLogService.LogWithContext(c, "INFO", "update_node", "node", req.Name, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Node updated"})
}
