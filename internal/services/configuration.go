package services

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"superview/internal/models"
)

// ConfigurationService 配置管理服务
type ConfigurationService struct {
	db *gorm.DB
}

// NewConfigurationService 创建配置管理服务实例
func NewConfigurationService(db *gorm.DB) *ConfigurationService {
	return &ConfigurationService{db: db}
}

// CreateConfiguration 创建配置项
func (s *ConfigurationService) CreateConfiguration(config *models.Configuration) error {
	// 检查配置键是否已存在
	var existing models.Configuration
	err := s.db.Where("key = ? AND scope = ? AND COALESCE(node_id, 0) = COALESCE(?, 0) AND COALESCE(user_id, '') = COALESCE(?, '')",
		config.Key, config.Scope, config.NodeID, config.UserID).First(&existing).Error
	if err == nil {
		return fmt.Errorf("configuration key already exists in this scope")
	}

	// 创建审计日志
	s.createAuditLog("create", "configuration", 0, config.Key, nil, config.CreatedBy, "", "", true, nil)

	return s.db.Create(config).Error
}

// GetConfigurations 获取配置列表
func (s *ConfigurationService) GetConfigurations(page, pageSize int, filters map[string]interface{}) ([]models.Configuration, int64, error) {
	var configs []models.Configuration
	var total int64

	query := s.db.Model(&models.Configuration{}).Preload("User").Preload("Updater").Preload("Node").Preload("Owner")

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "category":
			query = query.Where("category = ?", value)
		case "scope":
			query = query.Where("scope = ?", value)
		case "type":
			query = query.Where("type = ?", value)
		case "node_id":
			query = query.Where("node_id = ?", value)
		case "user_id":
			query = query.Where("user_id = ?", value)
		case "is_required":
			query = query.Where("is_required = ?", value)
		case "is_readonly":
			query = query.Where("is_readonly = ?", value)
		case "is_secret":
			query = query.Where("is_secret = ?", value)
		case "search":
			query = query.Where("key LIKE ? OR description LIKE ?", "%"+value.(string)+"%", "%"+value.(string)+"%")
		}
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("category, `order`, key").Find(&configs).Error

	// 对于敏感配置，隐藏值
	for i := range configs {
		if configs[i].IsSecret {
			configs[i].Value = "********"
		}
	}

	return configs, total, err
}

// GetConfigurationByID 根据ID获取配置
func (s *ConfigurationService) GetConfigurationByID(id uint, userID string, showSecret bool) (*models.Configuration, error) {
	var config models.Configuration
	err := s.db.Preload("User").Preload("Updater").Preload("Node").Preload("Owner").First(&config, id).Error
	if err != nil {
		return nil, err
	}

	// 创建审计日志
	s.createAuditLog("view", "configuration", config.ID, config.Key, nil, userID, "", "", true, nil)

	// 对于敏感配置，根据权限决定是否隐藏值
	if config.IsSecret && !showSecret {
		config.Value = "********"
	}

	return &config, nil
}

// GetConfigurationByKey 根据键获取配置
func (s *ConfigurationService) GetConfigurationByKey(key string, scope string, nodeID *uint, userID *string) (*models.Configuration, error) {
	var config models.Configuration
	query := s.db.Where("key = ? AND scope = ?", key, scope)

	if nodeID != nil {
		query = query.Where("node_id = ?", *nodeID)
	} else {
		query = query.Where("node_id IS NULL")
	}

	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	} else {
		query = query.Where("user_id IS NULL")
	}

	err := query.First(&config).Error
	return &config, err
}

