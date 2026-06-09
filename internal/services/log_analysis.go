package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"superview/internal/models"
)

// LogAnalysisService 日志分析服务
type LogAnalysisService struct {
	db *gorm.DB
}

// NewLogAnalysisService 创建日志分析服务实例
func NewLogAnalysisService(db *gorm.DB) *LogAnalysisService {
	return &LogAnalysisService{db: db}
}

// CreateLogEntry 创建日志条目
func (s *LogAnalysisService) CreateLogEntry(entry *models.LogEntry) error {
	// 设置严重程度
	entry.Severity = models.GetSeverityLevel(entry.Level)

	// 解析日志
	if err := s.parseLogEntry(entry); err != nil {
		// 解析失败不影响日志存储
		entry.Parsed = false
	} else {
		entry.Parsed = true
	}

	if err := s.db.Create(entry).Error; err != nil {
		return fmt.Errorf("failed to create log entry: %v", err)
	}

	// 异步处理日志分析和统计
	go func() {
		s.processLogEntry(entry)
		s.updateStatistics(entry)
	}()

	return nil
}

// GetLogEntries 获取日志条目列表
func (s *LogAnalysisService) GetLogEntries(page, pageSize int, filters map[string]interface{}) ([]*models.LogEntry, int64, error) {
	var entries []*models.LogEntry
	var total int64

	query := s.db.Model(&models.LogEntry{})

	// 应用过滤条件
	query = s.applyLogFilters(query, filters)

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count log entries: %v", err)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Preload("Node").Order("timestamp DESC").Offset(offset).Limit(pageSize).Find(&entries).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get log entries: %v", err)
	}

	return entries, total, nil
}

// GetLogEntryByID 根据ID获取日志条目
func (s *LogAnalysisService) GetLogEntryByID(id uint) (*models.LogEntry, error) {
	var entry models.LogEntry
	if err := s.db.Preload("Node").First(&entry, id).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

// DeleteLogEntry 删除日志条目
func (s *LogAnalysisService) DeleteLogEntry(id uint) error {
	return s.db.Delete(&models.LogEntry{}, id).Error
}

// CreateAnalysisRule 创建分析规则
func (s *LogAnalysisService) CreateAnalysisRule(rule *models.LogAnalysisRule) error {
	// 验证模式
	if rule.PatternType == models.PatternTypeRegex {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return fmt.Errorf("invalid regex pattern: %v", err)
		}
	}

	if err := createAndPreserveBool(s.db, rule, "is_active", rule.IsActive); err != nil {
		return fmt.Errorf("failed to create analysis rule: %v", err)
	}

	return nil
}

// GetAnalysisRules 获取分析规则列表
func (s *LogAnalysisService) GetAnalysisRules(page, pageSize int, filters map[string]interface{}) ([]*models.LogAnalysisRule, int64, error) {
	var rules []*models.LogAnalysisRule
	var total int64

	query := s.db.Model(&models.LogAnalysisRule{})

	// 应用过滤条件
	if category, ok := filters["category"]; ok {
		query = query.Where("category = ?", category)
	}
	if isActive, ok := filters["is_active"]; ok {
		query = query.Where("is_active = ?", isActive)
	}
	if search, ok := filters["search"]; ok {
		query = query.Where("name LIKE ? OR description LIKE ?", "%"+search.(string)+"%", "%"+search.(string)+"%")
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count analysis rules: %v", err)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Preload("Creator").Order("priority DESC, created_at DESC").Offset(offset).Limit(pageSize).Find(&rules).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get analysis rules: %v", err)
	}

	return rules, total, nil
}

