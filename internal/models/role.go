package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

// Role 角色模型
type Role struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"uniqueIndex;size:50;not null" json:"name"`
	DisplayName string         `gorm:"size:100" json:"display_name"`
	Description string         `gorm:"size:255" json:"description"`
	IsSystem    bool           `gorm:"default:false" json:"is_system"` // 系统内置角色不可删除
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index:idx_role_deleted_at" json:"-"`

	// 关联关系
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions,omitempty"`
	Users       []User       `gorm:"many2many:user_roles;" json:"users,omitempty"`
}

// Permission 权限模型
type Permission struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"uniqueIndex;size:100;not null" json:"name"`
	DisplayName string         `gorm:"size:100" json:"display_name"`
	Description string         `gorm:"size:255" json:"description"`
	Resource    string         `gorm:"size:50" json:"resource"`        // 资源类型：node, process, user, system等
	Action      string         `gorm:"size:50" json:"action"`          // 操作类型：read, write, delete, execute等
	IsSystem    bool           `gorm:"default:false" json:"is_system"` // 系统内置权限不可删除
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index:idx_permission_deleted_at" json:"-"`

	// 关联关系
	Roles []Role `gorm:"many2many:role_permissions;" json:"roles,omitempty"`
}

// UserRole 用户角色关联
type UserRole struct {
	UserID    string    `gorm:"primaryKey" json:"user_id"`
	RoleID    string    `gorm:"primaryKey" json:"role_id"`
	GrantedBy string    `gorm:"size:50" json:"granted_by"` // 授权人
	CreatedAt time.Time `json:"created_at"`

	// 关联关系
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Role Role `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

// RolePermission 角色权限关联
type RolePermission struct {
	RoleID       string    `gorm:"primaryKey" json:"role_id"`
	PermissionID string    `gorm:"primaryKey" json:"permission_id"`
	CreatedAt    time.Time `json:"created_at"`

	// 关联关系
	Role       Role       `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	Permission Permission `gorm:"foreignKey:PermissionID" json:"permission,omitempty"`
}

// NodeAccess 节点访问权限
type NodeAccess struct {
	ID        string         `gorm:"primaryKey" json:"id"`
	UserID    string         `gorm:"not null" json:"user_id"`
	NodeID    uint           `gorm:"not null" json:"node_id"`
	CanRead   bool           `gorm:"default:true" json:"can_read"`
	CanWrite  bool           `gorm:"default:false" json:"can_write"`
	CanDelete bool           `gorm:"default:false" json:"can_delete"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index:idx_node_access_deleted_at" json:"-"`

	// 关联关系
	User User `gorm:"foreignKey:UserID;references:ID" json:"user,omitempty"`
	Node Node `gorm:"foreignKey:NodeID;references:ID" json:"node,omitempty"`
}

// BeforeCreate 为节点访问记录生成主键
func (n *NodeAccess) BeforeCreate(tx *gorm.DB) error {
	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	return nil
}

// 预定义角色常量
const (
	RoleSuperAdmin       = "super_admin"
	RoleEnvironmentAdmin = "environment_admin"
	RoleNodeOperator     = "node_operator"
	RoleReadOnlyUser     = "read_only_user"
)

// 预定义权限常量
const (
	// 系统权限
	PermissionSystemManage = "system:manage"
	PermissionSystemConfig = "system:config"

	// 用户权限
	PermissionUserRead   = "user:read"
	PermissionUserWrite  = "user:write"
	PermissionUserDelete = "user:delete"

	// 节点权限
	PermissionNodeRead   = "node:read"
	PermissionNodeWrite  = "node:write"
	PermissionNodeDelete = "node:delete"

	// 进程权限
	PermissionProcessRead    = "process:read"
	PermissionProcessWrite   = "process:write"
	PermissionProcessExecute = "process:execute"
	PermissionProcessDelete  = "process:delete"

	// 日志权限
	PermissionLogRead   = "log:read"
	PermissionLogWrite  = "log:write"
	PermissionLogDelete = "log:delete"

	// 配置权限
	PermissionConfigRead       = "config:read"
	PermissionConfigWrite      = "config:write"
	PermissionConfigDelete     = "config:delete"
	PermissionConfigViewSecret = "config:view_secret" // 查看敏感配置信息

	// 环境变量权限
	PermissionEnvVarRead       = "env_var:read"
	PermissionEnvVarWrite      = "env_var:write"
	PermissionEnvVarDelete     = "env_var:delete"
	PermissionEnvVarViewSecret = "env_var:view_secret" // 查看敏感环境变量
)
