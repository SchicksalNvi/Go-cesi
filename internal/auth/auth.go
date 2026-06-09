package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"superview/internal/models"
	"superview/internal/services"
)

type AuthService struct {
	db                 *gorm.DB
	activityLogService *services.ActivityLogService
}

func NewAuthService(db *gorm.DB, activityLogService ...*services.ActivityLogService) *AuthService {
	s := &AuthService{db: db}
	if len(activityLogService) > 0 {
		s.activityLogService = activityLogService[0]
	}
	return s
}

// logAuth 记录认证相关的活动日志
func (s *AuthService) logAuth(c *gin.Context, action, userID, username, message string) {
	if s.activityLogService == nil {
		return
	}
	s.activityLogService.LogActivity(&models.ActivityLog{
		Level:     "INFO",
		Action:    action,
		Resource:  "auth",
		Target:    username,
		Message:   message,
		UserID:    userID,
		Username:  username,
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Status:    "success",
		CreatedAt: time.Now(),
	})
}

// isSecureRequest 检查请求是否通过 HTTPS
func isSecureRequest(c *gin.Context) bool {
	// 检查 X-Forwarded-Proto（反向代理场景）
	if proto := c.GetHeader("X-Forwarded-Proto"); proto == "https" {
		return true
	}
	// 检查请求 scheme
	return c.Request.TLS != nil
}

// setCookie 设置 Cookie，自动根据请求协议设置 Secure 标志
func (s *AuthService) setCookie(c *gin.Context, name, value string, maxAge int) {
	secure := isSecureRequest(c)
	c.SetCookie(name, value, maxAge, "/", "", secure, true)
}

func (s *AuthService) Login(c *gin.Context) {
	type loginRequest struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid request format",
		})
		return
	}

	var user models.User
	if err := s.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": "Invalid username/password",
		})
		return
	}

	// 验证密码
	passwordValid := user.VerifyPassword(req.Password)

	if !passwordValid {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": "Invalid username/password",
		})
		return
	}

	// 生成JWT令牌
	token, err := GenerateToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to generate token",
		})
		return
	}

	// 更新最后登录时间
	now := time.Now()
	s.db.Model(&user).Update("last_login", now)

	// 设置Cookie（自动检测 HTTPS 并设置 Secure 标志）
	s.setCookie(c, "token", token, 3600*24)

	s.logAuth(c, "login", user.ID, user.Username, fmt.Sprintf("User %s logged in", user.Username))

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login successful",
		"data": gin.H{
			"token": token,
			"user": gin.H{
				"id":         user.ID,
				"username":   user.Username,
				"email":      user.Email,
				"full_name":  user.FullName,
				"is_admin":   user.IsAdmin,
				"is_active":  user.IsActive,
				"created_at": user.CreatedAt,
				"updated_at": user.UpdatedAt,
			},
		},
	})
}

func (s *AuthService) Logout(c *gin.Context) {
	// 尝试从 token 获取用户信息用于日志记录
	var tokenString string
	if auth := c.GetHeader("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			tokenString = parts[1]
		}
	}
	if tokenString == "" {
		tokenString, _ = c.Cookie("token")
	}
	if tokenString != "" {
		if claims, err := ParseToken(tokenString); err == nil {
			var user models.User
			if s.db.Where("id = ?", claims.UserID).First(&user).Error == nil {
				s.logAuth(c, "logout", user.ID, user.Username, fmt.Sprintf("User %s logged out", user.Username))
			}
		}
	}

	// 清除Cookie（自动检测 HTTPS 并设置 Secure 标志）
	s.setCookie(c, "token", "", -1)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Logout successful",
	})
}

func (s *AuthService) GetCurrentUser(c *gin.Context) {
	// 从请求头或Cookie获取令牌
	var tokenString string

	// 先尝试从Authorization头获取
	auth := c.GetHeader("Authorization")
	if auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			tokenString = parts[1]
		}
	}

	// 如果请求头中没有令牌，尝试从Cookie获取
	if tokenString == "" {
		token, err := c.Cookie("token")
		if err == nil {
			tokenString = token
		}
	}

	// 如果都没有找到令牌
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "Not authenticated",
		})
		return
	}

	// 验证令牌
	claims, err := ParseToken(tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "Invalid or expired token",
		})
		return
	}

	// 获取用户信息
	var user models.User
	if err := s.db.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get user info",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"user": gin.H{
				"id":        user.ID,
				"username":  user.Username,
				"email":     user.Email,
				"full_name": user.FullName,
				"is_active": user.IsActive,
				"is_admin":  user.IsAdmin,
			},
		},
	})
}

func (s *AuthService) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 排除登录页面和静态资源
		if c.Request.URL.Path == "/login" || strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Next()
			return
		}

		// 从请求头或Cookie获取令牌
		var tokenString string

		// 先尝试从Authorization头获取
		auth := c.GetHeader("Authorization")
		if auth != "" {
			parts := strings.SplitN(auth, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		}

		// 如果请求头中没有令牌，尝试从Cookie获取
		if tokenString == "" {
			token, err := c.Cookie("token")
			if err == nil {
				tokenString = token
			}
		}

		// 如果还没有找到令牌，尝试从URL参数获取（用于WebSocket连接）
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		// 如果都没有找到令牌
		if tokenString == "" {
			// 如果是API请求返回JSON错误
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  "error",
					"message": "Authorization is required",
				})
				c.Abort()
				return
			}
			// 否则重定向到登录页面
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// 验证令牌
		claims, err := ParseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// Store both keys during the migration window to avoid breaking
		// handlers that still read the legacy context name.
		c.Set("user_id", claims.UserID)
		c.Set("userID", claims.UserID)

		// Query the fully-hydrated user object for downstream permission checks.
		// Fall back to a plain user load when association tables are unavailable
		// in narrower test fixtures.
		var user models.User
		err = s.db.
			Preload("Roles.Permissions").
			Preload("NodeAccess").
			Where("id = ?", claims.UserID).
			First(&user).Error
		if err != nil {
			err = s.db.Where("id = ?", claims.UserID).First(&user).Error
		}
		if err == nil {
			c.Set("user", &user)
		}

		c.Next()
	}
}
