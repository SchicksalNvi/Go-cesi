package api

import (
	"strconv"

	appErrors "superview/internal/errors"
	"superview/internal/models"

	"github.com/gin-gonic/gin"
)

// parseIDParam 解析URL参数中的ID
func parseIDParam(c *gin.Context, paramName string) (uint, error) {
	id, err := strconv.ParseUint(c.Param(paramName), 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

// getUserID 从上下文中获取用户ID（返回 string）
func getUserIDString(c *gin.Context) (string, bool) {
	for _, key := range []string{"user_id", "userID"} {
		if userID, exists := c.Get(key); exists {
			if userIDStr, ok := userID.(string); ok && userIDStr != "" {
				return userIDStr, true
			}
		}
	}

	if userValue, exists := c.Get("user"); exists {
		if user, ok := userValue.(*models.User); ok && user != nil && user.ID != "" {
			return user.ID, true
		}
	}

	return "", false
}

// 旧的响应类型（保持向后兼容）
type ErrorResponse = ErrorResponseLegacy
type SuccessResponse = SuccessResponseLegacy

// SuccessResponse 别名函数，保持向后兼容
func SuccessResponseFunc(c *gin.Context, data interface{}) {
	Success(c, data)
}

// ErrorResponse 别名函数，保持向后兼容
func ErrorResponseFunc(c *gin.Context, err error) {
	Error(c, err)
}

// handleError 统一错误处理（已废弃，使用 Error() 代替）
func handleError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, ErrorResponse{Error: message})
}

// handleAppError 处理应用程序错误
func handleAppError(c *gin.Context, err error) {
	Error(c, err)
}

// handleSuccess 统一成功响应
func handleSuccess(c *gin.Context, message string, data interface{}) {
	SuccessWithMessage(c, message, data)
}

// handleInvalidID 处理无效ID错误
func handleInvalidID(c *gin.Context, resourceType string) {
	err := appErrors.NewValidationError("id", "Invalid "+resourceType+" ID")
	handleAppError(c, err)
}

// handleUnauthorized 处理未授权错误
func handleUnauthorized(c *gin.Context) {
	err := appErrors.NewUnauthorizedError("User not authenticated")
	handleAppError(c, err)
}

// handleInternalError 处理内部服务器错误
func handleInternalError(c *gin.Context, err error) {
	appErr := appErrors.NewInternalError("Internal server error", err)
	handleAppError(c, appErr)
}

// handleBadRequest 处理请求参数错误
func handleBadRequest(c *gin.Context, err error) {
	appErr := appErrors.NewValidationError("request", "Invalid request parameters: "+err.Error())
	handleAppError(c, appErr)
}

// handleNotFound 处理资源未找到错误
func handleNotFound(c *gin.Context, resource, identifier string) {
	err := appErrors.NewNotFoundError(resource, identifier)
	handleAppError(c, err)
}

// handleConflict 处理资源冲突错误
func handleConflict(c *gin.Context, resource, message string) {
	err := appErrors.NewConflictError(resource, message)
	handleAppError(c, err)
}

// handleForbidden 处理禁止访问错误
func handleForbidden(c *gin.Context, message string) {
	err := appErrors.NewForbiddenError(message)
	handleAppError(c, err)
}

// parseAndValidateID 解析并验证ID参数的通用函数
func parseAndValidateID(c *gin.Context, paramName, resourceType string) (uint, bool) {
	id, err := parseIDParam(c, paramName)
	if err != nil {
		handleInvalidID(c, resourceType)
		return 0, false
	}
	return id, true
}

// validateUserAuth 验证用户认证的通用函数（返回 string）
func validateUserAuthString(c *gin.Context) (string, bool) {
	userID, exists := getUserIDString(c)
	if !exists {
		handleUnauthorized(c)
		return "", false
	}
	return userID, true
}

func boolValueOrDefault(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}