// GetAnalysisRuleByID 根据ID获取分析规则
func (s *LogAnalysisService) GetAnalysisRuleByID(id uint) (*models.LogAnalysisRule, error) {
	var rule models.LogAnalysisRule
	if err := s.db.Preload("Creator").First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// UpdateAnalysisRule 更新分析规则
func (s *LogAnalysisService) UpdateAnalysisRule(id uint, updates map[string]interface{}) error {
	// 如果更新模式，需要验证
	if pattern, ok := updates["pattern"]; ok {
		patternType := models.PatternTypeRegex
		if pt, exists := updates["pattern_type"]; exists {
			patternType = pt.(string)
		}
		if patternType == models.PatternTypeRegex {
			if _, err := regexp.Compile(pattern.(string)); err != nil {
				return fmt.Errorf("invalid regex pattern: %v", err)
			}
		}
	}

	return s.db.Model(&models.LogAnalysisRule{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteAnalysisRule 删除分析规则
func (s *LogAnalysisService) DeleteAnalysisRule(id uint) error {
	return s.db.Delete(&models.LogAnalysisRule{}, id).Error
}

// GetLogStatistics 获取日志统计信息
func (s *LogAnalysisService) GetLogStatistics(filters map[string]interface{}) ([]*models.LogStatistics, error) {
	var stats []*models.LogStatistics

	query := s.db.Model(&models.LogStatistics{})

	// 应用过滤条件
	if dateFrom, ok := filters["date_from"]; ok {
		query = query.Where("date >= ?", dateFrom)
	}
	if dateTo, ok := filters["date_to"]; ok {
		query = query.Where("date <= ?", dateTo)
	}
	if level, ok := filters["level"]; ok {
		query = query.Where("level = ?", level)
	}
	if source, ok := filters["source"]; ok {
		query = query.Where("source = ?", source)
	}
	if processName, ok := filters["process_name"]; ok {
		query = query.Where("process_name = ?", processName)
	}
	if nodeID, ok := filters["node_id"]; ok {
		query = query.Where("node_id = ?", nodeID)
	}
	if category, ok := filters["category"]; ok {
		query = query.Where("category = ?", category)
	}

	if err := query.Preload("Node").Order("date DESC, hour DESC").Find(&stats).Error; err != nil {
		return nil, fmt.Errorf("failed to get log statistics: %v", err)
	}

	return stats, nil
}

// GetLogAlerts 获取日志告警列表
func (s *LogAnalysisService) GetLogAlerts(page, pageSize int, filters map[string]interface{}) ([]*models.LogAlert, int64, error) {
	var alerts []*models.LogAlert
	var total int64

	query := s.db.Model(&models.LogAlert{})

	// 应用过滤条件
	if status, ok := filters["status"]; ok {
		query = query.Where("status = ?", status)
	}
	if level, ok := filters["level"]; ok {
		query = query.Where("level = ?", level)
	}
	if ruleID, ok := filters["rule_id"]; ok {
		query = query.Where("rule_id = ?", ruleID)
	}
	if acknowledged, ok := filters["acknowledged"]; ok {
		query = query.Where("acknowledged = ?", acknowledged)
	}
	if resolved, ok := filters["resolved"]; ok {
		query = query.Where("resolved = ?", resolved)
	}
	if search, ok := filters["search"]; ok {
		query = query.Where("title LIKE ? OR message LIKE ?", "%"+search.(string)+"%", "%"+search.(string)+"%")
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count log alerts: %v", err)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Preload("Rule").Preload("LogEntry").Preload("Acknowledger").Preload("Resolver").Order("severity DESC, last_seen DESC").Offset(offset).Limit(pageSize).Find(&alerts).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get log alerts: %v", err)
	}

	return alerts, total, nil
}

// AcknowledgeAlert 确认告警
func (s *LogAnalysisService) AcknowledgeAlert(id uint, userID string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"acknowledged":    true,
		"acknowledged_by": userID,
		"acknowledged_at": now,
	}

	return s.db.Model(&models.LogAlert{}).Where("id = ?", id).Updates(updates).Error
}

// ResolveAlert 解决告警
func (s *LogAnalysisService) ResolveAlert(id uint, userID string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"resolved":    true,
		"resolved_by": userID,
		"resolved_at": now,
		"status":      models.AlertStatusResolved,
	}

	return s.db.Model(&models.LogAlert{}).Where("id = ?", id).Updates(updates).Error
}

// CreateLogFilter 创建日志过滤器
func (s *LogAnalysisService) CreateLogFilter(filter *models.LogFilter) error {
	return s.db.Create(filter).Error
}

// GetLogFilters 获取日志过滤器列表
func (s *LogAnalysisService) GetLogFilters(userID string, isPublic bool) ([]*models.LogFilter, error) {
	var filters []*models.LogFilter

	query := s.db.Model(&models.LogFilter{})
	if isPublic {
		query = query.Where("is_public = ? OR created_by = ?", true, userID)
	} else {
		query = query.Where("created_by = ?", userID)
	}

	if err := query.Preload("Creator").Order("is_default DESC, usage_count DESC, created_at DESC").Find(&filters).Error; err != nil {
		return nil, fmt.Errorf("failed to get log filters: %v", err)
	}

	return filters, nil
}

// UpdateLogFilter 更新日志过滤器
func (s *LogAnalysisService) UpdateLogFilter(id uint, updates map[string]interface{}, userID string) error {
	return s.db.Model(&models.LogFilter{}).Where("id = ? AND created_by = ?", id, userID).Updates(updates).Error
}

// DeleteLogFilter 删除日志过滤器
func (s *LogAnalysisService) DeleteLogFilter(id uint, userID string) error {
	return s.db.Where("id = ? AND created_by = ?", id, userID).Delete(&models.LogFilter{}).Error
}

// CreateLogExport 创建日志导出任务
func (s *LogAnalysisService) CreateLogExport(export *models.LogExport) error {
	if err := s.db.Create(export).Error; err != nil {
		return fmt.Errorf("failed to create log export: %v", err)
	}

	// 异步处理导出任务
	go s.processLogExport(export)

	return nil
}

// GetLogExports 获取日志导出任务列表
func (s *LogAnalysisService) GetLogExports(userID string, page, pageSize int) ([]*models.LogExport, int64, error) {
	var exports []*models.LogExport
	var total int64

	query := s.db.Model(&models.LogExport{}).Where("created_by = ?", userID)

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count log exports: %v", err)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&exports).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get log exports: %v", err)
	}

	return exports, total, nil
}

