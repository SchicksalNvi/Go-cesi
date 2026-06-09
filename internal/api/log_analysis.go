package api

import (
	"encoding/json"
	"errors"
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

// LogAnalysisHandler 日志分析处理器
type LogAnalysisHandler struct {
	service            *services.LogAnalysisService
	db                 *gorm.DB
	activityLogService *services.ActivityLogService
}

// NewLogAnalysisHandler 创建日志分析处理器实例
func NewLogAnalysisHandler(db *gorm.DB, activityLogService ...*services.ActivityLogService) *LogAnalysisHandler {
	h := &LogAnalysisHandler{
		service: services.NewLogAnalysisService(db),
		db:      db,
	}
	if len(activityLogService) > 0 {
		h.activityLogService = activityLogService[0]
	}
	return h
}

// CreateLogEntry 创建日志条目
func (h *LogAnalysisHandler) CreateLogEntry(c *gin.Context) {
	var req struct {
		Timestamp   *time.Time              `json:"timestamp"`
		Level       string                  `json:"level" binding:"required"`
		Source      string                  `json:"source" binding:"required"`
		ProcessName string                  `json:"process_name" binding:"required"`
		NodeID      *uint                   `json:"node_id"`
		Message     string                  `json:"message" binding:"required"`
		RawLog      string                  `json:"raw_log"`
		Metadata    *map[string]interface{} `json:"metadata"`
		Tags        *[]string               `json:"tags"`
		Category    string                  `json:"category"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleBadRequest(c, err)
		return
	}

	// 验证日志级别
	if !models.IsValidLogLevel(req.Level) {
		handleBadRequest(c, errors.New("Invalid log level"))
		return
	}

	entry := &models.LogEntry{
		Level:       req.Level,
		Source:      req.Source,
		ProcessName: req.ProcessName,
		NodeID:      req.NodeID,
		Message:     req.Message,
		RawLog:      req.RawLog,
		Category:    req.Category,
	}

	// 设置时间戳
	if req.Timestamp != nil {
		entry.Timestamp = *req.Timestamp
	} else {
		entry.Timestamp = time.Now()
	}

	// 如果没有原始日志，使用消息作为原始日志
	if entry.RawLog == "" {
		entry.RawLog = entry.Message
	}

	// 处理元数据
	if req.Metadata != nil {
		metadataJSON, _ := json.Marshal(*req.Metadata)
		metadataStr := string(metadataJSON)
		entry.Metadata = &metadataStr
	}

	// 处理标签
	if req.Tags != nil {
		tagsJSON, _ := json.Marshal(*req.Tags)
		tagsStr := string(tagsJSON)
		entry.Tags = &tagsStr
	}

	err := h.service.CreateLogEntry(entry)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusCreated, entry)
}

// GetLogEntries 获取日志条目列表
func (h *LogAnalysisHandler) GetLogEntries(c *gin.Context) {
	// 验证分页参数
	validator := validation.NewValidator()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	// 使用统一的分页验证
	validator.ValidatePagination("page", strconv.Itoa(page))
	validator.ValidatePagination("page_size", strconv.Itoa(pageSize))
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pagination parameters", "details": validator.Errors()})
		return
	}

	// 过滤条件
	filters := make(map[string]interface{})
	if level := c.Query("level"); level != "" {
		// 验证日志级别
		validator.ValidateLogLevel("level", level)
		if validator.HasErrors() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid log level", "details": validator.Errors()})
			return
		}
		filters["level"] = level
	}
	if source := c.Query("source"); source != "" {
		filters["source"] = source
	}
	if processName := c.Query("process_name"); processName != "" {
		filters["process_name"] = processName
	}
	if nodeID := c.Query("node_id"); nodeID != "" {
		if id, err := strconv.ParseUint(nodeID, 10, 32); err == nil {
			filters["node_id"] = uint(id)
		}
	}
	if category := c.Query("category"); category != "" {
		filters["category"] = category
	}
	if severity := c.Query("severity"); severity != "" {
		if s, err := strconv.Atoi(severity); err == nil {
			filters["severity"] = s
		}
	}
	if parsed := c.Query("parsed"); parsed != "" {
		filters["parsed"] = parsed == "true"
	}
	if archived := c.Query("archived"); archived != "" {
		filters["archived"] = archived == "true"
	}
	if timeFrom := c.Query("time_from"); timeFrom != "" {
		if t, err := time.Parse(time.RFC3339, timeFrom); err == nil {
			filters["time_from"] = t
		}
	}
	if timeTo := c.Query("time_to"); timeTo != "" {
		if t, err := time.Parse(time.RFC3339, timeTo); err == nil {
			filters["time_to"] = t
		}
	}
	if search := c.Query("search"); search != "" {
		// 验证搜索查询
		validator.ValidateSearchQuery("search", search)
		if validator.HasErrors() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid search query", "details": validator.Errors()})
			return
		}
		filters["search"] = search
	}

	entries, total, err := h.service.GetLogEntries(page, pageSize, filters)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        entries,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetLogEntry 获取单个日志条目
func (h *LogAnalysisHandler) GetLogEntry(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		handleInvalidID(c, "log entry")
		return
	}

	entry, err := h.service.GetLogEntryByID(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			handleNotFound(c, "log entry", c.Param("id"))
		} else {
			handleAppError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, entry)
}

// DeleteLogEntry 删除日志条目
func (h *LogAnalysisHandler) DeleteLogEntry(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		handleInvalidID(c, "log entry")
		return
	}

	err = h.service.DeleteLogEntry(uint(id))
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Log entry deleted successfully"})
}

// CreateAnalysisRule 创建分析规则
func (h *LogAnalysisHandler) CreateAnalysisRule(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        string                  `json:"name" binding:"required"`
		Description string                  `json:"description"`
		Pattern     string                  `json:"pattern" binding:"required"`
		PatternType string                  `json:"pattern_type"`
		Conditions  *map[string]interface{} `json:"conditions"`
		Actions     *map[string]interface{} `json:"actions"`
		Priority    int                     `json:"priority"`
		IsActive    *bool                   `json:"is_active"`
		Category    string                  `json:"category"`
		Tags        *[]string               `json:"tags"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleBadRequest(c, err)
		return
	}

	// 设置默认模式类型
	if req.PatternType == "" {
		req.PatternType = models.PatternTypeRegex
	}

	// 验证模式类型
	if !models.IsValidPatternType(req.PatternType) {
		handleBadRequest(c, errors.New("Invalid pattern type"))
		return
	}

	rule := &models.LogAnalysisRule{
		Name:        req.Name,
		Description: req.Description,
		Pattern:     req.Pattern,
		PatternType: req.PatternType,
		Priority:    req.Priority,
		IsActive:    boolValueOrDefault(req.IsActive, true),
		Category:    req.Category,
		CreatedBy:   userID,
	}

	// 处理条件
	if req.Conditions != nil {
		conditionsJSON, _ := json.Marshal(*req.Conditions)
		conditionsStr := string(conditionsJSON)
		rule.Conditions = &conditionsStr
	}

	// 处理动作
	if req.Actions != nil {
		actionsJSON, _ := json.Marshal(*req.Actions)
		actionsStr := string(actionsJSON)
		rule.Actions = &actionsStr
	}

	// 处理标签
	if req.Tags != nil {
		tagsJSON, _ := json.Marshal(*req.Tags)
		tagsStr := string(tagsJSON)
		rule.Tags = &tagsStr
	}

	err := h.service.CreateAnalysisRule(rule)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusCreated, rule)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created analysis rule: %s", rule.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_analysis_rule", "analysis_rule", rule.Name, msg, nil)
	}
}

