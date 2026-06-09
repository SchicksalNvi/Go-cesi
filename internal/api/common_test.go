package api

import (
	"testing"

	"superview/internal/models"

	"github.com/gin-gonic/gin"
)

func TestGetUserIDStringSupportsLegacyAndHydratedContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		setup    func(*gin.Context)
		expected string
		ok       bool
	}{
		{
			name: "prefers user_id context key",
			setup: func(c *gin.Context) {
				c.Set("user_id", "user-from-user-id")
				c.Set("userID", "user-from-userID")
			},
			expected: "user-from-user-id",
			ok:       true,
		},
		{
			name: "falls back to userID context key",
			setup: func(c *gin.Context) {
				c.Set("userID", "user-from-userID")
			},
			expected: "user-from-userID",
			ok:       true,
		},
		{
			name: "falls back to hydrated user object",
			setup: func(c *gin.Context) {
				c.Set("user", &models.User{ID: "user-from-user-object"})
			},
			expected: "user-from-user-object",
			ok:       true,
		},
		{
			name:     "returns false when no user context exists",
			setup:    func(c *gin.Context) {},
			expected: "",
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			tt.setup(c)

			got, ok := getUserIDString(c)
			if ok != tt.ok {
				t.Fatalf("expected ok=%v, got %v", tt.ok, ok)
			}
			if got != tt.expected {
				t.Fatalf("expected user ID %q, got %q", tt.expected, got)
			}
		})
	}
}
