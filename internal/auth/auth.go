package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"superview/internal/logger"
	"superview/internal/models"
	"superview/internal/services"
)

type AuthService struct {
	db                 *gorm.DB
	activityLogService *services.ActivityLogService
}

var ErrSessionUnavailable = errors.New("authenticated user session is unavailable")

func loadUserForSession(db *gorm.DB, userID string, tokenVersion uint64) (*models.User, error) {
	if db == nil {
		return nil, fmt.Errorf("load authenticated user: database is nil")
	}

	var user models.User
	err := db.
		Preload("Roles.Permissions").
		Preload("NodeAccess").
		Where("id = ?", userID).
		First(&user).Error
	if err != nil {
		// Some focused tests intentionally omit association tables. Preserve the
		// existing fallback while still requiring the user record itself.
		err = db.Where("id = ?", userID).First(&user).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionUnavailable
		}
		return nil, fmt.Errorf("load authenticated user: %w", err)
	}
	if !user.IsActive || user.TokenVersion != tokenVersion {
		return nil, ErrSessionUnavailable
	}
	return &user, nil
}

// AuthenticateToken validates the JWT and binds it to the current database
// state. Deleting, disabling, logging out, or otherwise revoking a user makes
// previously issued tokens fail this check.
func AuthenticateToken(db *gorm.DB, tokenString string) (*models.User, *Claims, error) {
	claims, err := ParseToken(tokenString)
	if err != nil {
		return nil, nil, err
	}

	user, err := loadUserForSession(db, claims.UserID, claims.TokenVersion)
	if err != nil {
		return nil, nil, err
	}
	return user, claims, nil
}

// ValidateUserSession is the lightweight variant used to revalidate long-lived
// connections such as WebSockets.
func ValidateUserSession(db *gorm.DB, userID string, tokenVersion uint64) error {
	if db == nil {
		return fmt.Errorf("validate user session: database is nil")
	}

	var user models.User
	err := db.Select("id", "is_active", "token_version").Where("id = ?", userID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSessionUnavailable
		}
		return fmt.Errorf("validate user session: %w", err)
	}
	if !user.IsActive || user.TokenVersion != tokenVersion {
		return ErrSessionUnavailable
	}
	return nil
}

func (s *AuthService) loadUserForResponse(userID string, tokenVersion uint64) (*models.User, error) {
	return loadUserForSession(s.db, userID, tokenVersion)
}

func buildAuthUserPayload(user *models.User) gin.H {
	return gin.H{
		"id":          user.ID,
		"username":    user.Username,
		"email":       user.Email,
		"full_name":   user.FullName,
		"is_admin":    user.IsAdmin,
		"is_active":   user.IsActive,
		"roles":       user.GetRoleNames(),
		"permissions": user.GetPermissionNames(),
		"created_at":  user.CreatedAt,
		"updated_at":  user.UpdatedAt,
	}
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

	if !passwordValid || !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": "Invalid username/password",
		})
		return
	}

	userWithPermissions, err := s.loadUserForResponse(user.ID, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": "Invalid username/password",
		})
		return
	}

	// 生成与当前用户会话版本绑定的JWT令牌
	token, err := GenerateToken(userWithPermissions.ID, userWithPermissions.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to generate token",
		})
		return
	}

	// 更新最后登录时间
	now := time.Now()
	s.db.Model(&models.User{}).Where("id = ? AND token_version = ?", user.ID, user.TokenVersion).Update("last_login", now)

	// 设置Cookie（自动检测 HTTPS 并设置 Secure 标志）
	s.setCookie(c, "token", token, 3600*24)

	s.logAuth(c, "login", user.ID, user.Username, fmt.Sprintf("User %s logged in", user.Username))

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Login successful",
		"data": gin.H{
			"token": token,
			"user":  buildAuthUserPayload(userWithPermissions),
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
	var revokeErr error
	if tokenString != "" && s.db != nil {
		if claims, err := ParseToken(tokenString); err == nil {
			var user models.User
			lookupErr := s.db.Where("id = ? AND token_version = ?", claims.UserID, claims.TokenVersion).First(&user).Error
			if lookupErr == nil {
				// Incrementing the version revokes all tokens issued for the current
				// session generation. The version predicate prevents an already
				// revoked token from invalidating a newer login.
				result := s.db.Model(&models.User{}).
					Where("id = ? AND token_version = ?", claims.UserID, claims.TokenVersion).
					UpdateColumn("token_version", gorm.Expr("token_version + 1"))
				if result.Error != nil {
					revokeErr = result.Error
					logger.Error("Failed to revoke user sessions during logout",
						zap.String("user_id", claims.UserID),
						zap.Error(result.Error))
				} else if result.RowsAffected > 0 {
					s.logAuth(c, "logout", user.ID, user.Username, fmt.Sprintf("User %s logged out", user.Username))
				}
			} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				revokeErr = lookupErr
				logger.Error("Failed to load user session during logout",
					zap.String("user_id", claims.UserID),
					zap.Error(lookupErr))
			}
		}
	}

	// 清除Cookie（自动检测 HTTPS 并设置 Secure 标志）
	s.setCookie(c, "token", "", -1)
	if revokeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to revoke session",
		})
		return
	}

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

	// 验证令牌并确认用户仍存在、启用且会话版本有效
	user, _, err := AuthenticateToken(s.db, tokenString)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "Invalid or expired token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"user": buildAuthUserPayload(user),
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

		// 验证令牌，并将JWT与当前用户状态/会话版本绑定。
		user, claims, err := AuthenticateToken(s.db, tokenString)
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
		c.Set("user_id", user.ID)
		c.Set("userID", user.ID)
		c.Set("user", user)
		c.Set("token_version", claims.TokenVersion)

		c.Next()
	}
}
