package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// ConfigurationCategory 配置分类常量
const (
	ConfigCategorySystem       = "system"
	ConfigCategoryDatabase     = "database"
	ConfigCategoryAuth         = "auth"
	ConfigCategoryNotification = "notification"
	ConfigCategoryMonitoring   = "monitoring"
	ConfigCategoryLogging      = "logging"
	ConfigCategoryCustom       = "custom"
)

// ConfigurationType 配置类型常量
const (
	ConfigTypeString  = "string"
	ConfigTypeNumber  = "number"
	ConfigTypeBoolean = "boolean"
	ConfigTypeJSON    = "json"
	ConfigTypeArray   = "array"
	ConfigTypeObject  = "object"
)

// ConfigurationScope 配置作用域常量
const (
	ConfigScopeGlobal = "global"
	ConfigScopeNode   = "node"
	ConfigScopeUser   = "user"
)

// ChangeType 变更类型常量
const (
	ChangeTypeCreate  = "create"
	ChangeTypeUpdate  = "update"
	ChangeTypeDelete  = "delete"
	ChangeTypeImport  = "import"
	ChangeTypeExport  = "export"
	ChangeTypeRestore = "restore"
)

// Configuration 系统配置模型
type Configuration struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Key          string    `json:"key" gorm:"uniqueIndex;not null"`
	Value        string    `json:"value" gorm:"type:text"`
	DefaultValue string    `json:"default_value" gorm:"type:text"`
	Description  string    `json:"description" gorm:"type:text"`
	Category     string    `json:"category" gorm:"not null;index"`
	Type         string    `json:"type" gorm:"not null;default:'string'"`
	Scope        string    `json:"scope" gorm:"not null;default:'global'"`
	NodeID       *uint     `json:"node_id" gorm:"index"`
	UserID       *string   `json:"user_id" gorm:"size:50;index"`
	IsRequired   bool      `json:"is_required" gorm:"default:false"`
	IsReadonly   bool      `json:"is_readonly" gorm:"default:false"`
	IsSecret     bool      `json:"is_secret" gorm:"default:false"`
	Validation   *string   `json:"validation" gorm:"type:text"` // JSON格式的验证规则
	Options      *string   `json:"options" gorm:"type:text"`    // JSON格式的选项列表
	Order        int       `json:"order" gorm:"default:0"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedBy    string    `json:"created_by" gorm:"size:50;not null;index"`
	UpdatedBy    *string   `json:"updated_by" gorm:"size:50;index"`

	// 关联
	User    *User `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
	Updater *User `json:"updater,omitempty" gorm:"foreignKey:UpdatedBy;references:ID"`
	Node    *Node `json:"node,omitempty" gorm:"foreignKey:NodeID"`
	Owner   *User `json:"owner,omitempty" gorm:"foreignKey:UserID;references:ID"`
}

// GetParsedValue 获取解析后的值
func (c *Configuration) GetParsedValue() (interface{}, error) {
	value := c.Value
	if value == "" {
		value = c.DefaultValue
	}

	switch c.Type {
	case ConfigTypeString:
		return value, nil
	case ConfigTypeNumber:
		var num float64
		err := json.Unmarshal([]byte(value), &num)
		return num, err
	case ConfigTypeBoolean:
		var b bool
		err := json.Unmarshal([]byte(value), &b)
		return b, err
	case ConfigTypeJSON, ConfigTypeArray, ConfigTypeObject:
		var result interface{}
		err := json.Unmarshal([]byte(value), &result)
		return result, err
	default:
		return value, nil
	}
}

// SetValue 设置值
func (c *Configuration) SetValue(value interface{}) error {
	switch c.Type {
	case ConfigTypeString:
		c.Value = value.(string)
	case ConfigTypeNumber, ConfigTypeBoolean, ConfigTypeJSON, ConfigTypeArray, ConfigTypeObject:
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		c.Value = string(data)
	default:
		c.Value = value.(string)
	}
	return nil
}

// GetValidationRules 获取验证规则
func (c *Configuration) GetValidationRules() (map[string]interface{}, error) {
	if c.Validation == nil {
		return nil, nil
	}
	var rules map[string]interface{}
	err := json.Unmarshal([]byte(*c.Validation), &rules)
	return rules, err
}

// GetOptions 获取选项列表
func (c *Configuration) GetOptions() ([]interface{}, error) {
	if c.Options == nil {
		return nil, nil
	}
	var options []interface{}
	err := json.Unmarshal([]byte(*c.Options), &options)
	return options, err
}