// GetAnalysisRules 获取分析规则列表
func (h *LogAnalysisHandler) GetAnalysisRules(c *gin.Context) {
	// 验证分页参数
	validator := validation.NewValidator()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 使用统一的分页验证
	validator.ValidatePagination("page", strconv.Itoa(page))
	validator.ValidatePagination("page_size", strconv.Itoa(pageSize))
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pagination parameters", "details": validator.Errors()})
		return
	}

	// 过滤条件
	filters := make(map[string]interface{})
	if category := c.Query("category"); category != "" {
		filters["category"] = category
	}
	if isActive := c.Query("is_active"); isActive != "" {
		filters["is_active"] = isActive == "true"
	}
	if search := c.Query("search"); search != "" {
		// 验证搜索查询
		validator.ValidateSearchQuery("search", search)
		if validator.HasErrors() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid search query", "details": validator.Errors()})
			return
		}
		filters["search"] = search
	}

	rules, total, err := h.service.GetAnalysisRules(page, pageSize, filters)
	if err != nil {
		handleAppError(c, err)
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

// GetAnalysisRule 获取单个分析规则
func (h *LogAnalysisHandler) GetAnalysisRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		handleInvalidID(c, "rule")
		return
	}

	rule, err := h.service.GetAnalysisRuleByID(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			handleNotFound(c, "analysis rule", c.Param("id"))
		} else {
			handleAppError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, rule)
}