// UpdateConfiguration 更新配置
func (s *ConfigurationService) UpdateConfiguration(id uint, updates map[string]interface{}, userID string) error {
	// 获取原配置
	var oldConfig models.Configuration
	err := s.db.First(&oldConfig, id).Error
	if err != nil {
		return err
	}

	// 检查是否为只读配置
	if oldConfig.IsReadonly {
		return fmt.Errorf("configuration is readonly")
	}

	// 记录变更历史
	for field, newValue := range updates {
		if field == "updated_at" || field == "updated_by" {
			continue
		}

		var oldValue interface{}
		switch field {
		case "value":
			oldValue = oldConfig.Value
		case "description":
			oldValue = oldConfig.Description
		case "category":
			oldValue = oldConfig.Category
		case "type":
			oldValue = oldConfig.Type
		case "is_required":
			oldValue = oldConfig.IsRequired
		case "is_readonly":
			oldValue = oldConfig.IsReadonly
		case "is_secret":
			oldValue = oldConfig.IsSecret
		case "validation":
			oldValue = oldConfig.Validation
		case "options":
			oldValue = oldConfig.Options
		case "order":
			oldValue = oldConfig.Order
		}

		if oldValue != newValue {
			s.createConfigHistory(&oldConfig.ID, nil, models.ChangeTypeUpdate, field, oldValue, newValue, userID)
		}
	}

	// 添加更新信息
	updates["updated_at"] = time.Now()
	updates["updated_by"] = userID

	// 创建审计日志
	s.createAuditLog("update", "configuration", oldConfig.ID, oldConfig.Key, updates, userID, "", "", true, nil)

	return s.db.Model(&models.Configuration{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteConfiguration 删除配置
func (s *ConfigurationService) DeleteConfiguration(id uint, userID string) error {
	// 获取配置信息
	var config models.Configuration
	err := s.db.First(&config, id).Error
	if err != nil {
		return err
	}

	// 检查是否为必需配置
	if config.IsRequired {
		return fmt.Errorf("required configuration cannot be deleted")
	}

	// 记录变更历史
	s.createConfigHistory(&config.ID, nil, models.ChangeTypeDelete, "entire_record", config, nil, userID)

	// 创建审计日志
	s.createAuditLog("delete", "configuration", config.ID, config.Key, nil, userID, "", "", true, nil)

	// 删除相关的变更历史
	s.db.Where("config_id = ?", id).Delete(&models.ConfigurationHistory{})

	return s.db.Delete(&models.Configuration{}, id).Error
}

// CreateEnvironmentVariable 创建环境变量
func (s *ConfigurationService) CreateEnvironmentVariable(envVar *models.EnvironmentVariable) error {
	// 检查环境变量名是否已存在
	var existing models.EnvironmentVariable
	query := s.db.Where("name = ?", envVar.Name)
	if envVar.NodeID != nil {
		query = query.Where("node_id = ?", *envVar.NodeID)
	} else {
		query = query.Where("node_id IS NULL")
	}
	if envVar.ProcessName != nil {
		query = query.Where("process_name = ?", *envVar.ProcessName)
	} else {
		query = query.Where("process_name IS NULL")
	}

	err := query.First(&existing).Error
	if err == nil {
		return fmt.Errorf("environment variable already exists")
	}

	// 创建审计日志
	s.createAuditLog("create", "environment_variable", 0, envVar.Name, nil, envVar.CreatedBy, "", "", true, nil)

	return createAndPreserveBool(s.db, envVar, "is_active", envVar.IsActive)
}

// GetEnvironmentVariables 获取环境变量列表
func (s *ConfigurationService) GetEnvironmentVariables(page, pageSize int, filters map[string]interface{}) ([]models.EnvironmentVariable, int64, error) {
	var envVars []models.EnvironmentVariable
	var total int64

	query := s.db.Model(&models.EnvironmentVariable{}).Preload("User").Preload("Updater").Preload("Node")

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "node_id":
			query = query.Where("node_id = ?", value)
		case "process_name":
			query = query.Where("process_name = ?", value)
		case "is_secret":
			query = query.Where("is_secret = ?", value)
		case "is_active":
			query = query.Where("is_active = ?", value)
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
	err = query.Offset(offset).Limit(pageSize).Order("name").Find(&envVars).Error

	// 对于敏感环境变量，隐藏值
	for i := range envVars {
		if envVars[i].IsSecret {
			envVars[i].Value = "********"
		}
	}

	return envVars, total, err
}

// GetEnvironmentVariableByID 根据ID获取环境变量
func (s *ConfigurationService) GetEnvironmentVariableByID(id uint, userID string, showSecret bool) (*models.EnvironmentVariable, error) {
	var envVar models.EnvironmentVariable
	err := s.db.Preload("User").Preload("Updater").Preload("Node").First(&envVar, id).Error
	if err != nil {
		return nil, err
	}

	// 创建审计日志
	s.createAuditLog("view", "environment_variable", envVar.ID, envVar.Name, nil, userID, "", "", true, nil)

	// 对于敏感环境变量，根据权限决定是否隐藏值
	if envVar.IsSecret && !showSecret {
		envVar.Value = "********"
	}

	return &envVar, nil
}

// UpdateEnvironmentVariable 更新环境变量
func (s *ConfigurationService) UpdateEnvironmentVariable(id uint, updates map[string]interface{}, userID string) error {
	// 获取原环境变量
	var oldEnvVar models.EnvironmentVariable
	err := s.db.First(&oldEnvVar, id).Error
	if err != nil {
		return err
	}

	// 记录变更历史
	for field, newValue := range updates {
		if field == "updated_at" || field == "updated_by" {
			continue
		}

		var oldValue interface{}
		switch field {
		case "value":
			oldValue = oldEnvVar.Value
		case "description":
			oldValue = oldEnvVar.Description
		case "is_secret":
			oldValue = oldEnvVar.IsSecret
		case "is_active":
			oldValue = oldEnvVar.IsActive
		}

		if oldValue != newValue {
			s.createConfigHistory(nil, &oldEnvVar.ID, models.ChangeTypeUpdate, field, oldValue, newValue, userID)
		}
	}

	// 添加更新信息
	updates["updated_at"] = time.Now()
	updates["updated_by"] = userID

	// 创建审计日志
	s.createAuditLog("update", "environment_variable", oldEnvVar.ID, oldEnvVar.Name, updates, userID, "", "", true, nil)

	return s.db.Model(&models.EnvironmentVariable{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteEnvironmentVariable 删除环境变量
func (s *ConfigurationService) DeleteEnvironmentVariable(id uint, userID string) error {
	// 获取环境变量信息
	var envVar models.EnvironmentVariable
	err := s.db.First(&envVar, id).Error
	if err != nil {
		return err
	}

	// 记录变更历史
	s.createConfigHistory(nil, &envVar.ID, models.ChangeTypeDelete, "entire_record", envVar, nil, userID)

	// 创建审计日志
	s.createAuditLog("delete", "environment_variable", envVar.ID, envVar.Name, nil, userID, "", "", true, nil)

	// 删除相关的变更历史
	s.db.Where("env_var_id = ?", id).Delete(&models.ConfigurationHistory{})

	return s.db.Delete(&models.EnvironmentVariable{}, id).Error
}

// ExportConfigurations 导出配置
func (s *ConfigurationService) ExportConfigurations(filters map[string]interface{}, userID string) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	// 导出配置
	var configs []models.Configuration
	configQuery := s.db.Model(&models.Configuration{})
	for key, value := range filters {
		switch key {
		case "category":
			configQuery = configQuery.Where("category = ?", value)
		case "scope":
			configQuery = configQuery.Where("scope = ?", value)
		case "node_id":
			configQuery = configQuery.Where("node_id = ?", value)
		case "user_id":
			configQuery = configQuery.Where("user_id = ?", value)
		}
	}
	configQuery.Find(&configs)
	data["configurations"] = configs

	// 导出环境变量
	var envVars []models.EnvironmentVariable
	envQuery := s.db.Model(&models.EnvironmentVariable{})
	for key, value := range filters {
		switch key {
		case "node_id":
			envQuery = envQuery.Where("node_id = ?", value)
		case "process_name":
			envQuery = envQuery.Where("process_name = ?", value)
		}
	}
	envQuery.Find(&envVars)
	data["environment_variables"] = envVars

	// 添加元数据
	data["export_time"] = time.Now()
	data["exported_by"] = userID
	data["version"] = "1.0"

	// 创建审计日志
	s.createAuditLog("export", "configuration", 0, "bulk_export", filters, userID, "", "", true, nil)

	return data, nil
}

// ImportConfigurations 导入配置
func (s *ConfigurationService) ImportConfigurations(data map[string]interface{}, userID string, options map[string]interface{}) (err error) {
	// 开始事务(M-15: 所有 Create/Updates 错误均导致回滚)
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			if err == nil {
				err = fmt.Errorf("recovered from panic during import: %v", r)
			}
		}
	}()

	overwriteExisting := false
	if val, exists := options["overwrite_existing"]; exists {
		overwriteExisting = val.(bool)
	}

	// 导入配置
	if configs, exists := data["configurations"]; exists {
		configList := configs.([]interface{})
		for _, configData := range configList {
			configBytes, _ := json.Marshal(configData)
			var config models.Configuration
			json.Unmarshal(configBytes, &config)

			// 重置ID和时间戳
			config.ID = 0
			config.CreatedAt = time.Now()
			config.UpdatedAt = time.Now()
			config.CreatedBy = userID
			config.UpdatedBy = nil

			// 检查是否已存在
			var existing models.Configuration
			existErr := tx.Where("key = ? AND scope = ? AND COALESCE(node_id, 0) = COALESCE(?, 0) AND COALESCE(user_id, '') = COALESCE(?, '')",
				config.Key, config.Scope, config.NodeID, config.UserID).First(&existing).Error
			if existErr == nil {
				if overwriteExisting {
					// 更新现有配置(M-15: 检查错误)
					if err := tx.Model(&existing).Updates(map[string]interface{}{
						"value":       config.Value,
						"description": config.Description,
						"updated_at":  time.Now(),
						"updated_by":  userID,
					}).Error; err != nil {
						tx.Rollback()
						return fmt.Errorf("failed to update configuration %s: %w", config.Key, err)
					}
				}
				// 如果不覆盖,跳过已存在的配置
			} else {
				// 创建新配置(M-15: 检查错误)
				if err := tx.Create(&config).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to create configuration %s: %w", config.Key, err)
				}
			}
		}
	}

	// 导入环境变量
	if envVars, exists := data["environment_variables"]; exists {
		envVarList := envVars.([]interface{})
		for _, envVarData := range envVarList {
			envVarBytes, _ := json.Marshal(envVarData)
			var envVar models.EnvironmentVariable
			json.Unmarshal(envVarBytes, &envVar)

			// 重置ID和时间戳
			envVar.ID = 0
			envVar.CreatedAt = time.Now()
			envVar.UpdatedAt = time.Now()
			envVar.CreatedBy = userID
			envVar.UpdatedBy = nil

			// 检查是否已存在
			var existing models.EnvironmentVariable
			query := tx.Where("name = ?", envVar.Name)
			if envVar.NodeID != nil {
				query = query.Where("node_id = ?", *envVar.NodeID)
			} else {
				query = query.Where("node_id IS NULL")
			}
			if envVar.ProcessName != nil {
				query = query.Where("process_name = ?", *envVar.ProcessName)
			} else {
				query = query.Where("process_name IS NULL")
			}

			existErr := query.First(&existing).Error
			if existErr == nil {
				if overwriteExisting {
					// 更新现有环境变量(M-15: 检查错误)
					if err := tx.Model(&existing).Updates(map[string]interface{}{
						"value":       envVar.Value,
						"description": envVar.Description,
						"is_secret":   envVar.IsSecret,
						"is_active":   envVar.IsActive,
						"updated_at":  time.Now(),
						"updated_by":  userID,
					}).Error; err != nil {
						tx.Rollback()
						return fmt.Errorf("failed to update environment variable %s: %w", envVar.Name, err)
					}
				}
			} else {
				// 创建新环境变量(M-15: 检查错误)
				if err := tx.Create(&envVar).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to create environment variable %s: %w", envVar.Name, err)
				}
			}
		}
	}

	// 提交事务
	err = tx.Commit().Error
	if err != nil {
		return err
	}

	// 创建审计日志
	s.createAuditLog("import", "configuration", 0, "bulk_import", options, userID, "", "", true, nil)

	return nil
}

// GetConfigurationHistory 获取配置变更历史
func (s *ConfigurationService) GetConfigurationHistory(configID *uint, envVarID *uint, page, pageSize int) ([]models.ConfigurationHistory, int64, error) {
	var history []models.ConfigurationHistory
	var total int64

	query := s.db.Model(&models.ConfigurationHistory{}).Preload("User").Preload("Configuration").Preload("EnvVar")

	if configID != nil {
		query = query.Where("config_id = ?", *configID)
	}
	if envVarID != nil {
		query = query.Where("env_var_id = ?", *envVarID)
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&history).Error

	return history, total, err
}

// GetAuditLogs 获取审计日志
func (s *ConfigurationService) GetAuditLogs(page, pageSize int, filters map[string]interface{}) ([]models.ConfigurationAudit, int64, error) {
	var audits []models.ConfigurationAudit
	var total int64

	query := s.db.Model(&models.ConfigurationAudit{}).Preload("User")

	// 应用过滤条件
	for key, value := range filters {
		switch key {
		case "action":
			query = query.Where("action = ?", value)
		case "resource_type":
			query = query.Where("resource_type = ?", value)
		case "resource_id":
			query = query.Where("resource_id = ?", value)
		case "created_by":
			query = query.Where("created_by = ?", value)
		case "success":
			query = query.Where("success = ?", value)
		case "date_from":
			query = query.Where("created_at >= ?", value)
		case "date_to":
			query = query.Where("created_at <= ?", value)
		case "search":
			query = query.Where("resource_name LIKE ? OR ip_address LIKE ?", "%"+value.(string)+"%", "%"+value.(string)+"%")
		}
	}

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&audits).Error

	return audits, total, err
}

