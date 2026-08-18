package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"superview/internal/models"
	"superview/internal/services"
)

type RoleHandler struct {
	roleService        *services.RoleService
	permissionService  *services.PermissionService
	activityLogService *services.ActivityLogService
}

func NewRoleHandler(db *gorm.DB, activityLogService ...*services.ActivityLogService) *RoleHandler {
	h := &RoleHandler{
		roleService:       services.NewRoleService(db),
		permissionService: services.NewPermissionService(db),
	}
	if len(activityLogService) > 0 {
		h.activityLogService = activityLogService[0]
	}
	return h
}

// CreateRole 创建角色
// @Summary 创建角色
// @Description 创建新的角色
// @Tags roles
// @Accept json
// @Produce json
// @Param role body models.Role true "角色信息"
// @Success 201 {object} models.Role
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/roles [post]
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var role models.Role
	if err := c.ShouldBindJSON(&role); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	
	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "未授权"})
		return
	}
	
	currentUser := user.(*models.User)
	if !currentUser.HasPermission(models.PermissionUserWrite) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "权限不足"})
		return
	}

	// 只有超级管理员可以创建系统角色或保留名称的角色
	if role.IsSystem || isSystemRoleName(role.Name) {
		if !currentUser.IsSuperAdmin() {
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "只有超级管理员可以管理系统角色"})
			return
		}
	}
	
	if err := h.roleService.CreateRole(&role); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	
	c.JSON(http.StatusCreated, role)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Created role: %s", role.Name)
		h.activityLogService.LogWithContext(c, "INFO", "create_role", "role", role.Name, msg, nil)
	}
}

// GetRoles 获取角色列表
// @Summary 获取角色列表
// @Description 获取所有角色列表
// @Tags roles
// @Produce json
// @Param page query int false "页码" default(1)
// @Param limit query int false "每页数量" default(10)
// @Success 200 {object} PaginatedResponse{data=[]models.Role}
// @Failure 500 {object} ErrorResponse
// @Router /api/roles [get]
func (h *RoleHandler) GetRoles(c *gin.Context) {
	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "未授权"})
		return
	}
	
	currentUser := user.(*models.User)
	if !currentUser.HasPermission(models.PermissionUserRead) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "权限不足"})
		return
	}
	
	roles, err := h.roleService.GetAllRoles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	
	// 分页处理
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	
	total := len(roles)
	start := (page - 1) * limit
	end := start + limit
	
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	
	paginatedRoles := roles[start:end]
	
	response := PaginatedResponse{
		Data:       paginatedRoles,
		Total:      int64(total),
		Page:       page,
		PageSize:   limit,
		TotalPages: int64((total + limit - 1) / limit),
	}
	
	c.JSON(http.StatusOK, response)
}

// GetRole 获取角色详情
// @Summary 获取角色详情
// @Description 根据ID获取角色详情
// @Tags roles
// @Produce json
// @Param id path string true "角色ID"
// @Success 200 {object} models.Role
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/roles/{id} [get]
func (h *RoleHandler) GetRole(c *gin.Context) {
	id := c.Param("id")
	
	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "未授权"})
		return
	}
	
	currentUser := user.(*models.User)
	if !currentUser.HasPermission(models.PermissionUserRead) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "权限不足"})
		return
	}
	
	role, err := h.roleService.GetRoleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "角色不存在"})
		return
	}
	
	c.JSON(http.StatusOK, role)
}

// UpdateRole 更新角色
// @Summary 更新角色
// @Description 更新角色信息
// @Tags roles
// @Accept json
// @Produce json
// @Param id path string true "角色ID"
// @Param role body models.Role true "角色信息"
// @Success 200 {object} models.Role
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/roles/{id} [put]
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	id := c.Param("id")
	
	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "未授权"})
		return
	}
	
	currentUser := user.(*models.User)
	if !currentUser.HasPermission(models.PermissionUserWrite) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "权限不足"})
		return
	}

	// 只有超级管理员可以更新系统角色
	existingRole, err := h.roleService.GetRoleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "角色不存在"})
		return
	}
	if existingRole.IsSystem {
		if !currentUser.IsSuperAdmin() {
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "只有超级管理员可以修改系统角色"})
			return
		}
	}
	
	var role models.Role
	if err := c.ShouldBindJSON(&role); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	
	role.ID = id
	if err := h.roleService.UpdateRole(&role); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, role)

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Updated role: %s", role.Name)
		h.activityLogService.LogWithContext(c, "INFO", "update_role", "role", id, msg, nil)
	}
}

// DeleteRole 删除角色
// @Summary 删除角色
// @Description 删除指定角色
// @Tags roles
// @Produce json
// @Param id path string true "角色ID"
// @Success 200 {object} SuccessResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/roles/{id} [delete]
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	id := c.Param("id")
	
	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "未授权"})
		return
	}
	
	currentUser := user.(*models.User)
	if !currentUser.HasPermission(models.PermissionUserDelete) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "权限不足"})
		return
	}

	// 只有超级管理员可以删除系统角色
	existingRole, err := h.roleService.GetRoleByID(id)
	if err == nil && existingRole.IsSystem {
		if !currentUser.IsSuperAdmin() {
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "只有超级管理员可以删除系统角色"})
			return
		}
	}
	
	if err := h.roleService.DeleteRole(id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, SuccessResponse{Message: "角色删除成功"})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Deleted role ID: %s", id)
		h.activityLogService.LogWithContext(c, "WARNING", "delete_role", "role", id, msg, nil)
	}
}