// EnvironmentVariable 环境变量模型
type EnvironmentVariable struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null;index"`
	Value       string    `json:"value" gorm:"type:text"`
	Description string    `json:"description" gorm:"type:text"`
	NodeID      *uint     `json:"node_id" gorm:"index"`
	ProcessName *string   `json:"process_name" gorm:"index"`
	IsSecret    bool      `json:"is_secret" gorm:"default:false"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by" gorm:"size:50;not null;index"`
	UpdatedBy   *string   `json:"updated_by" gorm:"size:50;index"`

	// 关联
	User    *User `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
	Updater *User `json:"updater,omitempty" gorm:"foreignKey:UpdatedBy;references:ID"`
	Node    *Node `json:"node,omitempty" gorm:"foreignKey:NodeID"`
}

// GetMaskedValue 获取掩码值（用于显示敏感信息）
func (e *EnvironmentVariable) GetMaskedValue() string {
	if e.IsSecret && e.Value != "" {
		return "********"
	}
	return e.Value
}

// ConfigurationHistory 配置变更历史模型
type ConfigurationHistory struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	ConfigID   *uint     `json:"config_id" gorm:"index"`
	EnvVarID   *uint     `json:"env_var_id" gorm:"index"`
	ChangeType string    `json:"change_type" gorm:"not null;index"`
	FieldName  string    `json:"field_name" gorm:"not null"`
	OldValue   *string   `json:"old_value" gorm:"type:text"`
	NewValue   *string   `json:"new_value" gorm:"type:text"`
	Reason     string    `json:"reason" gorm:"type:text"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  string    `json:"created_by" gorm:"size:50;not null;index"`

	// 关联
	User          *User                `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
	Configuration *Configuration       `json:"configuration,omitempty" gorm:"foreignKey:ConfigID"`
	EnvVar        *EnvironmentVariable `json:"env_var,omitempty" gorm:"foreignKey:EnvVarID"`
}

// ConfigurationTemplate 配置模板模型
type ConfigurationTemplate struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null;uniqueIndex"`
	Description string    `json:"description" gorm:"type:text"`
	Category    string    `json:"category" gorm:"not null;index"`
	Template    string    `json:"template" gorm:"type:longtext;not null"` // JSON格式的模板数据
	IsPublic    bool      `json:"is_public" gorm:"default:false"`
	UsageCount  int       `json:"usage_count" gorm:"default:0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by" gorm:"size:50;not null;index"`
	UpdatedBy   *string   `json:"updated_by" gorm:"size:50;index"`

	// 关联
	User    *User `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
	Updater *User `json:"updater,omitempty" gorm:"foreignKey:UpdatedBy;references:ID"`
}

// GetParsedTemplate 获取解析后的模板数据
func (c *ConfigurationTemplate) GetParsedTemplate() (map[string]interface{}, error) {
	var template map[string]interface{}
	err := json.Unmarshal([]byte(c.Template), &template)
	return template, err
}

// SetTemplate 设置模板数据
func (c *ConfigurationTemplate) SetTemplate(template map[string]interface{}) error {
	jsonData, err := json.Marshal(template)
	if err != nil {
		return err
	}
	c.Template = string(jsonData)
	return nil
}

// IncrementUsage 增加使用次数
func (c *ConfigurationTemplate) IncrementUsage() {
	c.UsageCount++
}

// ConfigurationValidation 配置验证规则模型
type ConfigurationValidation struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	ConfigKey      string    `json:"config_key" gorm:"not null;index"`
	ValidatorType  string    `json:"validator_type" gorm:"not null"` // required, min, max, pattern, enum, custom
	ValidatorValue string    `json:"validator_value" gorm:"type:text"`
	ErrorMessage   string    `json:"error_message" gorm:"type:text"`
	IsActive       bool      `json:"is_active" gorm:"default:true"`
	Order          int       `json:"order" gorm:"default:0"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedBy      string    `json:"created_by" gorm:"size:50;not null;index"`

	// 关联
	User *User `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
}

// ConfigurationAudit 配置审计日志模型
type ConfigurationAudit struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Action       string    `json:"action" gorm:"not null;index"`        // view, create, update, delete, export, import
	ResourceType string    `json:"resource_type" gorm:"not null;index"` // configuration, environment_variable, template
	ResourceID   uint      `json:"resource_id" gorm:"not null;index"`
	ResourceName string    `json:"resource_name" gorm:"not null"`
	Details      *string   `json:"details" gorm:"type:text"` // JSON格式的详细信息
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	Success      bool      `json:"success" gorm:"default:true"`
	ErrorMessage *string   `json:"error_message" gorm:"type:text"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by" gorm:"size:50;not null;index"`

	// 关联
	User *User `json:"user,omitempty" gorm:"foreignKey:CreatedBy;references:ID"`
}

// GetParsedDetails 获取解析后的详细信息
func (c *ConfigurationAudit) GetParsedDetails() (map[string]interface{}, error) {
	if c.Details == nil {
		return nil, nil
	}
	var details map[string]interface{}
	err := json.Unmarshal([]byte(*c.Details), &details)
	return details, err
}

// SetDetails 设置详细信息
func (c *ConfigurationAudit) SetDetails(details map[string]interface{}) error {
	jsonData, err := json.Marshal(details)
	if err != nil {
		return err
	}
	detailsStr := string(jsonData)
	c.Details = &detailsStr
	return nil
}

// Node 节点模型已在node.go中定义

// BeforeCreate 创建前钩子
func (c *Configuration) BeforeCreate(tx *gorm.DB) error {
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	return nil
}

// BeforeUpdate 更新前钩子
func (c *Configuration) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}

// BeforeCreate 创建前钩子
func (e *EnvironmentVariable) BeforeCreate(tx *gorm.DB) error {
	e.CreatedAt = time.Now()
	e.UpdatedAt = time.Now()
	return nil
}

// BeforeUpdate 更新前钩子
func (e *EnvironmentVariable) BeforeUpdate(tx *gorm.DB) error {
	e.UpdatedAt = time.Now()
	return nil
}
