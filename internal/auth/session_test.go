package auth

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"superview/internal/models"
)

const sessionTestPassword = "valid-password-123"

func setupAuthSessionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	return db
}

func createAuthSessionTestUser(t *testing.T, db *gorm.DB, username string) *models.User {
	t.Helper()

	user := &models.User{
		Username: username,
		Email:    username + "@example.com",
		IsActive: true,
	}
	if err := user.SetPassword(sessionTestPassword); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func performAuthenticatedRequest(router http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestLoginRejectsInactiveUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	db := setupAuthSessionTestDB(t)
	user := createAuthSessionTestUser(t, db, "inactive-login")
	if err := db.Model(user).UpdateColumn("is_active", false).Error; err != nil {
		t.Fatalf("disable user: %v", err)
	}

	router := gin.New()
	router.POST("/api/auth/login", NewAuthService(db).Login)
	body := bytes.NewBufferString(`{"username":"inactive-login","password":"valid-password-123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected inactive login to be forbidden, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"token"`) {
		t.Fatal("inactive login returned a token")
	}
}

func TestAuthMiddlewareRejectsUnavailableOrRevokedUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	db := setupAuthSessionTestDB(t)
	user := createAuthSessionTestUser(t, db, "session-user")
	token, err := GenerateToken(user.ID, user.TokenVersion)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	router := gin.New()
	router.Use(NewAuthService(db).AuthMiddleware())
	router.GET("/api/protected", func(c *gin.Context) {
		if c.GetString("user_id") != user.ID {
			c.Status(http.StatusInternalServerError)
			return
		}
		if _, exists := c.Get("user"); !exists {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	if w := performAuthenticatedRequest(router, token); w.Code != http.StatusOK {
		t.Fatalf("expected active session to pass, got %d: %s", w.Code, w.Body.String())
	}

	if err := db.Model(user).Updates(map[string]interface{}{
		"is_active":     false,
		"token_version": gorm.Expr("token_version + 1"),
	}).Error; err != nil {
		t.Fatalf("disable and revoke user: %v", err)
	}
	if w := performAuthenticatedRequest(router, token); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled user token to be rejected, got %d: %s", w.Code, w.Body.String())
	}

	if err := db.Model(user).UpdateColumn("is_active", true).Error; err != nil {
		t.Fatalf("reactivate user: %v", err)
	}
	if w := performAuthenticatedRequest(router, token); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected old token to remain revoked after reactivation, got %d: %s", w.Code, w.Body.String())
	}

	var reloaded models.User
	if err := db.First(&reloaded, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	newToken, err := GenerateToken(reloaded.ID, reloaded.TokenVersion)
	if err != nil {
		t.Fatalf("generate replacement token: %v", err)
	}
	if w := performAuthenticatedRequest(router, newToken); w.Code != http.StatusOK {
		t.Fatalf("expected replacement token to pass, got %d: %s", w.Code, w.Body.String())
	}

	if err := db.Delete(&reloaded).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if w := performAuthenticatedRequest(router, newToken); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected deleted user token to be rejected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogoutRevokesCurrentTokenVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	db := setupAuthSessionTestDB(t)
	user := createAuthSessionTestUser(t, db, "logout-user")
	token, err := GenerateToken(user.ID, user.TokenVersion)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	router := gin.New()
	router.POST("/api/auth/logout", NewAuthService(db).Logout)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected logout success, got %d: %s", w.Code, w.Body.String())
	}

	if err := ValidateUserSession(db, user.ID, user.TokenVersion); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatalf("expected logged-out token version to be revoked, got %v", err)
	}
}