// AssignPermissions 为角色分配权限
// @Summary 为角色分配权限
// @Description 为指定角色分配权限列表
// @Tags roles
// @Accept json
// @Produce json
// @Param id path string true "角色ID"
// @Param permissions body []string true "权限ID列表"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/roles/{id}/permissions [post]
func (h *RoleHandler) AssignPermissions(c *gin.Context) {
	id := c.Param("id")
	
	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "未授权"})
		return
	}
	
	currentUser := user.(*models.User)
	if !currentUser.HasPermission(models.PermissionUserWrite) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "权限不足"})
		return
	}

	// 只有超级管理员可以修改系统角色的权限
	role, err := h.roleService.GetRoleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "角色不存在"})
		return
	}
	if role.IsSystem {
		if !currentUser.IsSuperAdmin() {
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "只有超级管理员可以修改系统角色的权限"})
			return
		}
	}
	
	var permissionIDs []string
	if err := c.ShouldBindJSON(&permissionIDs); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	
	if err := h.roleService.AssignPermissionsToRole(id, permissionIDs); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, SuccessResponse{Message: "权限分配成功"})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Assigned %d permissions to role %s", len(permissionIDs), id)
		h.activityLogService.LogWithContext(c, "INFO", "assign_permissions", "role", id, msg, nil)
	}
}

// AssignRoleToUser 为用户分配角色
// @Summary 为用户分配角色
// @Description 为指定用户分配角色
// @Tags roles
// @Accept json
// @Produce json
// @Param roleId path string true "角色ID"
// @Param userId path string true "用户ID"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/roles/{roleId}/users/{userId} [post]
func (h *RoleHandler) AssignRoleToUser(c *gin.Context) {
	roleID := c.Param("roleId")
	userID := c.Param("userId")
	
	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "未授权"})
		return
	}
	
	currentUser := user.(*models.User)
	if !currentUser.HasPermission(models.PermissionUserWrite) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "权限不足"})
		return
	}

	// 只有超级管理员可以分配/移除系统角色(如 super_admin)
	role, err := h.roleService.GetRoleByID(roleID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "角色不存在"})
		return
	}
	if role.IsSystem && !currentUser.IsSuperAdmin() {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "只有超级管理员可以分配系统角色"})
		return
	}

	// 禁止用户给自己分配系统角色(自我提权防护)
	if role.IsSystem && userID == currentUser.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "禁止自我分配系统角色"})
		return
	}
	
	if err := h.roleService.AssignRoleToUser(userID, roleID, currentUser.ID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, SuccessResponse{Message: "角色分配成功"})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Assigned role %s to user %s", roleID, userID)
		h.activityLogService.LogWithContext(c, "INFO", "assign_role", "role", roleID, msg, nil)
	}
}

// RemoveRoleFromUser 移除用户角色
// @Summary 移除用户角色
// @Description 移除用户的指定角色
// @Tags roles
// @Produce json
// @Param roleId path string true "角色ID"
// @Param userId path string true "用户ID"
// @Success 200 {object} SuccessResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/roles/{roleId}/users/{userId} [delete]
func (h *RoleHandler) RemoveRoleFromUser(c *gin.Context) {
	roleID := c.Param("roleId")
	userID := c.Param("userId")
	
	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "未授权"})
		return
	}
	
	currentUser := user.(*models.User)
	if !currentUser.HasPermission(models.PermissionUserWrite) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "权限不足"})
		return
	}

	// 只有超级管理员可以移除系统角色
	role, err := h.roleService.GetRoleByID(roleID)
	if err == nil && role.IsSystem {
		if !currentUser.IsSuperAdmin() {
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "只有超级管理员可以移除系统角色"})
			return
		}
	}
	
	if err := h.roleService.RemoveRoleFromUser(userID, roleID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, SuccessResponse{Message: "角色移除成功"})

	if h.activityLogService != nil {
		msg := fmt.Sprintf("Removed role %s from user %s", roleID, userID)
		h.activityLogService.LogWithContext(c, "WARNING", "remove_role", "role", roleID, msg, nil)
	}
}

// GetPermissions 获取权限列表
// @Summary 获取权限列表
// @Description 获取所有权限列表
// @Tags permissions
// @Produce json
// @Param resource query string false "资源类型过滤"
// @Success 200 {object} []models.Permission
// @Failure 500 {object} ErrorResponse
// @Router /api/permissions [get]
func (h *RoleHandler) GetPermissions(c *gin.Context) {
	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "未授权"})
		return
	}
	
	currentUser := user.(*models.User)
	if !currentUser.HasPermission(models.PermissionUserRead) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "权限不足"})
		return
	}
	
	resource := c.Query("resource")
	var permissions []models.Permission
	var err error
	
	if resource != "" {
		permissions, err = h.permissionService.GetPermissionsByResource(resource)
	} else {
		permissions, err = h.permissionService.GetAllPermissions()
	}
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, permissions)
}

// isSystemRoleName 判断角色名是否为预定义的系统角色
func isSystemRoleName(name string) bool {
	switch name {
	case models.RoleSuperAdmin, models.RoleEnvironmentAdmin, models.RoleNodeOperator, models.RoleReadOnlyUser:
		return true
	}
	return false
}