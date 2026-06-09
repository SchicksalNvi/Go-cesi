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
