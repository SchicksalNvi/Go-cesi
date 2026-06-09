package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"superview/internal/errors"
	"superview/internal/logger"
	"superview/internal/repository"
)

// PermissionChecker 权限检查器
type PermissionChecker struct {
	db       *gorm.DB
	userRepo repository.UserRepository
}

func getContextUserID(c *gin.Context) (string, bool) {
	if userID, exists := c.Get("user_id"); exists {
		if userIDStr, ok := userID.(string); ok && userIDStr != "" {
			return userIDStr, true
		}
	}

	if userID, exists := c.Get("userID"); exists {
		if userIDStr, ok := userID.(string); ok && userIDStr != "" {
			return userIDStr, true
		}
	}

	return "", false
}

// NewPermissionChecker 创建权限检查器
func NewPermissionChecker(db *gorm.DB) *PermissionChecker {
	return &PermissionChecker{
		db:       db,
		userRepo: repository.NewUserRepository(db),
	}
}

// RequirePermission 要求特定权限的中间件
func (pc *PermissionChecker) RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr, exists := getContextUserID(c)
		if !exists {
			logger.Warn("Permission check failed: no user ID in context",
				zap.String("permission", permission))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "未认证",
				},
			})
			return
		}

		// 检查用户是否有权限
		hasPermission, err := pc.CheckPermission(userIDStr, permission)
		if err != nil {
			logger.Error("Permission check error",
				zap.String("userID", userIDStr),
				zap.String("permission", permission),
				zap.Error(err))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error": gin.H{
					"code":    "INTERNAL_ERROR",
					"message": "权限检查失败",
				},
			})
			return
		}

		if !hasPermission {
			logger.Warn("Permission denied",
				zap.String("userID", userIDStr),
				zap.String("permission", permission))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"status": "error",
				"error": gin.H{
					"code":    "FORBIDDEN",
					"message": "权限不足",
					"details": gin.H{
						"required_permission": permission,
					},
				},
			})
			return
		}

		c.Next()
	}
}

// RequireAnyPermission 要求任意一个权限的中间件
func (pc *PermissionChecker) RequireAnyPermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr, exists := getContextUserID(c)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "未认证",
				},
			})
			return
		}

		// 检查用户是否有任意一个权限
		for _, permission := range permissions {
			hasPermission, err := pc.CheckPermission(userIDStr, permission)
			if err != nil {
				logger.Error("Permission check error",
					zap.String("userID", userIDStr),
					zap.String("permission", permission),
					zap.Error(err))
				continue
			}

			if hasPermission {
				c.Next()
				return
			}
		}

		logger.Warn("Permission denied (any)",
			zap.String("userID", userIDStr),
			zap.Strings("permissions", permissions))
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error": gin.H{
				"code":    "FORBIDDEN",
				"message": "权限不足",
				"details": gin.H{
					"required_permissions": permissions,
				},
			},
		})
	}
}

// RequireAllPermissions 要求所有权限的中间件
func (pc *PermissionChecker) RequireAllPermissions(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr, exists := getContextUserID(c)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "未认证",
				},
			})
			return
		}

		// 检查用户是否有所有权限
		for _, permission := range permissions {
			hasPermission, err := pc.CheckPermission(userIDStr, permission)
			if err != nil {
				logger.Error("Permission check error",
					zap.String("userID", userIDStr),
					zap.String("permission", permission),
					zap.Error(err))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"status": "error",
					"error": gin.H{
						"code":    "INTERNAL_ERROR",
						"message": "权限检查失败",
					},
				})
				return
			}

			if !hasPermission {
				logger.Warn("Permission denied (all)",
					zap.String("userID", userIDStr),
					zap.String("missing_permission", permission))
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"status": "error",
					"error": gin.H{
						"code":    "FORBIDDEN",
						"message": "权限不足",
						"details": gin.H{
							"required_permissions": permissions,
							"missing_permission":   permission,
						},
					},
				})
				return
			}
		}

		c.Next()
	}
}

// RequireRole 要求特定角色的中间件
func (pc *PermissionChecker) RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr, exists := getContextUserID(c)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "未认证",
				},
			})
			return
		}

		// 检查用户是否有角色
		hasRole, err := pc.CheckRole(userIDStr, role)
		if err != nil {
			logger.Error("Role check error",
				zap.String("userID", userIDStr),
				zap.String("role", role),
				zap.Error(err))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error": gin.H{
					"code":    "INTERNAL_ERROR",
					"message": "角色检查失败",
				},
			})
			return
		}

		if !hasRole {
			logger.Warn("Role denied",
				zap.String("userID", userIDStr),
				zap.String("role", role))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"status": "error",
				"error": gin.H{
					"code":    "FORBIDDEN",
					"message": "权限不足",
					"details": gin.H{
						"required_role": role,
					},
				},
			})
			return
		}

		c.Next()
	}
}

// CheckPermission 检查用户是否有特定权限
func (pc *PermissionChecker) CheckPermission(userID, permission string) (bool, error) {
	// 获取用户及其角色和权限
	user, err := pc.userRepo.GetByID(userID)
	if err != nil {
		return false, errors.NewDatabaseError("get user", err)
	}

	// Super admins bypass all permission checks, whether granted by role
	// or by the legacy IsAdmin flag.
	if user.IsSuperAdmin() {
		return true, nil
	}

	// 检查用户的角色是否有该权限
	var count int64
	err = pc.db.Table("role_permissions").
		Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("user_roles.user_id = ? AND permissions.name = ?", userID, permission).
		Count(&count).Error

	if err != nil {
		return false, errors.NewDatabaseError("check permission", err)
	}

	return count > 0, nil
}

// CheckRole 检查用户是否有特定角色
func (pc *PermissionChecker) CheckRole(userID, roleName string) (bool, error) {
	var count int64
	err := pc.db.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.name = ?", userID, roleName).
		Count(&count).Error

	if err != nil {
		return false, errors.NewDatabaseError("check role", err)
	}

	return count > 0, nil
}

// GetUserPermissions 获取用户的所有权限
func (pc *PermissionChecker) GetUserPermissions(userID string) ([]string, error) {
	var permissions []string
	err := pc.db.Table("permissions").
		Select("DISTINCT permissions.name").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id").
		Where("user_roles.user_id = ?", userID).
		Pluck("permissions.name", &permissions).Error

	if err != nil {
		return nil, errors.NewDatabaseError("get user permissions", err)
	}

	return permissions, nil
}

// GetUserRoles 获取用户的所有角色
func (pc *PermissionChecker) GetUserRoles(userID string) ([]string, error) {
	var roles []string
	err := pc.db.Table("roles").
		Select("roles.name").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id = ?", userID).
		Pluck("roles.name", &roles).Error

	if err != nil {
		return nil, errors.NewDatabaseError("get user roles", err)
	}

	return roles, nil
}

// HasAnyRole 检查用户是否有任意一个角色
func (pc *PermissionChecker) HasAnyRole(userID string, roles ...string) (bool, error) {
	var count int64
	err := pc.db.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.name IN ?", userID, roles).
		Count(&count).Error

	if err != nil {
		return false, errors.NewDatabaseError("check any role", err)
	}

	return count > 0, nil
}

// HasAllRoles 检查用户是否有所有角色
func (pc *PermissionChecker) HasAllRoles(userID string, roles ...string) (bool, error) {
	var count int64
	err := pc.db.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.name IN ?", userID, roles).
		Count(&count).Error

	if err != nil {
		return false, errors.NewDatabaseError("check all roles", err)
	}

	return int(count) == len(roles), nil
}
