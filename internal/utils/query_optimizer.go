package utils

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// QueryOptimizer 数据库查询优化器
type QueryOptimizer struct {
	db *gorm.DB
}

// NewQueryOptimizer 创建新的查询优化器
func NewQueryOptimizer(db *gorm.DB) *QueryOptimizer {
	return &QueryOptimizer{db: db}
}

// PaginationConfig 分页配置
type PaginationConfig struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Offset   int `json:"offset"`
	Limit    int `json:"limit"`
}

// NewPaginationConfig 创建分页配置
func NewPaginationConfig(page, pageSize int) *PaginationConfig {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	return &PaginationConfig{
		Page:     page,
		PageSize: pageSize,
		Offset:   offset,
		Limit:    pageSize,
	}
}

// ApplyPagination 应用分页到查询
func (qo *QueryOptimizer) ApplyPagination(query *gorm.DB, config *PaginationConfig) *gorm.DB {
	return query.Offset(config.Offset).Limit(config.Limit)
}

// OptimizeLogQuery 优化日志查询
func (qo *QueryOptimizer) OptimizeLogQuery(query *gorm.DB, filters map[string]interface{}) *gorm.DB {
	// 使用索引友好的查询顺序
	if timestamp, ok := filters["timestamp"]; ok {
		query = query.Where("timestamp >= ?", timestamp)
	}

	if level, ok := filters["level"]; ok {
		query = query.Where("level = ?", level)
	}

	if nodeID, ok := filters["node_id"]; ok {
		query = query.Where("node_id = ?", nodeID)
	}

	if source, ok := filters["source"]; ok {
		query = query.Where("source = ?", source)
	}

	if processName, ok := filters["process_name"]; ok {
		query = query.Where("process_name = ?", processName)
	}

	if category, ok := filters["category"]; ok {
		query = query.Where("category = ?", category)
	}

	if severity, ok := filters["severity"]; ok {
		query = query.Where("severity = ?", severity)
	}

	// 排除已删除的记录
	query = query.Where("deleted_at IS NULL")

	// 默认按时间倒序排列，利用索引
	return query.Order("timestamp DESC")
}

// OptimizeConfigQuery 优化配置查询
func (qo *QueryOptimizer) OptimizeConfigQuery(query *gorm.DB, filters map[string]interface{}) *gorm.DB {
	// 使用索引友好的查询顺序
	if scope, ok := filters["scope"]; ok {
		query = query.Where("scope = ?", scope)
	}

	if category, ok := filters["category"]; ok {
		query = query.Where("category = ?", category)
	}

	if nodeID, ok := filters["node_id"]; ok {
		query = query.Where("node_id = ?", nodeID)
	}

	if userID, ok := filters["user_id"]; ok {
		query = query.Where("user_id = ?", userID)
	}

	if configType, ok := filters["type"]; ok {
		query = query.Where("type = ?", configType)
	}

	// 排除已删除的记录
	query = query.Where("deleted_at IS NULL")

	// 默认按创建时间倒序排列
	return query.Order("created_at DESC")
}

// OptimizeActivityLogQuery 优化活动日志查询
func (qo *QueryOptimizer) OptimizeActivityLogQuery(query *gorm.DB, filters map[string]interface{}) *gorm.DB {
	// 使用索引友好的查询顺序
	if username, ok := filters["username"]; ok {
		query = query.Where("username = ?", username)
	}

	if userID, ok := filters["user_id"]; ok {
		query = query.Where("user_id = ?", userID)
	}

	if action, ok := filters["action"]; ok {
		query = query.Where("action = ?", action)
	}

	if level, ok := filters["level"]; ok {
		query = query.Where("level = ?", level)
	}

	if resource, ok := filters["resource"]; ok {
		query = query.Where("resource = ?", resource)
	}

	if status, ok := filters["status"]; ok {
		query = query.Where("status = ?", status)
	}

	if createdAt, ok := filters["created_at"]; ok {
		query = query.Where("created_at >= ?", createdAt)
	}

	// 排除已删除的记录
	query = query.Where("deleted_at IS NULL")

	// 默认按创建时间倒序排列
	return query.Order("created_at DESC")
}

// OptimizeSearchQuery 优化搜索查询
func (qo *QueryOptimizer) OptimizeSearchQuery(query *gorm.DB, searchTerm string, searchFields []string) *gorm.DB {
	if searchTerm == "" || len(searchFields) == 0 {
		return query
	}

	// 构建搜索条件
	searchTerm = strings.TrimSpace(searchTerm)
	if searchTerm == "" {
		return query
	}

	// 使用LIKE查询进行模糊搜索
	searchPattern := fmt.Sprintf("%%%s%%", searchTerm)
	conditions := make([]string, len(searchFields))
	args := make([]interface{}, len(searchFields))

	for i, field := range searchFields {
		conditions[i] = fmt.Sprintf("%s LIKE ?", field)
		args[i] = searchPattern
	}

	searchCondition := strings.Join(conditions, " OR ")
	return query.Where(searchCondition, args...)
}

// GetTotalCount 获取总记录数（用于分页）
func (qo *QueryOptimizer) GetTotalCount(query *gorm.DB) (int64, error) {
	var count int64
	// 移除ORDER BY和LIMIT子句以提高计数性能
	countQuery := query.Session(&gorm.Session{}).Select("COUNT(*)")
	err := countQuery.Count(&count).Error
	return count, err
}

// BatchInsert 批量插入优化
func (qo *QueryOptimizer) BatchInsert(records interface{}, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 100
	}
	return qo.db.CreateInBatches(records, batchSize).Error
}

// BatchUpdate 批量更新优化
func (qo *QueryOptimizer) BatchUpdate(model interface{}, updates map[string]interface{}, whereCondition string, args ...interface{}) error {
	query := qo.db.Model(model)
	if whereCondition != "" {
		query = query.Where(whereCondition, args...)
	}
	return query.Updates(updates).Error
}

// OptimizeQuery 通用查询优化
func (qo *QueryOptimizer) OptimizeQuery(query *gorm.DB, queryType string, filters map[string]interface{}) *gorm.DB {
	switch queryType {
	case "log":
		return qo.OptimizeLogQuery(query, filters)
	case "config":
		return qo.OptimizeConfigQuery(query, filters)
	case "activity_log":
		return qo.OptimizeActivityLogQuery(query, filters)
	default:
		// 默认优化：排除已删除记录，按创建时间排序
		query = query.Where("deleted_at IS NULL")
		return query.Order("created_at DESC")
	}
}