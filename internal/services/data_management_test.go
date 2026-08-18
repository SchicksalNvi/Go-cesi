package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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

	if err := db.AutoMigrate(&models.User{}, &models.BackupRecord{}, &models.DataImportRecord{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

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

func TestStartImportCleansSourceFileAfterCompletion(t *testing.T) {
	db := setupDataManagementTestDB(t)
	service := &DataManagementService{DB: db}

	user := &models.User{ID: "user-1", Username: "importer", Password: "hashed", IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	filePath := filepath.Join(t.TempDir(), "configs.json")
	if err := os.WriteFile(filePath, []byte(`[{"key":"site_name"}]`), 0600); err != nil {
		t.Fatalf("failed to create import source: %v", err)
	}

	record, err := service.CreateImportRecord(models.ImportTypeConfigs, filePath, 21, user.ID)
	if err != nil {
		t.Fatalf("failed to create import record: %v", err)
	}
	if err := service.StartImport(record, func(context.Context) (int, string, error) {
		return 1, "validated", nil
	}); err != nil {
		t.Fatalf("failed to start import: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		var reloaded models.DataImportRecord
		if err := db.First(&reloaded, "id = ?", record.ID).Error; err != nil {
			t.Fatalf("failed to reload import record: %v", err)
		}
		if reloaded.Status == models.StatusCompleted && reloaded.SourceFile == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("import did not complete and clean its source: status=%s source=%q", reloaded.Status, reloaded.SourceFile)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected source file to be removed, stat error: %v", err)
	}
}

func TestRunDataJobRejectsWorkBeyondSharedCapacity(t *testing.T) {
	started := make(chan struct{}, maxConcurrentDataJobs)
	release := make(chan struct{})
	done := make(chan struct{}, maxConcurrentDataJobs)

	for i := 0; i < maxConcurrentDataJobs; i++ {
		id := string(rune('a' + i))
		if err := runDataJob("capacity-test", id, func(context.Context) {
			started <- struct{}{}
			<-release
			done <- struct{}{}
		}); err != nil {
			t.Fatalf("failed to start job %d: %v", i, err)
		}
	}

	for i := 0; i < maxConcurrentDataJobs; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for data job to start")
		}
	}

	if err := runDataJob("capacity-test", "overflow", func(context.Context) {}); !errors.Is(err, ErrDataJobCapacity) {
		t.Fatalf("expected ErrDataJobCapacity, got %v", err)
	}

	close(release)
	for i := 0; i < maxConcurrentDataJobs; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for data job to finish")
		}
	}

	deadline := time.Now().Add(time.Second)
	for len(dataJobSlots) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("data job slots were not released")
		}
		time.Sleep(time.Millisecond)
	}
}
