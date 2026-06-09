package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"superview/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSystemSettingsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&models.User{}, &models.UserPreferences{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TestUpdateUserPreferencesCreatesRecordWithExplicitFalseValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupSystemSettingsTestDB(t)
	handler := NewSystemSettingsAPI(db)

	body := []byte(`{"auto_refresh":false,"email_notifications":false,"process_alerts":false,"system_alerts":false,"node_status_changes":true,"weekly_report":true}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/system-settings/user-preferences", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("user_id", "user-1")

	handler.UpdateUserPreferences(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var preferences models.UserPreferences
	if err := db.Where("user_id = ?", "user-1").First(&preferences).Error; err != nil {
		t.Fatalf("failed to reload user preferences: %v", err)
	}

	if preferences.AutoRefresh {
		t.Fatal("expected auto_refresh=false to be preserved on create")
	}
	if preferences.EmailNotifications {
		t.Fatal("expected email_notifications=false to be preserved on create")
	}
	if preferences.ProcessAlerts {
		t.Fatal("expected process_alerts=false to be preserved on create")
	}
	if preferences.SystemAlerts {
		t.Fatal("expected system_alerts=false to be preserved on create")
	}
	if !preferences.NodeStatusChanges {
		t.Fatal("expected node_status_changes=true to be preserved on create")
	}
	if !preferences.WeeklyReport {
		t.Fatal("expected weekly_report=true to be preserved on create")
	}
}

func TestUpdateUserPreferencesKeepsOmittedBooleanFieldsUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupSystemSettingsTestDB(t)
	handler := NewSystemSettingsAPI(db)

	existing := &models.UserPreferences{
		UserID:             "user-1",
		Theme:              "light",
		Language:           "en",
		Timezone:           "UTC",
		DateFormat:         "YYYY-MM-DD",
		TimeFormat:         "HH:mm:ss",
		PageSize:           20,
		AutoRefresh:        true,
		RefreshInterval:    30,
		EmailNotifications: true,
		ProcessAlerts:      true,
		SystemAlerts:       true,
		NodeStatusChanges:  true,
		WeeklyReport:       true,
	}
	if err := db.Create(existing).Error; err != nil {
		t.Fatalf("failed to seed user preferences: %v", err)
	}

	body := []byte(`{"theme":"dark"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/system-settings/user-preferences", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("user_id", "user-1")

	handler.UpdateUserPreferences(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var preferences models.UserPreferences
	if err := db.Where("user_id = ?", "user-1").First(&preferences).Error; err != nil {
		t.Fatalf("failed to reload user preferences: %v", err)
	}

	if preferences.Theme != "dark" {
		t.Fatalf("expected theme to update to dark, got %q", preferences.Theme)
	}
	if !preferences.AutoRefresh {
		t.Fatal("expected omitted auto_refresh to remain true")
	}
	if !preferences.EmailNotifications {
		t.Fatal("expected omitted email_notifications to remain true")
	}
	if !preferences.ProcessAlerts {
		t.Fatal("expected omitted process_alerts to remain true")
	}
	if !preferences.SystemAlerts {
		t.Fatal("expected omitted system_alerts to remain true")
	}
	if !preferences.NodeStatusChanges {
		t.Fatal("expected omitted node_status_changes to remain true")
	}
	if !preferences.WeeklyReport {
		t.Fatal("expected omitted weekly_report to remain true")
	}
}
