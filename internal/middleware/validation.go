package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"superview/internal/validation"
)

// ValidationMiddleware 输入验证中间件
type ValidationMiddleware struct{}

// NewValidationMiddleware 创建验证中间件
func NewValidationMiddleware() *ValidationMiddleware {
	return &ValidationMiddleware{}
}

// ValidatePathParams 验证路径参数
func (vm *ValidationMiddleware) ValidatePathParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		validator := validation.NewValidator()
		
		// 验证常见的路径参数
		if nodeName := c.Param("node_name"); nodeName != "" {
			validator.ValidateNodeName("node_name", nodeName)
			validator.ValidateNoSQLInjection("node_name", nodeName)
			c.Set("node_name", validation.SanitizeInput(nodeName))
		}
		
		if processName := c.Param("process_name"); processName != "" {
			validator.ValidateProcessName("process_name", processName)
			validator.ValidateNoSQLInjection("process_name", processName)
			c.Set("process_name", validation.SanitizeInput(processName))
		}
		
		if username := c.Param("username"); username != "" {
			validator.ValidateRequired("username", username)
			validator.ValidateLength("username", username, 3, 50)
			validator.ValidateAlphanumeric("username", username)
			validator.ValidateNoSQLInjection("username", username)
			c.Set("username", validation.SanitizeInput(username))
		}
		
		if idStr := c.Param("id"); idStr != "" {
			id := validator.ValidateID("id", idStr)
			c.Set("id", id)
		}
		
		if validator.HasErrors() {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "路径参数验证失败",
				"errors":  validator.Errors(),
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// ValidateQueryParams 验证查询参数
func (vm *ValidationMiddleware) ValidateQueryParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		validator := validation.NewValidator()
		
		// 验证分页参数
		page := c.Query("page")
		limit := c.Query("limit")
		pageNum, limitNum := validator.ValidatePagination(page, limit)
		c.Set("page", pageNum)
		c.Set("limit", limitNum)
		
		// 验证搜索参数
		if search := c.Query("search"); search != "" {
			validator.ValidateLength("search", search, 0, 100)
			validator.ValidateNoSQLInjection("search", search)
			c.Set("search", validation.SanitizeInput(search))
		}
		
		// 验证排序参数
		if sortBy := c.Query("sort_by"); sortBy != "" {
			validator.ValidateAlphanumeric("sort_by", sortBy)
			validator.ValidateNoSQLInjection("sort_by", sortBy)
			c.Set("sort_by", validation.SanitizeInput(sortBy))
		}
		
		if sortOrder := c.Query("sort_order"); sortOrder != "" {
			if sortOrder != "asc" && sortOrder != "desc" {
				validator.AddError("sort_order", "must be 'asc' or 'desc'")
			} else {
				c.Set("sort_order", sortOrder)
			}
		}
		
		// 验证时间范围参数
		if startTime := c.Query("start_time"); startTime != "" {
			if _, err := strconv.ParseInt(startTime, 10, 64); err != nil {
				validator.AddError("start_time", "must be a valid timestamp")
			} else {
				c.Set("start_time", startTime)
			}
		}
		
		if endTime := c.Query("end_time"); endTime != "" {
			if _, err := strconv.ParseInt(endTime, 10, 64); err != nil {
				validator.AddError("end_time", "must be a valid timestamp")
			} else {
				c.Set("end_time", endTime)
			}
		}
		
		if validator.HasErrors() {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "查询参数验证失败",
				"errors":  validator.Errors(),
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// ValidateJSONBody 验证JSON请求体
func (vm *ValidationMiddleware) ValidateJSONBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查Content-Type
		contentType := c.GetHeader("Content-Type")
		if contentType != "" && contentType != "application/json" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Content-Type must be application/json",
			})
			c.Abort()
			return
		}
		
		// 检查请求体大小
		if c.Request.ContentLength > 1024*1024 { // 1MB limit
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"status":  "error",
				"message": "Request body too large (max 1MB)",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// RateLimitByIP IP限流中间件
func (vm *ValidationMiddleware) RateLimitByIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 这里可以实现基于IP的限流逻辑
		// 暂时跳过，可以后续集成redis或内存限流器
		c.Next()
	}
}

// SecurityHeaders 安全头中间件
func (vm *ValidationMiddleware) SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置安全头
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		// Ant Design (web/react-app) 通过 CSS-in-JS 在运行时动态注入内联样式,
		// 且图标多使用 data:URI。因此 style-src 必须含 'unsafe-inline'(运行时生成的
		// 样式哈希不可预测,无法用固定 hash 覆盖),img-src 需允许 data:。
		// script-src 仍由 default-src 'self' 约束,保持严格。
		c.Header("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'")
		
		c.Next()
	}
}

// GetValidatedParam 获取已验证的路径参数
func GetValidatedParam(c *gin.Context, key string) string {
	if value, exists := c.Get(key); exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return c.Param(key)
}

// GetValidatedParamInt 获取已验证的整数路径参数
func GetValidatedParamInt(c *gin.Context, key string) int {
	if value, exists := c.Get(key); exists {
		if intVal, ok := value.(int); ok {
			return intVal
		}
	}
	return 0
}

// GetValidatedQuery 获取已验证的查询参数
func GetValidatedQuery(c *gin.Context, key string) string {
	if value, exists := c.Get(key); exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return c.Query(key)
}

// GetValidatedQueryInt 获取已验证的整数查询参数
func GetValidatedQueryInt(c *gin.Context, key string) int {
	if value, exists := c.Get(key); exists {
		if intVal, ok := value.(int); ok {
			return intVal
		}
	}
	return 0
}