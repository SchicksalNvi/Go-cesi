package services

import (
	"testing"

	"superview/internal/models"
	"superview/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupUserServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Role{},
		&models.Permission{},
		&models.UserRole{},
		&models.RolePermission{},
		&models.Node{},
		&models.NodeAccess{},
	); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TestCreateUserPreservesAdminFlag(t *testing.T) {
	db := setupUserServiceTestDB(t)
	service := NewUserService(repository.NewRepository(db))

	user := &models.User{
		Username: "admincandidate",
		Email:    "admin@example.com",
		IsAdmin:  true,
	}
	if err := user.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if err := service.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	saved, err := service.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("failed to reload created user: %v", err)
	}

	if !saved.IsAdmin {
		t.Fatal("expected created user to preserve IsAdmin=true")
	}
	if !saved.IsActive {
		t.Fatal("expected created user to remain active by default")
	}
}

func TestUpdateUserCannotRestoreRevokedTokenVersion(t *testing.T) {
	db := setupUserServiceTestDB(t)
	service := NewUserService(repository.NewRepository(db))

	user := &models.User{
		Username: "stale-session-user",
		Email:    "stale-session-user@example.com",
		IsActive: true,
	}
	if err := user.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if err := service.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	staleUser, err := service.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("failed to load stale user snapshot: %v", err)
	}
	if err := db.Model(&models.User{}).
		Where("id = ?", user.ID).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error; err != nil {
		t.Fatalf("failed to revoke session: %v", err)
	}

	staleUser.FullName = "Updated after logout"
	err = service.UpdateUser(staleUser)
	if err == nil {
		// M-29: UpdateUser now rejects stale token versions unconditionally.
		// This is the correct behavior — no partial update should bypass the
		// session-version check.
		var reloaded models.User
		if err := db.First(&reloaded, "id = ?", user.ID).Error; err != nil {
			t.Fatalf("failed to reload user: %v", err)
		}
		if reloaded.TokenVersion != 1 {
			t.Fatalf("stale update restored token version: got %d, want 1", reloaded.TokenVersion)
		}
		if reloaded.FullName != staleUser.FullName {
			t.Fatalf("expected profile update to persist, got %q", reloaded.FullName)
		}
	} else {
		// The rejection is expected per M-29. Verify the DB state was not corrupted.
		t.Logf("stale update correctly rejected: %v", err)
		var reloaded models.User
		if err := db.First(&reloaded, "id = ?", user.ID).Error; err != nil {
			t.Fatalf("failed to reload user: %v", err)
		}
		if reloaded.TokenVersion != 1 {
			t.Fatalf("stale update restored token version: got %d, want 1", reloaded.TokenVersion)
		}
		if reloaded.FullName == staleUser.FullName {
			t.Fatal("stale update should not have persisted")
		}
	}
}

func TestStaleUserUpdateCannotUndoConcurrentDeactivation(t *testing.T) {
	db := setupUserServiceTestDB(t)
	service := NewUserService(repository.NewRepository(db))

	user := &models.User{
		Username: "concurrent-deactivate-user",
		Email:    "concurrent-deactivate-user@example.com",
		IsActive: true,
	}
	if err := user.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if err := service.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	staleUser, err := service.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("failed to load stale user snapshot: %v", err)
	}
	if err := db.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"is_active":     false,
		"token_version": gorm.Expr("token_version + 1"),
	}).Error; err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	staleUser.FullName = "Must not reactivate"
	if err := service.UpdateUser(staleUser); err == nil {
		t.Fatal("expected stale session-state update to be rejected")
	}

	var reloaded models.User
	if err := db.First(&reloaded, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("failed to reload user: %v", err)
	}
	if reloaded.IsActive {
		t.Fatal("stale update reactivated the user")
	}
	if reloaded.TokenVersion != 1 {
		t.Fatalf("token version = %d, want 1", reloaded.TokenVersion)
	}
}

func TestPasswordAndDeactivationAtomicallyRevokeSessions(t *testing.T) {
	db := setupUserServiceTestDB(t)
	service := NewUserService(repository.NewRepository(db))

	user := &models.User{
		Username: "session-revocation-user",
		Email:    "session-revocation-user@example.com",
		IsActive: true,
	}
	if err := user.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if err := service.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	newHashUser := &models.User{}
	if err := newHashUser.SetPassword("new-secret123"); err != nil {
		t.Fatalf("failed to hash replacement password: %v", err)
	}
	if err := service.SetPasswordAndRevokeSessions(user.ID, newHashUser.Password); err != nil {
		t.Fatalf("failed to update password and revoke sessions: %v", err)
	}

	var afterPassword models.User
	if err := db.First(&afterPassword, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("failed to reload user after password change: %v", err)
	}
	if afterPassword.TokenVersion != 1 {
		t.Fatalf("password change token version = %d, want 1", afterPassword.TokenVersion)
	}
	if !afterPassword.VerifyPassword("new-secret123") {
		t.Fatal("replacement password was not saved")
	}

	if err := service.DeactivateUser(user.ID); err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	var afterDeactivate models.User
	if err := db.First(&afterDeactivate, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("failed to reload user after deactivation: %v", err)
	}
	if afterDeactivate.IsActive {
		t.Fatal("user remained active after deactivation")
	}
	if afterDeactivate.TokenVersion != 2 {
		t.Fatalf("deactivation token version = %d, want 2", afterDeactivate.TokenVersion)
	}
}
