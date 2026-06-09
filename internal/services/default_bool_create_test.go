package services

import (
	"testing"

	"superview/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupDefaultBoolCreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.Node{},
		&models.AlertRule{},
		&models.NotificationChannel{},
		&models.EnvironmentVariable{},
		&models.ConfigurationAudit{},
		&models.ProcessGroup{},
		&models.ScheduledTask{},
		&models.LogAnalysisRule{},
		&models.LogRetentionPolicy{},
	); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TestAlertServiceCreateAlertRulePreservesExplicitDisabledState(t *testing.T) {
	db := setupDefaultBoolCreateTestDB(t)
	service := NewAlertService(db)

	rule := &models.AlertRule{
		Name:      "cpu-disabled",
		Metric:    "cpu",
		Condition: ">",
		Threshold: 90,
		Duration:  5,
		Severity:  models.AlertSeverityHigh,
		Enabled:   false,
		CreatedBy: "tester",
	}

	if err := service.CreateAlertRule(rule); err != nil {
		t.Fatalf("failed to create alert rule: %v", err)
	}

	var saved models.AlertRule
	if err := db.First(&saved, rule.ID).Error; err != nil {
		t.Fatalf("failed to reload alert rule: %v", err)
	}
	if saved.Enabled {
		t.Fatal("expected alert rule to remain disabled when enabled=false is provided")
	}
}

func TestAlertServiceCreateNotificationChannelPreservesExplicitDisabledState(t *testing.T) {
	db := setupDefaultBoolCreateTestDB(t)
	service := NewAlertService(db)

	channel := &models.NotificationChannel{
		Name:      "webhook-disabled",
		Type:      models.ChannelTypeWebhook,
		Config:    `{"url":"https://example.test/hook"}`,
		Enabled:   false,
		CreatedBy: "tester",
	}

	if err := service.CreateNotificationChannel(channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	var saved models.NotificationChannel
	if err := db.First(&saved, channel.ID).Error; err != nil {
		t.Fatalf("failed to reload notification channel: %v", err)
	}
	if saved.Enabled {
		t.Fatal("expected notification channel to remain disabled when enabled=false is provided")
	}
}

func TestConfigurationServiceCreateEnvironmentVariablePreservesExplicitInactiveState(t *testing.T) {
	db := setupDefaultBoolCreateTestDB(t)
	service := NewConfigurationService(db)

	envVar := &models.EnvironmentVariable{
		Name:      "FEATURE_FLAG",
		Value:     "off",
		IsActive:  false,
		CreatedBy: "tester",
	}

	if err := service.CreateEnvironmentVariable(envVar); err != nil {
		t.Fatalf("failed to create environment variable: %v", err)
	}

	var saved models.EnvironmentVariable
	if err := db.First(&saved, envVar.ID).Error; err != nil {
		t.Fatalf("failed to reload environment variable: %v", err)
	}
	if saved.IsActive {
		t.Fatal("expected environment variable to remain inactive when is_active=false is provided")
	}
}

func TestProcessEnhancedServiceCreateProcessGroupPreservesExplicitDisabledState(t *testing.T) {
	db := setupDefaultBoolCreateTestDB(t)
	service := NewProcessEnhancedService(db)

	group := &models.ProcessGroup{
		Name:      "group-disabled-service",
		Enabled:   false,
		CreatedBy: "tester",
	}

	if err := service.CreateProcessGroup(group); err != nil {
		t.Fatalf("failed to create process group: %v", err)
	}

	var saved models.ProcessGroup
	if err := db.First(&saved, group.ID).Error; err != nil {
		t.Fatalf("failed to reload process group: %v", err)
	}
	if saved.Enabled {
		t.Fatal("expected process group to remain disabled when enabled=false is provided")
	}
}

func TestProcessEnhancedServiceCreateScheduledTaskPreservesExplicitDisabledState(t *testing.T) {
	db := setupDefaultBoolCreateTestDB(t)
	service := NewProcessEnhancedService(db)

	task := &models.ScheduledTask{
		Name:       "task-disabled",
		TaskType:   models.TaskTypeCustomCommand,
		TargetType: models.TargetTypeNode,
		TargetID:   "node-1",
		CronExpr:   "*/5 * * * *",
		Enabled:    false,
		CreatedBy:  "tester",
	}

	if err := service.CreateScheduledTask(task); err != nil {
		t.Fatalf("failed to create scheduled task: %v", err)
	}

	var saved models.ScheduledTask
	if err := db.First(&saved, task.ID).Error; err != nil {
		t.Fatalf("failed to reload scheduled task: %v", err)
	}
	if saved.Enabled {
		t.Fatal("expected scheduled task to remain disabled when enabled=false is provided")
	}
	if saved.NextRun == nil {
		t.Fatal("expected scheduled task to calculate next run")
	}
}

func TestProcessEnhancedServiceStartSchedulerLoadsStandardCronTasks(t *testing.T) {
	db := setupDefaultBoolCreateTestDB(t)
	service := NewProcessEnhancedService(db)
	t.Cleanup(service.StopScheduler)

	task := &models.ScheduledTask{
		Name:       "task-standard-cron",
		TaskType:   models.TaskTypeCustomCommand,
		TargetType: models.TargetTypeNode,
		TargetID:   "node-1",
		CronExpr:   "*/5 * * * *",
		Enabled:    true,
		CreatedBy:  "tester",
	}

	if err := service.CreateScheduledTask(task); err != nil {
		t.Fatalf("failed to create scheduled task: %v", err)
	}

	if err := service.StartScheduler(); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}

	if !service.scheduler.running {
		t.Fatal("expected scheduler to be marked as running")
	}
	if len(service.cronJob.Entries()) != 1 {
		t.Fatalf("expected exactly one scheduled cron entry, got %d", len(service.cronJob.Entries()))
	}
}

