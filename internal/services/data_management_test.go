package services

import (
	"testing"

	"superview/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupDataManagementTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&models.User{}, &models.BackupRecord{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TestUpdateBackupStatusPersistsFailureDetails(t *testing.T) {
	db := setupDataManagementTestDB(t)
	service := &DataManagementService{DB: db}

	record := &models.BackupRecord{
		ID:         "backup-1",
		Name:       "nightly",
		FilePath:   "",
		BackupType: models.BackupTypeFull,
		Status:     models.StatusPending,
		CreatedBy:  "user-1",
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("failed to seed backup record: %v", err)
	}

	service.updateBackupStatus(record, models.StatusFailed, "backup failed")

	var reloaded models.BackupRecord
	if err := db.First(&reloaded, "id = ?", record.ID).Error; err != nil {
		t.Fatalf("failed to reload backup record: %v", err)
	}

	if reloaded.Status != models.StatusFailed {
		t.Fatalf("expected status %q, got %q", models.StatusFailed, reloaded.Status)
	}
	if reloaded.ErrorMsg != "backup failed" {
		t.Fatalf("expected error message to persist, got %q", reloaded.ErrorMsg)
	}
	if reloaded.CompletedAt == nil {
		t.Fatal("expected completed_at to be set for failed backup")
	}
}
