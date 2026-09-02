package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"superview/internal/models"
	"superview/internal/services"
	"superview/internal/validation"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ConfigurationHandler 配置管理处理器
type ConfigurationHandler struct {
	service            *services.ConfigurationService
	db                 *gorm.DB
	validator          *validation.Validator
	activityLogService *services.ActivityLogService
}

// NewConfigurationHandler 创建配置管理处理器实例
func NewConfigurationHandler(db *gorm.DB, activityLogService ...*services.ActivityLogService) *ConfigurationHandler {
	h := &ConfigurationHandler{
		service:   services.NewConfigurationService(db),
		db:        db,
		validator: validation.NewValidator(),
	}
	if len(activityLogService) > 0 {
		h.activityLogService = activityLogService[0]
	}
	return h
}

// getUserWithPermissions 获取用户信息并检查权限
func (h *ConfigurationHandler) getUserWithPermissions(c *gin.Context) (*models.User, error) {
	userID, exists := getUserIDString(c)
	if !exists {
		return nil, fmt.Errorf("unauthorized")
	}

	var user models.User
	err := h.db.Preload("Roles.Permissions").Where("id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// checkSecretPermission 检查用户是否有查看敏感信息的权限
func (h *ConfigurationHandler) checkSecretPermission(user *models.User, resourceType string) bool {
	// 超级管理员拥有所有权限
	if user.IsSuperAdmin() {
		return true
	}

	// 检查具体权限
	switch resourceType {
	case "config":
		return user.HasPermission(models.PermissionConfigViewSecret)
	case "env_var":
		return user.HasPermission(models.PermissionEnvVarViewSecret)
	default:
		return false
	}
}

// CreateConfiguration 创建配置项
func (h *ConfigurationHandler) CreateConfiguration(c *gin.Context) {
	// 权限检查
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Key          string                  `json:"key" binding:"required"`
		Value        string                  `json:"value"`
		DefaultValue string                  `json:"default_value"`
		Description  string                  `json:"description"`
		Category     string                  `json:"category" binding:"required"`
		Type         string                  `json:"type" binding:"required"`
		Scope        string                  `json:"scope" binding:"required"`
		NodeID       *uint                   `json:"node_id"`
		UserID       *string                 `json:"user_id"`
		IsRequired   bool                    `json:"is_required"`
		IsReadonly   bool                    `json:"is_readonly"`
		IsSecret     bool                    `json:"is_secret"`
		Validation   *map[string]interface{} `json:"validation"`
		Options      *[]interface{}          `json:"options"`
		Order        int                     `json:"order"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config := &models.Configuration{
		Key:          req.Key,
		Value:        req.Value,
		DefaultValue: req.DefaultValue,
		Description:  req.Description,
		Category:     req.Category,
		Type:         req.Type,
		Scope:        req.Scope,
		NodeID:       req.NodeID,
		UserID:       req.UserID,
		IsRequired:   req.IsRequired,
		IsReadonly:   req.IsReadonly,
		IsSecret:     req.IsSecret,
		Order:        req.Order,
		CreatedBy:    userID,
	}

	// 处理验证规则
	if req.Validation != nil {
		validationJSON, _ := json.Marshal(*req.Validation)
		validationStr := string(validationJSON)
		config.Validation = &validationStr
	}

	// 处理选项列表
	if req.Options != nil {
		optionsJSON, _ := json.Marshal(*req.Options)
		optionsStr := string(optionsJSON)
		config.Options = &optionsStr
	}

	err := h.service.CreateConfiguration(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, config)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created configuration: %s", config.Key)
		h.activityLogService.LogWithContext(c, "INFO", "create_configuration", "configuration", config.Key, msg, nil)
	}
}

// GetConfigurations 获取配置列表
func (h *ConfigurationHandler) GetConfigurations(c *gin.Context) {
	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 过滤条件
	filters := make(map[string]interface{})
	if category := c.Query("category"); category != "" {
		filters["category"] = category
	}
	if scope := c.Query("scope"); scope != "" {
		filters["scope"] = scope
	}
	if configType := c.Query("type"); configType != "" {
		filters["type"] = configType
	}
	if nodeID := c.Query("node_id"); nodeID != "" {
		if id, err := strconv.ParseUint(nodeID, 10, 32); err == nil {
			filters["node_id"] = uint(id)
		}
	}
	if userID := c.Query("user_id"); userID != "" {
		filters["user_id"] = userID
	}
	if isRequired := c.Query("is_required"); isRequired != "" {
		filters["is_required"] = isRequired == "true"
	}
	if isReadonly := c.Query("is_readonly"); isReadonly != "" {
		filters["is_readonly"] = isReadonly == "true"
	}
	if isSecret := c.Query("is_secret"); isSecret != "" {
		filters["is_secret"] = isSecret == "true"
	}
	if search := c.Query("search"); search != "" {
		h.validator.ValidateSearchQuery("search", search)
		if h.validator.HasErrors() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid search query", "details": h.validator.Errors()})
			return
		}
		filters["search"] = search
	}

	configs, total, err := h.service.GetConfigurations(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        configs,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetConfiguration 获取单个配置
func (h *ConfigurationHandler) GetConfiguration(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration ID"})
		return
	}

	// 获取用户信息
	user, err := h.getUserWithPermissions(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 检查是否显示敏感信息
	showSecret := c.Query("show_secret") == "true"
	if showSecret && !h.checkSecretPermission(user, "config") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to view secret information"})
		return
	}

	configUserID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	config, err := h.service.GetConfigurationByID(uint(id), configUserID, showSecret)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Configuration not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, config)
}

// UpdateConfiguration 更新配置
func (h *ConfigurationHandler) UpdateConfiguration(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration ID"})
		return
	}

	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.service.UpdateConfiguration(uint(id), updates, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Configuration updated successfully"})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Updated configuration ID: %d", id)
		h.activityLogService.LogWithContext(c, "INFO", "update_configuration", "configuration", fmt.Sprintf("%d", id), msg, nil)
	}
}

// DeleteConfiguration 删除配置
func (h *ConfigurationHandler) DeleteConfiguration(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration ID"})
		return
	}

	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err = h.service.DeleteConfiguration(uint(id), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted configuration ID: %d", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_configuration", "configuration", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Configuration deleted successfully"})
}

// CreateEnvironmentVariable 创建环境变量
func (h *ConfigurationHandler) CreateEnvironmentVariable(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        string  `json:"name" binding:"required"`
		Value       string  `json:"value"`
		Description string  `json:"description"`
		NodeID      *uint   `json:"node_id"`
		ProcessName *string `json:"process_name"`
		IsSecret    bool    `json:"is_secret"`
		IsActive    *bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	envVar := &models.EnvironmentVariable{
		Name:        req.Name,
		Value:       req.Value,
		Description: req.Description,
		NodeID:      req.NodeID,
		ProcessName: req.ProcessName,
		IsSecret:    req.IsSecret,
		IsActive:    boolValueOrDefault(req.IsActive, true),
		CreatedBy:   userID,
	}

	err := h.service.CreateEnvironmentVariable(envVar)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, envVar)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created environment variable: %s", envVar.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_env_var", "env_var", envVar.Name, msg, nil)
	}
}

// GetEnvironmentVariables 获取环境变量列表
func (h *ConfigurationHandler) GetEnvironmentVariables(c *gin.Context) {
	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 过滤条件
	filters := make(map[string]interface{})
	if nodeID := c.Query("node_id"); nodeID != "" {
		if id, err := strconv.ParseUint(nodeID, 10, 32); err == nil {
			filters["node_id"] = uint(id)
		}
	}
	if processName := c.Query("process_name"); processName != "" {
		filters["process_name"] = processName
	}
	if isSecret := c.Query("is_secret"); isSecret != "" {
		filters["is_secret"] = isSecret == "true"
	}
	if isActive := c.Query("is_active"); isActive != "" {
		filters["is_active"] = isActive == "true"
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	envVars, total, err := h.service.GetEnvironmentVariables(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        envVars,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetEnvironmentVariable 获取单个环境变量
func (h *ConfigurationHandler) GetEnvironmentVariable(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid environment variable ID"})
		return
	}

	// 获取用户信息
	user, err := h.getUserWithPermissions(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 检查是否显示敏感信息
	showSecret := c.Query("show_secret") == "true"
	if showSecret && !h.checkSecretPermission(user, "env_var") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions to view secret information"})
		return
	}

	envUserID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	envVar, err := h.service.GetEnvironmentVariableByID(uint(id), envUserID, showSecret)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Environment variable not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, envVar)
}

// UpdateEnvironmentVariable 更新环境变量
func (h *ConfigurationHandler) UpdateEnvironmentVariable(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid environment variable ID"})
		return
	}

	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.service.UpdateEnvironmentVariable(uint(id), updates, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Environment variable updated successfully"})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Updated environment variable ID: %d", id)
		h.activityLogService.LogWithContext(c, "INFO", "update_env_var", "env_var", fmt.Sprintf("%d", id), msg, nil)
	}
}

// DeleteEnvironmentVariable 删除环境变量
func (h *ConfigurationHandler) DeleteEnvironmentVariable(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid environment variable ID"})
		return
	}

	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err = h.service.DeleteEnvironmentVariable(uint(id), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted environment variable ID: %d", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_env_var", "env_var", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Environment variable deleted successfully"})
}

// ExportConfigurations 导出配置
func (h *ConfigurationHandler) ExportConfigurations(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 过滤条件
	filters := make(map[string]interface{})
	if category := c.Query("category"); category != "" {
		filters["category"] = category
	}
	if scope := c.Query("scope"); scope != "" {
		filters["scope"] = scope
	}
	if nodeID := c.Query("node_id"); nodeID != "" {
		if id, err := strconv.ParseUint(nodeID, 10, 32); err == nil {
			filters["node_id"] = uint(id)
		}
	}
	if userIDParam := c.Query("user_id"); userIDParam != "" {
		filters["user_id"] = userIDParam
	}
	if processName := c.Query("process_name"); processName != "" {
		filters["process_name"] = processName
	}

	data, err := h.service.ExportConfigurations(filters, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 设置下载头
	filename := fmt.Sprintf("configurations_export_%s.json", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/json")

	c.JSON(http.StatusOK, data)
}

// ImportConfigurations 导入配置
func (h *ConfigurationHandler) ImportConfigurations(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Data              map[string]interface{} `json:"data" binding:"required"`
		OverwriteExisting bool                   `json:"overwrite_existing"`
		ImportConfigs     bool                   `json:"import_configs"`
		ImportEnvVars     bool                   `json:"import_env_vars"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	options := map[string]interface{}{
		"overwrite_existing": req.OverwriteExisting,
		"import_configs":     req.ImportConfigs,
		"import_env_vars":    req.ImportEnvVars,
	}

	err := h.service.ImportConfigurations(req.Data, userID, options)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Configurations imported successfully"})

	if h.activityLogService != nil {
		h.activityLogService.LogWithContext(c, "INFO", "import_configurations", "configuration", "batch", "Imported configurations", nil)
	}
}

// GetConfigurationHistory 获取配置变更历史
func (h *ConfigurationHandler) GetConfigurationHistory(c *gin.Context) {
	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 验证分页参数
	validator := validation.NewValidator()
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")
	pageNum, limitNum := validator.ValidatePagination(pageStr, pageSizeStr)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pagination parameters", "details": validator.Errors()})
		return
	}
	page = pageNum
	pageSize = limitNum

	// 获取配置ID或环境变量ID
	var configID *uint
	var envVarID *uint

	if configIDStr := c.Query("config_id"); configIDStr != "" {
		if id, err := strconv.ParseUint(configIDStr, 10, 32); err == nil {
			configIDVal := uint(id)
			configID = &configIDVal
		}
	}

	if envVarIDStr := c.Query("env_var_id"); envVarIDStr != "" {
		if id, err := strconv.ParseUint(envVarIDStr, 10, 32); err == nil {
			envVarIDVal := uint(id)
			envVarID = &envVarIDVal
		}
	}

	history, total, err := h.service.GetConfigurationHistory(configID, envVarID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        history,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetAuditLogs 获取审计日志
func (h *ConfigurationHandler) GetAuditLogs(c *gin.Context) {
	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 验证分页参数
	validator := validation.NewValidator()
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")
	pageNum, limitNum := validator.ValidatePagination(pageStr, pageSizeStr)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pagination parameters", "details": validator.Errors()})
		return
	}
	page = pageNum
	pageSize = limitNum

	// 过滤条件
	filters := make(map[string]interface{})
	if action := c.Query("action"); action != "" {
		filters["action"] = action
	}
	if resourceType := c.Query("resource_type"); resourceType != "" {
		filters["resource_type"] = resourceType
	}
	if resourceID := c.Query("resource_id"); resourceID != "" {
		if id, err := strconv.ParseUint(resourceID, 10, 32); err == nil {
			filters["resource_id"] = uint(id)
		}
	}
	if createdBy := c.Query("created_by"); createdBy != "" {
		filters["created_by"] = createdBy
	}
	if success := c.Query("success"); success != "" {
		filters["success"] = success == "true"
	}
	if dateFrom := c.Query("date_from"); dateFrom != "" {
		if date, err := time.Parse("2006-01-02", dateFrom); err == nil {
			filters["date_from"] = date
		}
	}
	if dateTo := c.Query("date_to"); dateTo != "" {
		if date, err := time.Parse("2006-01-02", dateTo); err == nil {
			filters["date_to"] = date.Add(24 * time.Hour) // 包含整天
		}
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	audits, total, err := h.service.GetAuditLogs(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        audits,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// CleanupOldData 清理旧数据
func (h *ConfigurationHandler) CleanupOldData(c *gin.Context) {
	var req struct {
		AuditRetentionDays   int `json:"audit_retention_days"`
		HistoryRetentionDays int `json:"history_retention_days"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证保留天数参数
	validator := validation.NewValidator()
	if req.AuditRetentionDays > 0 {
		validator.ValidateRetentionDays("audit_retention_days", req.AuditRetentionDays)
	}
	if req.HistoryRetentionDays > 0 {
		validator.ValidateRetentionDays("history_retention_days", req.HistoryRetentionDays)
	}
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid retention days parameters", "details": validator.Errors()})
		return
	}

	// 清理审计日志
	if req.AuditRetentionDays > 0 {
		err := h.service.CleanupOldAuditLogs(req.AuditRetentionDays)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup audit logs: " + err.Error()})
			return
		}
	}

	// 清理变更历史
	if req.HistoryRetentionDays > 0 {
		err := h.service.CleanupOldHistory(req.HistoryRetentionDays)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cleanup history: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Old data cleaned up successfully"})
}