// UpdateAnalysisRule 更新分析规则
func (h *LogAnalysisHandler) UpdateAnalysisRule(c *gin.Context) {
	id, ok := parseAndValidateID(c, "id", "rule")
	if !ok {
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		handleBadRequest(c, err)
		return
	}

	// 验证模式类型（如果提供）
	if patternType, ok := updates["pattern_type"]; ok {
		if !models.IsValidPatternType(patternType.(string)) {
			handleBadRequest(c, errors.New("Invalid pattern type"))
			return
		}
	}

	err := h.service.UpdateAnalysisRule(id, updates)
	if err != nil {
		handleInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Analysis rule updated successfully"})
}

// DeleteAnalysisRule 删除分析规则
func (h *LogAnalysisHandler) DeleteAnalysisRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		handleInvalidID(c, "rule")
		return
	}

	err = h.service.DeleteAnalysisRule(uint(id))
	if err != nil {
		handleAppError(c, err)
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted analysis rule ID: %d", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_analysis_rule", "analysis_rule", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Analysis rule deleted successfully"})
}

// GetLogStatistics 获取日志统计信息
func (h *LogAnalysisHandler) GetLogStatistics(c *gin.Context) {
	// 过滤条件
	filters := make(map[string]interface{})
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
	if level := c.Query("level"); level != "" {
		// 验证日志级别
		validator := validation.NewValidator()
		validator.ValidateLogLevel("level", level)
		if validator.HasErrors() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid log level", "details": validator.Errors()})
			return
		}
		filters["level"] = level
	}
	if source := c.Query("source"); source != "" {
		filters["source"] = source
	}
	if processName := c.Query("process_name"); processName != "" {
		filters["process_name"] = processName
	}
	if nodeID := c.Query("node_id"); nodeID != "" {
		if id, err := strconv.ParseUint(nodeID, 10, 32); err == nil {
			filters["node_id"] = uint(id)
		}
	}
	if category := c.Query("category"); category != "" {
		filters["category"] = category
	}

	stats, err := h.service.GetLogStatistics(filters)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// GetLogAlerts 获取日志告警列表
