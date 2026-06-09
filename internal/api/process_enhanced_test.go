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

func setupProcessEnhancedTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&models.User{}, &models.ProcessGroup{}, &models.ScheduledTask{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TestCreateProcessGroupDefaultsEnabledToTrueWhenOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupProcessEnhancedTestDB(t)
	handler := NewProcessEnhancedHandler(db)

	body := []byte(`{"name":"group-default","description":"desc","priority":1}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/process-enhanced/groups", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("user_id", "creator-1")

	handler.CreateProcessGroup(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", recorder.Code)
	}

	var group models.ProcessGroup
	if err := db.First(&group, "name = ?", "group-default").Error; err != nil {
		t.Fatalf("failed to load created process group: %v", err)
	}
	if !group.Enabled {
		t.Fatal("expected process group to default to enabled=true when omitted")
	}
}

func TestCreateProcessGroupAllowsExplicitDisabledState(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupProcessEnhancedTestDB(t)
	handler := NewProcessEnhancedHandler(db)

	body := []byte(`{"name":"group-disabled","description":"desc","priority":1,"enabled":false}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/process-enhanced/groups", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("user_id", "creator-1")

	handler.CreateProcessGroup(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", recorder.Code)
	}

	var group models.ProcessGroup
	if err := db.First(&group, "name = ?", "group-disabled").Error; err != nil {
		t.Fatalf("failed to load created process group: %v", err)
	}
	if group.Enabled {
		t.Fatal("expected process group to remain disabled when enabled=false is provided")
	}
}

func TestCreateScheduledTaskRejectsInvalidCronExpression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupProcessEnhancedTestDB(t)
	handler := NewProcessEnhancedHandler(db)

	body := []byte(`{"name":"task-invalid","cron_expr":"invalid-cron","task_type":"custom_command","target_type":"node","target_id":"node-1"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/process-enhanced/scheduled-tasks", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("user_id", "creator-1")

	handler.CreateScheduledTask(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestUpdateScheduledTaskRejectsInvalidCronExpression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupProcessEnhancedTestDB(t)
	handler := NewProcessEnhancedHandler(db)

	task := &models.ScheduledTask{
		Name:       "task-valid",
		CronExpr:   "*/5 * * * *",
		TaskType:   models.TaskTypeCustomCommand,
		TargetType: models.TargetTypeNode,
		TargetID:   "node-1",
		Enabled:    true,
		CreatedBy:  "creator-1",
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to seed scheduled task: %v", err)
	}

	body := []byte(`{"cron_expr":"invalid-cron"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/process-enhanced/scheduled-tasks/1", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateScheduledTask(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}
