package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"superview/internal/models"
	"superview/internal/repository"
	"superview/internal/services"
)

type UserHandler struct {
	db                 *gorm.DB
	userService        *services.UserService
	activityLogService *services.ActivityLogService
}

func NewUserHandler(db *gorm.DB, activityLogService ...*services.ActivityLogService) *UserHandler {
	repo := repository.NewRepository(db)
	h := &UserHandler{
		db:          db,
		userService: services.NewUserService(repo),
	}
	if len(activityLogService) > 0 {
		h.activityLogService = activityLogService[0]
	}
	return h
}

type userNodeAccessEntryRequest struct {
	NodeID    uint  `json:"node_id"`
	CanRead   *bool `json:"can_read"`
	CanWrite  *bool `json:"can_write"`
	CanDelete *bool `json:"can_delete"`
}

func createNodeAccessRecord(tx *gorm.DB, access *models.NodeAccess) error {
	boolValues := map[string]interface{}{
		"can_read":   access.CanRead,
		"can_write":  access.CanWrite,
		"can_delete": access.CanDelete,
	}

	if err := tx.Create(access).Error; err != nil {
		return err
	}

	return tx.Model(access).UpdateColumns(boolValues).Error
}

// GetUsers 获取用户列表
func (h *UserHandler) GetUsers(c *gin.Context) {
	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	users, total, err := h.userService.ListUsers(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to fetch users",
		})
		return
	}

	// 清除密码字段
	for _, user := range users {
		user.Password = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"users":     users,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// CreateUser 创建用户
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=50"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
		FullName string `json:"full_name"`
		IsAdmin  bool   `json:"is_admin"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	// 只有超级管理员可以创建拥有管理员权限的用户
	if req.IsAdmin {
		currentUser, ok := getCurrentUser(c)
		if !ok {
			return
		}
		if !currentUser.IsSuperAdmin() {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": "Only super admins can create admin users",
			})
			return
		}
	}

	user := &models.User{
		Username: req.Username,
		Email:    req.Email,
		FullName: req.FullName,
		IsAdmin:  req.IsAdmin,
	}

	if err := user.SetPassword(req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to set password",
		})
		return
	}

	if err := h.userService.CreateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	// 清除密码
	user.Password = ""

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "User created successfully",
		"data":    user,
	})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created user %s", user.Username)
		h.activityLogService.LogWithContext(c, "INFO", "create_user", "user", user.Username, msg, nil)
	}
}

// UpdateUser 更新用户
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Email    string `json:"email" binding:"omitempty,email"`
		FullName string `json:"full_name"`
		IsAdmin  *bool  `json:"is_admin"`
		IsActive *bool  `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	currentUser, ok := getCurrentUser(c)
	if !ok {
		return
	}

	user, err := h.userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "User not found",
		})
		return
	}

	// 更新字段
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.FullName != "" {
		user.FullName = req.FullName
	}
	if req.IsAdmin != nil {
		// 只有超级管理员可以修改用户的 is_admin 字段
		if !currentUser.IsSuperAdmin() {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": "Only super admins can modify user admin privileges",
			})
			return
		}
		if !*req.IsAdmin && user.IsSuperAdmin() {
			if currentUser.ID == id {
				c.JSON(http.StatusForbidden, gin.H{
					"status":  "error",
					"message": "Cannot remove your own super admin privileges",
				})
				return
			}
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": "Cannot remove super admin privileges from super admin user",
			})
			return
		}
		user.IsAdmin = *req.IsAdmin
	}
	if req.IsActive != nil {
		if !*req.IsActive {
			if currentUser.ID == id {
				c.JSON(http.StatusForbidden, gin.H{
					"status":  "error",
					"message": "Cannot deactivate your own account",
				})
				return
			}
			if user.IsSuperAdmin() {
				c.JSON(http.StatusForbidden, gin.H{
					"status":  "error",
					"message": "Cannot deactivate super admin user",
				})
				return
			}
		}
		user.IsActive = *req.IsActive
	}

	if err := h.userService.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	// 清除密码
	user.Password = ""

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User updated successfully",
		"data":    user,
	})
}