// InitializeSystemConfigurations 初始化系统配置
func (s *ConfigurationService) InitializeSystemConfigurations() error {
	// 系统默认配置
	defaultConfigs := []models.Configuration{
		{
			Key:          "system.name",
			Value:        "Superview",
			DefaultValue: "Superview",
			Description:  "系统名称",
			Category:     models.ConfigCategorySystem,
			Type:         models.ConfigTypeString,
			Scope:        models.ConfigScopeGlobal,
			IsRequired:   true,
			Order:        1,
			CreatedBy:    "system",
		},
		{
			Key:          "system.version",
			Value:        "1.0.0",
			DefaultValue: "1.0.0",
			Description:  "系统版本",
			Category:     models.ConfigCategorySystem,
			Type:         models.ConfigTypeString,
			Scope:        models.ConfigScopeGlobal,
			IsRequired:   true,
			IsReadonly:   true,
			Order:        2,
			CreatedBy:    "system",
		},
		{
			Key:          "auth.session_timeout",
			Value:        "3600",
			DefaultValue: "3600",
			Description:  "会话超时时间（秒）",
			Category:     models.ConfigCategoryAuth,
			Type:         models.ConfigTypeNumber,
			Scope:        models.ConfigScopeGlobal,
			IsRequired:   true,
			Order:        1,
			CreatedBy:    "system",
		},
		{
			Key:          "logging.level",
			Value:        "info",
			DefaultValue: "info",
			Description:  "日志级别",
			Category:     models.ConfigCategoryLogging,
			Type:         models.ConfigTypeString,
			Scope:        models.ConfigScopeGlobal,
			IsRequired:   true,
			Order:        1,
			CreatedBy:    "system",
		},
	}

	for _, config := range defaultConfigs {
		// 检查配置是否已存在
		var existing models.Configuration
		err := s.db.Where("key = ? AND scope = ?", config.Key, config.Scope).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			// 配置不存在，创建新配置
			s.db.Create(&config)
		}
	}

	return nil
}