func (h *LogAnalysisHandler) GetLogAlerts(c *gin.Context) {
	// 验证分页参数
	validator := validation.NewValidator()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 使用统一的分页验证
	validator.ValidatePagination("page", strconv.Itoa(page))
	validator.ValidatePagination("page_size", strconv.Itoa(pageSize))
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pagination parameters", "details": validator.Errors()})
		return
	}

	// 过滤条件
	filters := make(map[string]interface{})
	if status := c.Query("status"); status != "" {
		filters["status"] = status
	}
	if level := c.Query("level"); level != "" {
		filters["level"] = level
	}
	if ruleID := c.Query("rule_id"); ruleID != "" {
		if id, err := strconv.ParseUint(ruleID, 10, 32); err == nil {
			filters["rule_id"] = uint(id)
		}
	}
	if acknowledged := c.Query("acknowledged"); acknowledged != "" {
		filters["acknowledged"] = acknowledged == "true"
	}
	if resolved := c.Query("resolved"); resolved != "" {
		filters["resolved"] = resolved == "true"
	}
	if search := c.Query("search"); search != "" {
		filters["search"] = search
	}

	alerts, total, err := h.service.GetLogAlerts(page, pageSize, filters)
	if err != nil {
		handleAppError(c, err)
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

// AcknowledgeAlert 确认告警
func (h *LogAnalysisHandler) AcknowledgeAlert(c *gin.Context) {
	id, ok := parseAndValidateID(c, "id", "alert")
	if !ok {
		return
	}

	userID, ok := validateUserAuthString(c)
	if !ok {
		return
	}

	err := h.service.AcknowledgeAlert(id, userID)
	if err != nil {
		handleInternalError(c, err)
		return
	}

	handleSuccess(c, "Alert acknowledged successfully", nil)
}

// ResolveAlert 解决告警
func (h *LogAnalysisHandler) ResolveAlert(c *gin.Context) {
	id, ok := parseAndValidateID(c, "id", "alert")
	if !ok {
		return
	}

	userID, ok := validateUserAuthString(c)
	if !ok {
		return
	}

	err := h.service.ResolveAlert(id, userID)
	if err != nil {
		handleInternalError(c, err)
		return
	}

	handleSuccess(c, "Alert resolved successfully", nil)
}

// CreateLogFilter 创建日志过滤器
func (h *LogAnalysisHandler) CreateLogFilter(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        string                 `json:"name" binding:"required"`
		Description string                 `json:"description"`
		Filters     map[string]interface{} `json:"filters" binding:"required"`
		IsPublic    bool                   `json:"is_public"`
		IsDefault   bool                   `json:"is_default"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleBadRequest(c, err)
		return
	}

	filtersJSON, _ := json.Marshal(req.Filters)
	filter := &models.LogFilter{
		Name:        req.Name,
		Description: req.Description,
		Filters:     string(filtersJSON),
		IsPublic:    req.IsPublic,
		IsDefault:   req.IsDefault,
		CreatedBy:   userID,
	}

	err := h.service.CreateLogFilter(filter)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusCreated, filter)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created log filter: %s", filter.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_log_filter", "log_filter", filter.Name, msg, nil)
	}
}

// GetLogFilters 获取日志过滤器列表
func (h *LogAnalysisHandler) GetLogFilters(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	isPublic := c.Query("public") == "true"

	filters, err := h.service.GetLogFilters(userID, isPublic)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": filters})
}

// UpdateLogFilter 更新日志过滤器
func (h *LogAnalysisHandler) UpdateLogFilter(c *gin.Context) {
	id, ok := parseAndValidateID(c, "id", "filter")
	if !ok {
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		handleBadRequest(c, err)
		return
	}

	// 如果更新过滤器条件，需要序列化
	if filters, ok := updates["filters"]; ok {
		filtersJSON, _ := json.Marshal(filters)
		updates["filters"] = string(filtersJSON)
	}

	userID, ok := validateUserAuthString(c)
	if !ok {
		return
	}

	err := h.service.UpdateLogFilter(id, updates, userID)
	if err != nil {
		handleInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Log filter updated successfully"})
}

// DeleteLogFilter 删除日志过滤器
func (h *LogAnalysisHandler) DeleteLogFilter(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		handleInvalidID(c, "filter")
		return
	}

	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err = h.service.DeleteLogFilter(uint(id), userID)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Log filter deleted successfully"})
}

// CreateLogExport 创建日志导出任务
func (h *LogAnalysisHandler) CreateLogExport(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        string                 `json:"name" binding:"required"`
		Description string                 `json:"description"`
		Filters     map[string]interface{} `json:"filters" binding:"required"`
		Format      string                 `json:"format"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleBadRequest(c, err)
		return
	}

	// 设置默认格式
	if req.Format == "" {
		req.Format = models.ExportFormatJSON
	}

	// 验证导出格式
	if !models.IsValidExportFormat(req.Format) {
		handleBadRequest(c, errors.New("Invalid export format"))
		return
	}

	filtersJSON, _ := json.Marshal(req.Filters)
	export := &models.LogExport{
		Name:        req.Name,
		Description: req.Description,
		Filters:     string(filtersJSON),
		Format:      req.Format,
		CreatedBy:   userID,
	}

	err := h.service.CreateLogExport(export)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusCreated, export)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created log export: %s", export.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_log_export", "log_export", export.Name, msg, nil)
	}
}