// DeleteUser 删除用户
func (h *UserHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")

	currentUser, ok := getCurrentUser(c)
	if !ok {
		return
	}

	if currentUser.ID == id {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": "Cannot delete your own account",
		})
		return
	}

	user, err := h.userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "User not found",
		})
		return
	}

	// 不允许删除超级管理员
	if user.IsSuperAdmin() {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": "Cannot delete super admin user",
		})
		return
	}

	if err := h.userService.DeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted user %s", user.Username)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_user", "user", user.Username, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User deleted successfully",
	})
}

// ToggleUserStatus 切换用户状态
func (h *UserHandler) ToggleUserStatus(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		IsActive bool `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request",
		})
		return
	}

	currentUser, ok := getCurrentUser(c)
	if !ok {
		return
	}

	targetUser, err := h.userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "User not found",
		})
		return
	}

	if !req.IsActive {
		if currentUser.ID == id {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": "Cannot deactivate your own account",
			})
			return
		}
		if targetUser.IsSuperAdmin() {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": "Cannot deactivate super admin user",
			})
			return
		}
	}

	if req.IsActive {
		err = h.userService.ActivateUser(id)
	} else {
		err = h.userService.DeactivateUser(id)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User status updated successfully",
	})

	if h.activityLogService != nil {
		action := "activate"
		if !req.IsActive {
			action = "deactivate"
		}
		msg := fmt.Sprintf("User %s %sd", id, action)
		h.activityLogService.LogWithContext(c, "INFO", action+"_user", "user", id, msg, nil)
	}
}

// GetUserByID 获取单个用户
func (h *UserHandler) GetUserByID(c *gin.Context) {
	id := c.Param("id")

	user, err := h.userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "User not found",
		})
		return
	}

	// 清除密码
	user.Password = ""

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   user,
	})
}

// ResetPassword 管理员重置用户密码
func (h *UserHandler) ResetPassword(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	user, err := h.userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "User not found",
		})
		return
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to hash password",
		})
		return
	}
	if err := h.userService.SetPasswordAndRevokeSessions(user.ID, user.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Password reset successfully",
	})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Reset password for user %s", user.Username)
		h.activityLogService.LogWithContext(c, "WARNING", "change_password", "user", user.Username, msg, nil)
	}
}

// GetUserNodeAccess 获取用户节点访问权限
func (h *UserHandler) GetUserNodeAccess(c *gin.Context) {
	id := c.Param("id")

	user, err := h.userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "User not found",
		})
		return
	}

	var nodes []models.Node
	if err := h.db.Order("name ASC").Find(&nodes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to load nodes",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"user_id":         user.ID,
			"node_access":     user.NodeAccess,
			"available_nodes": nodes,
		},
	})
}

// UpdateUserNodeAccess 替换用户节点访问权限
func (h *UserHandler) UpdateUserNodeAccess(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		NodeAccess []userNodeAccessEntryRequest `json:"node_access"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request: " + err.Error(),
		})
		return
	}

	targetUser, err := h.userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "User not found",
		})
		return
	}

	seenNodeIDs := make(map[uint]struct{}, len(req.NodeAccess))
	for _, entry := range req.NodeAccess {
		if entry.NodeID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "node_id is required",
			})
			return
		}
		if _, exists := seenNodeIDs[entry.NodeID]; exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Duplicate node access entry for node %d", entry.NodeID),
			})
			return
		}
		seenNodeIDs[entry.NodeID] = struct{}{}

		var node models.Node
		if err := h.db.First(&node, entry.NodeID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Node %d not found", entry.NodeID),
			})
			return
		}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&models.NodeAccess{}).Error; err != nil {
			return err
		}

		for _, entry := range req.NodeAccess {
			access := &models.NodeAccess{
				UserID:    id,
				NodeID:    entry.NodeID,
				CanRead:   boolValueOrDefault(entry.CanRead, true),
				CanWrite:  boolValueOrDefault(entry.CanWrite, false),
				CanDelete: boolValueOrDefault(entry.CanDelete, false),
			}
			if err := createNodeAccessRecord(tx, access); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to update node access",
		})
		return
	}

	updatedUser, err := h.userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to reload updated user",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User node access updated successfully",
		"data": gin.H{
			"user_id":     updatedUser.ID,
			"node_access": updatedUser.NodeAccess,
		},
	})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Updated node access for user %s", targetUser.Username)
		h.activityLogService.LogWithContext(c, "INFO", "update_user_node_access", "user", targetUser.ID, msg, map[string]interface{}{
			"node_access_count": len(req.NodeAccess),
		})
	}
}