// createConfigHistory 创建配置变更历史记录
func (s *ConfigurationService) createConfigHistory(configID *uint, envVarID *uint, changeType, fieldName string, oldValue, newValue interface{}, userID string) {
	history := &models.ConfigurationHistory{
		ConfigID:   configID,
		EnvVarID:   envVarID,
		ChangeType: changeType,
		FieldName:  fieldName,
		CreatedBy:  userID,
	}

	if oldValue != nil {
		oldStr := fmt.Sprintf("%v", oldValue)
		history.OldValue = &oldStr
	}
	if newValue != nil {
		newStr := fmt.Sprintf("%v", newValue)
		history.NewValue = &newStr
	}

	s.db.Create(history)
}

// createAuditLog 创建审计日志
func (s *ConfigurationService) createAuditLog(action, resourceType string, resourceID uint, resourceName string, details interface{}, userID string, ipAddress, userAgent string, success bool, errorMsg *string) {
	audit := &models.ConfigurationAudit{
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Success:      success,
		ErrorMessage: errorMsg,
		CreatedBy:    userID,
	}

	if details != nil {
		audit.SetDetails(map[string]interface{}{"details": details})
	}

	s.db.Create(audit)
}

// CleanupOldAuditLogs 清理旧的审计日志
func (s *ConfigurationService) CleanupOldAuditLogs(retentionDays int) error {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	return s.db.Where("created_at < ?", cutoffTime).Delete(&models.ConfigurationAudit{}).Error
}

// CleanupOldHistory 清理旧的变更历史
func (s *ConfigurationService) CleanupOldHistory(retentionDays int) error {
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	return s.db.Where("created_at < ?", cutoffTime).Delete(&models.ConfigurationHistory{}).Error
}