// GetLogExports 获取日志导出任务列表
func (h *LogAnalysisHandler) GetLogExports(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	exports, total, err := h.service.GetLogExports(userID, page, pageSize)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        exports,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// GetLogExport 获取单个日志导出任务
func (h *LogAnalysisHandler) GetLogExport(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		handleInvalidID(c, "export")
		return
	}

	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	export, err := h.service.GetLogExportByID(uint(id), userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			handleNotFound(c, "log export", c.Param("id"))
		} else {
			handleAppError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, export)
}

// DeleteLogExport 删除日志导出任务
func (h *LogAnalysisHandler) DeleteLogExport(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		handleInvalidID(c, "export")
		return
	}

	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err = h.service.DeleteLogExport(uint(id), userID)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Log export deleted successfully"})
}

// CreateRetentionPolicy 创建保留策略
func (h *LogAnalysisHandler) CreateRetentionPolicy(c *gin.Context) {
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name             string                 `json:"name" binding:"required"`
		Description      string                 `json:"description"`
		Conditions       map[string]interface{} `json:"conditions" binding:"required"`
		RetentionDays    int                    `json:"retention_days" binding:"required,min=1"`
		ArchiveAfterDays *int                   `json:"archive_after_days"`
		CompressionType  *string                `json:"compression_type"`
		IsActive         *bool                  `json:"is_active"`
		Priority         int                    `json:"priority"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleBadRequest(c, err)
		return
	}

	conditionsJSON, _ := json.Marshal(req.Conditions)
	policy := &models.LogRetentionPolicy{
		Name:             req.Name,
		Description:      req.Description,
		Conditions:       string(conditionsJSON),
		RetentionDays:    req.RetentionDays,
		ArchiveAfterDays: req.ArchiveAfterDays,
		CompressionType:  req.CompressionType,
		IsActive:         boolValueOrDefault(req.IsActive, true),
		Priority:         req.Priority,
		CreatedBy:        userID,
	}

	err := h.service.CreateRetentionPolicy(policy)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusCreated, policy)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created retention policy: %s", policy.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_retention_policy", "retention_policy", policy.Name, msg, nil)
	}
}

// GetRetentionPolicies 获取保留策略列表
func (h *LogAnalysisHandler) GetRetentionPolicies(c *gin.Context) {
	policies, err := h.service.GetRetentionPolicies()
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": policies})
}

// UpdateRetentionPolicy 更新保留策略
func (h *LogAnalysisHandler) UpdateRetentionPolicy(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		handleInvalidID(c, "policy")
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		handleBadRequest(c, err)
		return
	}

	// 如果更新条件，需要序列化
	if conditions, ok := updates["conditions"]; ok {
		conditionsJSON, _ := json.Marshal(conditions)
		updates["conditions"] = string(conditionsJSON)
	}

	err = h.service.UpdateRetentionPolicy(uint(id), updates)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Retention policy updated successfully"})
}

// DeleteRetentionPolicy 删除保留策略
func (h *LogAnalysisHandler) DeleteRetentionPolicy(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		handleInvalidID(c, "policy")
		return
	}

	err = h.service.DeleteRetentionPolicy(uint(id))
	if err != nil {
		handleAppError(c, err)
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted retention policy ID: %d", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_retention_policy", "retention_policy", fmt.Sprintf("%d", id), msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Retention policy deleted successfully"})
}

// ExecuteRetentionPolicies 执行保留策略
func (h *LogAnalysisHandler) ExecuteRetentionPolicies(c *gin.Context) {
	err := h.service.ExecuteRetentionPolicies()
	if err != nil {
		handleAppError(c, err)
		return
	}

	if h.activityLogService != nil {
		h.activityLogService.LogWithContext(c, "INFO", "execute_retention_policies", "retention_policy", "all", "Executed retention policies", nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Retention policies executed successfully"})
}

// CleanupOldLogs 清理旧日志
func (h *LogAnalysisHandler) CleanupOldLogs(c *gin.Context) {
	var req struct {
		RetentionDays int `json:"retention_days" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.service.CleanupOldLogs(req.RetentionDays)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Old logs cleaned up successfully"})
}