func TestProcessEnhancedServiceUpdateScheduledTaskRejectsInvalidCronExpr(t *testing.T) {
	db := setupDefaultBoolCreateTestDB(t)
	service := NewProcessEnhancedService(db)

	task := &models.ScheduledTask{
		Name:       "task-update-cron",
		TaskType:   models.TaskTypeCustomCommand,
		TargetType: models.TargetTypeNode,
		TargetID:   "node-1",
		CronExpr:   "*/5 * * * *",
		Enabled:    true,
		CreatedBy:  "tester",
	}

	if err := service.CreateScheduledTask(task); err != nil {
		t.Fatalf("failed to create scheduled task: %v", err)
	}

	if err := service.UpdateScheduledTask(task.ID, map[string]interface{}{"cron_expr": "invalid-cron"}); err == nil {
		t.Fatal("expected invalid cron expression update to be rejected")
	}
}

func TestLogAnalysisServiceCreateAnalysisRulePreservesExplicitInactiveState(t *testing.T) {
	db := setupDefaultBoolCreateTestDB(t)
	service := NewLogAnalysisService(db)

	rule := &models.LogAnalysisRule{
		Name:        "panic-detector",
		Pattern:     "panic",
		PatternType: models.PatternTypeContains,
		IsActive:    false,
		CreatedBy:   "tester",
	}

	if err := service.CreateAnalysisRule(rule); err != nil {
		t.Fatalf("failed to create analysis rule: %v", err)
	}

	var saved models.LogAnalysisRule
	if err := db.First(&saved, rule.ID).Error; err != nil {
		t.Fatalf("failed to reload analysis rule: %v", err)
	}
	if saved.IsActive {
		t.Fatal("expected analysis rule to remain inactive when is_active=false is provided")
	}
}

func TestLogAnalysisServiceCreateRetentionPolicyPreservesExplicitInactiveState(t *testing.T) {
	db := setupDefaultBoolCreateTestDB(t)
	service := NewLogAnalysisService(db)

	policy := &models.LogRetentionPolicy{
		Name:          "archive-only",
		Conditions:    `{"level":["info"]}`,
		RetentionDays: 30,
		IsActive:      false,
		CreatedBy:     "tester",
	}

	if err := service.CreateRetentionPolicy(policy); err != nil {
		t.Fatalf("failed to create retention policy: %v", err)
	}

	var saved models.LogRetentionPolicy
	if err := db.First(&saved, policy.ID).Error; err != nil {
		t.Fatalf("failed to reload retention policy: %v", err)
	}
	if saved.IsActive {
		t.Fatal("expected retention policy to remain inactive when is_active=false is provided")
	}
}