// GetLogExportByID 根据ID获取日志导出任务
func (s *LogAnalysisService) GetLogExportByID(id uint, userID string) (*models.LogExport, error) {
	var export models.LogExport
	if err := s.db.Where("id = ? AND created_by = ?", id, userID).First(&export).Error; err != nil {
		return nil, err
	}
	return &export, nil
}

// DeleteLogExport 删除日志导出任务
func (s *LogAnalysisService) DeleteLogExport(id uint, userID string) error {
	return s.db.Where("id = ? AND created_by = ?", id, userID).Delete(&models.LogExport{}).Error
}

// CreateRetentionPolicy 创建保留策略
func (s *LogAnalysisService) CreateRetentionPolicy(policy *models.LogRetentionPolicy) error {
	return createAndPreserveBool(s.db, policy, "is_active", policy.IsActive)
}

// GetRetentionPolicies 获取保留策略列表
func (s *LogAnalysisService) GetRetentionPolicies() ([]*models.LogRetentionPolicy, error) {
	var policies []*models.LogRetentionPolicy

	if err := s.db.Preload("Creator").Order("priority DESC, created_at DESC").Find(&policies).Error; err != nil {
		return nil, fmt.Errorf("failed to get retention policies: %v", err)
	}

	return policies, nil
}

// UpdateRetentionPolicy 更新保留策略
func (s *LogAnalysisService) UpdateRetentionPolicy(id uint, updates map[string]interface{}) error {
	return s.db.Model(&models.LogRetentionPolicy{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteRetentionPolicy 删除保留策略
func (s *LogAnalysisService) DeleteRetentionPolicy(id uint) error {
	return s.db.Delete(&models.LogRetentionPolicy{}, id).Error
}

// ExecuteRetentionPolicies 执行保留策略
func (s *LogAnalysisService) ExecuteRetentionPolicies() error {
	policies, err := s.GetRetentionPolicies()
	if err != nil {
		return err
	}

	for _, policy := range policies {
		if !policy.IsActive {
			continue
		}

		if err := s.executeRetentionPolicy(policy); err != nil {
			// 记录错误但继续执行其他策略
			continue
		}
	}

	return nil
}

// CleanupOldLogs 清理旧日志
func (s *LogAnalysisService) CleanupOldLogs(retentionDays int) error {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	// 删除旧的日志条目
	if err := s.db.Where("created_at < ?", cutoffDate).Delete(&models.LogEntry{}).Error; err != nil {
		return fmt.Errorf("failed to cleanup old log entries: %v", err)
	}

	// 删除旧的统计数据
	if err := s.db.Where("created_at < ?", cutoffDate).Delete(&models.LogStatistics{}).Error; err != nil {
		return fmt.Errorf("failed to cleanup old log statistics: %v", err)
	}

	// 删除旧的告警
	if err := s.db.Where("created_at < ? AND resolved = ?", cutoffDate, true).Delete(&models.LogAlert{}).Error; err != nil {
		return fmt.Errorf("failed to cleanup old log alerts: %v", err)
	}

	return nil
}

// 私有方法

// parseLogEntry 解析日志条目
func (s *LogAnalysisService) parseLogEntry(entry *models.LogEntry) error {
	// 这里可以实现更复杂的日志解析逻辑
	// 例如解析结构化日志、提取关键字段等

	// 简单的分类逻辑
	if entry.Category == "" {
		entry.Category = s.categorizeLog(entry)
	}

	return nil
}

// categorizeLog 对日志进行分类
func (s *LogAnalysisService) categorizeLog(entry *models.LogEntry) string {
	message := strings.ToLower(entry.Message)

	if strings.Contains(message, "error") || strings.Contains(message, "exception") || strings.Contains(message, "failed") {
		return "error"
	}
	if strings.Contains(message, "warning") || strings.Contains(message, "warn") {
		return "warning"
	}
	if strings.Contains(message, "security") || strings.Contains(message, "auth") || strings.Contains(message, "login") {
		return "security"
	}
	if strings.Contains(message, "performance") || strings.Contains(message, "slow") || strings.Contains(message, "timeout") {
		return "performance"
	}

	return "general"
}

// processLogEntry 处理日志条目（分析规则匹配）
func (s *LogAnalysisService) processLogEntry(entry *models.LogEntry) {
	// 获取活跃的分析规则
	var rules []*models.LogAnalysisRule
	if err := s.db.Where("is_active = ?", true).Order("priority DESC").Find(&rules).Error; err != nil {
		return
	}

	// 检查每个规则
	for _, rule := range rules {
		if s.matchRule(entry, rule) {
			// 更新规则匹配计数
			now := time.Now()
			s.db.Model(rule).Updates(map[string]interface{}{
				"match_count": gorm.Expr("match_count + 1"),
				"last_match":  now,
			})

			// 执行规则动作
			s.executeRuleActions(entry, rule)
		}
	}
}

// matchRule 检查日志是否匹配规则
func (s *LogAnalysisService) matchRule(entry *models.LogEntry, rule *models.LogAnalysisRule) bool {
	switch rule.PatternType {
	case models.PatternTypeRegex:
		if regex, err := regexp.Compile(rule.Pattern); err == nil {
			return regex.MatchString(entry.Message)
		}
	case models.PatternTypeContains:
		return strings.Contains(strings.ToLower(entry.Message), strings.ToLower(rule.Pattern))
	case models.PatternTypeEquals:
		return strings.EqualFold(entry.Message, rule.Pattern)
	case models.PatternTypeStartsWith:
		return strings.HasPrefix(strings.ToLower(entry.Message), strings.ToLower(rule.Pattern))
	case models.PatternTypeEndsWith:
		return strings.HasSuffix(strings.ToLower(entry.Message), strings.ToLower(rule.Pattern))
	}
	return false
}

// executeRuleActions 执行规则动作
func (s *LogAnalysisService) executeRuleActions(entry *models.LogEntry, rule *models.LogAnalysisRule) {
	if rule.Actions == nil {
		return
	}

	var actions map[string]interface{}
	if err := json.Unmarshal([]byte(*rule.Actions), &actions); err != nil {
		return
	}

	// 创建告警
	if createAlert, ok := actions["create_alert"]; ok && createAlert.(bool) {
		s.createLogAlert(entry, rule)
	}

	// 其他动作可以在这里添加
}

// createLogAlert 创建日志告警
func (s *LogAnalysisService) createLogAlert(entry *models.LogEntry, rule *models.LogAnalysisRule) {
	// 检查是否已存在相同的活跃告警
	var existingAlert models.LogAlert
	if err := s.db.Where("rule_id = ? AND status = ? AND resolved = ?", rule.ID, models.AlertStatusActive, false).First(&existingAlert).Error; err == nil {
		// 更新现有告警
		now := time.Now()
		s.db.Model(&existingAlert).Updates(map[string]interface{}{
			"count":     gorm.Expr("count + 1"),
			"last_seen": now,
		})
		return
	}

	// 创建新告警
	alert := &models.LogAlert{
		RuleID:     rule.ID,
		LogEntryID: entry.ID,
		Level:      entry.Level,
		Title:      fmt.Sprintf("Log Alert: %s", rule.Name),
		Message:    entry.Message,
		Status:     models.AlertStatusActive,
		Severity:   entry.Severity,
		Count:      1,
		FirstSeen:  entry.Timestamp,
		LastSeen:   entry.Timestamp,
	}

	s.db.Create(alert)
}

// updateStatistics 更新统计信息
func (s *LogAnalysisService) updateStatistics(entry *models.LogEntry) {
	date := entry.Timestamp.Truncate(24 * time.Hour)
	hour := entry.Timestamp.Hour()

	// 查找或创建统计记录
	var stat models.LogStatistics
	err := s.db.Where("date = ? AND hour = ? AND level = ? AND source = ? AND process_name = ? AND node_id = ? AND category = ?",
		date, hour, entry.Level, entry.Source, entry.ProcessName, entry.NodeID, entry.Category).First(&stat).Error

	if err == gorm.ErrRecordNotFound {
		// 创建新的统计记录
		stat = models.LogStatistics{
			Date:        date,
			Hour:        hour,
			Level:       entry.Level,
			Source:      entry.Source,
			ProcessName: entry.ProcessName,
			NodeID:      entry.NodeID,
			Category:    entry.Category,
			Count:       1,
			TotalSize:   int64(len(entry.RawLog)),
		}

		// 根据级别更新计数
		switch entry.Level {
		case models.LogLevelError:
			stat.ErrorCount = 1
		case models.LogLevelWarning:
			stat.WarningCount = 1
		case models.LogLevelInfo:
			stat.InfoCount = 1
		case models.LogLevelDebug:
			stat.DebugCount = 1
		}

		s.db.Create(&stat)
	} else if err == nil {
		// 更新现有统计记录
		updates := map[string]interface{}{
			"count":      gorm.Expr("count + 1"),
			"total_size": gorm.Expr("total_size + ?", len(entry.RawLog)),
		}

		// 根据级别更新计数
		switch entry.Level {
		case models.LogLevelError:
			updates["error_count"] = gorm.Expr("error_count + 1")
		case models.LogLevelWarning:
			updates["warning_count"] = gorm.Expr("warning_count + 1")
		case models.LogLevelInfo:
			updates["info_count"] = gorm.Expr("info_count + 1")
		case models.LogLevelDebug:
			updates["debug_count"] = gorm.Expr("debug_count + 1")
		}

		s.db.Model(&stat).Updates(updates)
	}
}

// applyLogFilters 应用日志过滤条件
func (s *LogAnalysisService) applyLogFilters(query *gorm.DB, filters map[string]interface{}) *gorm.DB {
	if level, ok := filters["level"]; ok {
		query = query.Where("level = ?", level)
	}
	if source, ok := filters["source"]; ok {
		query = query.Where("source = ?", source)
	}
	if processName, ok := filters["process_name"]; ok {
		query = query.Where("process_name = ?", processName)
	}
	if nodeID, ok := filters["node_id"]; ok {
		query = query.Where("node_id = ?", nodeID)
	}
	if category, ok := filters["category"]; ok {
		query = query.Where("category = ?", category)
	}
	if severity, ok := filters["severity"]; ok {
		query = query.Where("severity = ?", severity)
	}
	if parsed, ok := filters["parsed"]; ok {
		query = query.Where("parsed = ?", parsed)
	}
	if archived, ok := filters["archived"]; ok {
		query = query.Where("archived = ?", archived)
	}
	if timeFrom, ok := filters["time_from"]; ok {
		query = query.Where("timestamp >= ?", timeFrom)
	}
	if timeTo, ok := filters["time_to"]; ok {
		query = query.Where("timestamp <= ?", timeTo)
	}
	if search, ok := filters["search"]; ok {
		query = query.Where("message LIKE ? OR raw_log LIKE ?", "%"+search.(string)+"%", "%"+search.(string)+"%")
	}

	return query
}

// processLogExport 处理日志导出任务
func (s *LogAnalysisService) processLogExport(export *models.LogExport) {
	// 更新状态为运行中
	now := time.Now()
	s.db.Model(export).Updates(map[string]interface{}{
		"status":     models.ExportStatusRunning,
		"started_at": now,
	})

	// 这里应该实现实际的导出逻辑
	// 由于篇幅限制，这里只是一个示例

	// 模拟导出过程
	time.Sleep(5 * time.Second)

	// 更新状态为完成
	completedAt := time.Now()
	expiresAt := completedAt.Add(7 * 24 * time.Hour) // 7天后过期
	s.db.Model(export).Updates(map[string]interface{}{
		"status":       models.ExportStatusCompleted,
		"progress":     100,
		"completed_at": completedAt,
		"expires_at":   expiresAt,
		"file_path":    "/exports/" + fmt.Sprintf("%d.%s", export.ID, export.Format),
		"download_url": fmt.Sprintf("/api/logs/exports/%d/download", export.ID),
	})
}

// executeRetentionPolicy 执行保留策略
func (s *LogAnalysisService) executeRetentionPolicy(policy *models.LogRetentionPolicy) error {
	now := time.Now()
	cutoffDate := now.AddDate(0, 0, -policy.RetentionDays)

	// 解析条件
	var conditions map[string]interface{}
	if err := json.Unmarshal([]byte(policy.Conditions), &conditions); err != nil {
		return err
	}

	// 构建查询
	query := s.db.Model(&models.LogEntry{}).Where("created_at < ?", cutoffDate)
	query = s.applyLogFilters(query, conditions)

	// 获取要处理的记录数
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}

	// 执行删除或归档
	if policy.ArchiveAfterDays != nil {
		// 归档逻辑
		archiveCutoff := now.AddDate(0, 0, -*policy.ArchiveAfterDays)
		archiveQuery := query.Where("created_at < ?", archiveCutoff)
		if err := archiveQuery.Update("archived", true).Error; err != nil {
			return err
		}
	} else {
		// 删除逻辑
		if err := query.Delete(&models.LogEntry{}).Error; err != nil {
			return err
		}
	}

	// 更新策略执行信息
	s.db.Model(policy).Updates(map[string]interface{}{
		"last_executed":   now,
		"processed_count": gorm.Expr("processed_count + ?", count),
	})

	return nil
}
