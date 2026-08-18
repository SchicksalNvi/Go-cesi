package models

import (
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"time"
)

type User struct {
	ID           string         `gorm:"primaryKey;type:varchar(36)" json:"id" validate:"required,uuid4"`
	Username     string         `gorm:"uniqueIndex:idx_username;size:50;not null" json:"username" validate:"required,min=3,max=50,alphanum"`
	Password     string         `gorm:"size:120;not null" json:"-" validate:"required,min=8"`
	Email        string         `gorm:"uniqueIndex:idx_email;size:100" json:"email" validate:"omitempty,email,max=100"`
	FullName     string         `gorm:"size:100" json:"full_name" validate:"omitempty,max=100"`
	IsActive     bool           `gorm:"default:true;not null;index:idx_active" json:"is_active"`
	IsAdmin      bool           `gorm:"default:false;not null;index:idx_admin" json:"is_admin"` // 保持向后兼容
	TokenVersion uint64         `gorm:"default:0;not null" json:"-"`
	LastLogin    *time.Time     `gorm:"index:idx_last_login" json:"last_login"`
	CreatedAt    time.Time      `gorm:"not null;index:idx_created_at" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index:idx_user_deleted_at" json:"-"`

	// 关联关系
	Roles      []Role       `gorm:"many2many:user_roles;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"roles,omitempty"`
	NodeAccess []NodeAccess `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"node_access,omitempty"`
}

// BeforeCreate generates UUID for new users
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

func (u *User) SetPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

func (u *User) VerifyPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

// HasRole 检查用户是否拥有指定角色
func (u *User) HasRole(roleName string) bool {
	for _, role := range u.Roles {
		if role.Name == roleName {
			return true
		}
	}
	return false
}

// HasPermission 检查用户是否拥有指定权限
func (u *User) HasPermission(permissionName string) bool {
	for _, role := range u.Roles {
		for _, permission := range role.Permissions {
			if permission.Name == permissionName {
				return true
			}
		}
	}
	return false
}

// IsSuperAdmin 检查用户是否为超级管理员
func (u *User) IsSuperAdmin() bool {
	return u.IsAdmin || u.HasRole(RoleSuperAdmin)
}

// CanAccessNode 检查用户是否可以访问指定节点
func (u *User) CanAccessNode(nodeID uint, action string) bool {
	// 超级管理员拥有所有权限
	if u.IsSuperAdmin() {
		return true
	}

	// When no node-scoped ACL exists, route-level permission checks remain the source
	// of truth and the node scope should not impose extra restrictions.
	if len(u.NodeAccess) == 0 {
		return true
	}

	// 检查节点访问权限
	for _, access := range u.NodeAccess {
		if access.NodeID == nodeID {
			switch action {
			case "read", "log":
				return access.CanRead
			case "write", "execute":
				return access.CanWrite
			case "delete":
				return access.CanDelete
			}
		}
	}

	return false
}

// GetRoleNames 获取用户所有角色名称
func (u *User) GetRoleNames() []string {
	roles := make([]string, len(u.Roles))
	for i, role := range u.Roles {
		roles[i] = role.Name
	}
	return roles
}

// GetPermissionNames 获取用户所有权限名称
func (u *User) GetPermissionNames() []string {
	permissionMap := make(map[string]bool)
	for _, role := range u.Roles {
		for _, permission := range role.Permissions {
			permissionMap[permission.Name] = true
		}
	}

	permissions := make([]string, 0, len(permissionMap))
	for permission := range permissionMap {
		permissions = append(permissions, permission)
	}
	return permissions
}
