package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"superview/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupUsersHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
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

func TestDeleteUserRejectsDeletingSelf(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	currentUser := &models.User{
		Username: "self-user",
		Email:    "self@example.com",
	}
	if err := currentUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if err := db.Create(currentUser).Error; err != nil {
		t.Fatalf("failed to create current user: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/users/"+currentUser.ID, nil)
	ctx.Params = gin.Params{{Key: "id", Value: currentUser.ID}}
	ctx.Set("user", currentUser)

	handler.DeleteUser(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}

func TestDeleteUserRejectsDeletingSuperAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	currentUser := &models.User{
		Username: "operator",
		Email:    "operator@example.com",
	}
	if err := currentUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash current user password: %v", err)
	}
	if err := db.Create(currentUser).Error; err != nil {
		t.Fatalf("failed to create current user: %v", err)
	}

	targetUser := &models.User{
		Username: "superadmin",
		Email:    "superadmin@example.com",
	}
	if err := targetUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash target user password: %v", err)
	}
	if err := db.Create(targetUser).Error; err != nil {
		t.Fatalf("failed to create target user: %v", err)
	}

	superAdminRole := &models.Role{
		ID:   "role-super-admin",
		Name: models.RoleSuperAdmin,
	}
	if err := db.Create(superAdminRole).Error; err != nil {
		t.Fatalf("failed to create super admin role: %v", err)
	}
	if err := db.Model(targetUser).Association("Roles").Append(superAdminRole); err != nil {
		t.Fatalf("failed to assign super admin role: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/users/"+targetUser.ID, nil)
	ctx.Params = gin.Params{{Key: "id", Value: targetUser.ID}}
	ctx.Set("user", currentUser)

	handler.DeleteUser(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}

	var remaining models.User
	if err := db.First(&remaining, "id = ?", targetUser.ID).Error; err != nil {
		t.Fatalf("expected target user to remain in database: %v", err)
	}
}

func TestUpdateUserDoesNotClearBoolFieldsWhenOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	currentUser := &models.User{
		Username: "operator",
		Email:    "operator@example.com",
	}
	if err := currentUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash current user password: %v", err)
	}
	if err := db.Create(currentUser).Error; err != nil {
		t.Fatalf("failed to create current user: %v", err)
	}

	targetUser := &models.User{
		Username: "target",
		Email:    "target@example.com",
		FullName: "Before Update",
		IsAdmin:  true,
		IsActive: true,
	}
	if err := targetUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash target user password: %v", err)
	}
	if err := db.Create(targetUser).Error; err != nil {
		t.Fatalf("failed to create target user: %v", err)
	}

	body := []byte(`{"full_name":"After Update"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/users/"+targetUser.ID, bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: targetUser.ID}}
	ctx.Set("user", currentUser)

	handler.UpdateUser(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var updated models.User
	if err := db.First(&updated, "id = ?", targetUser.ID).Error; err != nil {
		t.Fatalf("failed to reload updated user: %v", err)
	}
	if !updated.IsAdmin {
		t.Fatal("expected IsAdmin to remain true when is_admin is omitted")
	}
	if !updated.IsActive {
		t.Fatal("expected IsActive to remain true when is_active is omitted")
	}
	if updated.FullName != "After Update" {
		t.Fatalf("expected FullName to be updated, got %q", updated.FullName)
	}
}

func TestToggleUserStatusRejectsDeactivatingSelf(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	currentUser := &models.User{
		Username: "self-user",
		Email:    "self@example.com",
		IsActive: true,
	}
	if err := currentUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if err := db.Create(currentUser).Error; err != nil {
		t.Fatalf("failed to create current user: %v", err)
	}

	body := []byte(`{"is_active":false}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/users/"+currentUser.ID+"/toggle", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: currentUser.ID}}
	ctx.Set("user", currentUser)

	handler.ToggleUserStatus(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}

func TestToggleUserStatusRejectsDeactivatingSuperAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	currentUser := &models.User{
		Username: "operator",
		Email:    "operator@example.com",
		IsActive: true,
	}
	if err := currentUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash current user password: %v", err)
	}
	if err := db.Create(currentUser).Error; err != nil {
		t.Fatalf("failed to create current user: %v", err)
	}

	targetUser := &models.User{
		Username: "legacy-admin",
		Email:    "legacy-admin@example.com",
		IsAdmin:  true,
		IsActive: true,
	}
	if err := targetUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash target user password: %v", err)
	}
	if err := db.Create(targetUser).Error; err != nil {
		t.Fatalf("failed to create target user: %v", err)
	}

	body := []byte(`{"is_active":false}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/users/"+targetUser.ID+"/toggle", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: targetUser.ID}}
	ctx.Set("user", currentUser)

	handler.ToggleUserStatus(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}

func TestUpdateUserNodeAccessReplacesEntriesAndPreservesExplicitFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	currentUser := &models.User{Username: "operator", Email: "operator@example.com"}
	if err := currentUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash current user password: %v", err)
	}
	if err := db.Create(currentUser).Error; err != nil {
		t.Fatalf("failed to create current user: %v", err)
	}

	targetUser := &models.User{Username: "target", Email: "target@example.com"}
	if err := targetUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash target user password: %v", err)
	}
	if err := db.Create(targetUser).Error; err != nil {
		t.Fatalf("failed to create target user: %v", err)
	}

	node1 := &models.Node{Name: "node-1", Host: "127.0.0.1", Port: 9001}
	node2 := &models.Node{Name: "node-2", Host: "127.0.0.2", Port: 9002}
	if err := db.Create(node1).Error; err != nil {
		t.Fatalf("failed to create node1: %v", err)
	}
	if err := db.Create(node2).Error; err != nil {
		t.Fatalf("failed to create node2: %v", err)
	}

	if err := db.Create(&models.NodeAccess{
		UserID:    targetUser.ID,
		NodeID:    node1.ID,
		CanRead:   true,
		CanWrite:  false,
		CanDelete: false,
	}).Error; err != nil {
		t.Fatalf("failed to seed existing node access: %v", err)
	}

	body := []byte(`{"node_access":[{"node_id":2,"can_read":false,"can_write":true,"can_delete":false}]}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/users/"+targetUser.ID+"/node-access", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: targetUser.ID}}

	handler.UpdateUserNodeAccess(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var accesses []models.NodeAccess
	if err := db.Where("user_id = ?", targetUser.ID).Order("node_id ASC").Find(&accesses).Error; err != nil {
		t.Fatalf("failed to reload node access entries: %v", err)
	}
	if len(accesses) != 1 {
		t.Fatalf("expected exactly one node access entry after replace, got %d", len(accesses))
	}
	if accesses[0].NodeID != node2.ID {
		t.Fatalf("expected node access to be replaced with node %d, got %d", node2.ID, accesses[0].NodeID)
	}
	if accesses[0].CanRead {
		t.Fatal("expected can_read=false to be preserved")
	}
	if !accesses[0].CanWrite {
		t.Fatal("expected can_write=true to be preserved")
	}
	if accesses[0].CanDelete {
		t.Fatal("expected can_delete=false to be preserved")
	}
}

func TestUpdateUserNodeAccessRejectsUnknownNode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	targetUser := &models.User{Username: "target", Email: "target@example.com"}
	if err := targetUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash target user password: %v", err)
	}
	if err := db.Create(targetUser).Error; err != nil {
		t.Fatalf("failed to create target user: %v", err)
	}

	body := []byte(`{"node_access":[{"node_id":999,"can_read":true}]}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/users/"+targetUser.ID+"/node-access", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: targetUser.ID}}

	handler.UpdateUserNodeAccess(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestGetUserNodeAccessReturnsNodeDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	targetUser := &models.User{Username: "target", Email: "target@example.com"}
	if err := targetUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash target user password: %v", err)
	}
	if err := db.Create(targetUser).Error; err != nil {
		t.Fatalf("failed to create target user: %v", err)
	}

	node := &models.Node{Name: "node-1", Host: "127.0.0.1", Port: 9001}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	if err := db.Create(&models.NodeAccess{
		UserID:    targetUser.ID,
		NodeID:    node.ID,
		CanRead:   true,
		CanWrite:  true,
		CanDelete: false,
	}).Error; err != nil {
		t.Fatalf("failed to create node access: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/users/"+targetUser.ID+"/node-access", nil)
	ctx.Params = gin.Params{{Key: "id", Value: targetUser.ID}}

	handler.GetUserNodeAccess(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var response struct {
		Status string `json:"status"`
		Data   struct {
			UserID     string `json:"user_id"`
			NodeAccess []struct {
				NodeID uint `json:"node_id"`
				Node   struct {
					ID   uint   `json:"id"`
					Name string `json:"name"`
				} `json:"node"`
			} `json:"node_access"`
			AvailableNodes []struct {
				ID   uint   `json:"id"`
				Name string `json:"name"`
			} `json:"available_nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if response.Data.UserID != targetUser.ID {
		t.Fatalf("expected user_id %s, got %s", targetUser.ID, response.Data.UserID)
	}
	if len(response.Data.NodeAccess) != 1 {
		t.Fatalf("expected one node access entry, got %d", len(response.Data.NodeAccess))
	}
	if response.Data.NodeAccess[0].Node.ID != node.ID || response.Data.NodeAccess[0].Node.Name != node.Name {
		t.Fatalf("expected node details to be included, got %+v", response.Data.NodeAccess[0].Node)
	}
	if len(response.Data.AvailableNodes) != 1 {
		t.Fatalf("expected one available node entry, got %d", len(response.Data.AvailableNodes))
	}
	if response.Data.AvailableNodes[0].ID != node.ID || response.Data.AvailableNodes[0].Name != node.Name {
		t.Fatalf("expected available node details to be included, got %+v", response.Data.AvailableNodes[0])
	}
}

func TestUpdateUserRejectsRemovingOwnSuperAdminPrivileges(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	currentUser := &models.User{
		Username: "self-admin",
		Email:    "self-admin@example.com",
		IsAdmin:  true,
		IsActive: true,
	}
	if err := currentUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash current user password: %v", err)
	}
	if err := db.Create(currentUser).Error; err != nil {
		t.Fatalf("failed to create current user: %v", err)
	}

	body := []byte(`{"is_admin":false}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/users/"+currentUser.ID, bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: currentUser.ID}}
	ctx.Set("user", currentUser)

	handler.UpdateUser(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}

func TestUpdateUserRejectsRemovingSuperAdminPrivilegesFromTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUsersHandlerTestDB(t)
	handler := NewUserHandler(db)

	currentUser := &models.User{
		Username: "operator",
		Email:    "operator@example.com",
		IsActive: true,
	}
	if err := currentUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash current user password: %v", err)
	}
	if err := db.Create(currentUser).Error; err != nil {
		t.Fatalf("failed to create current user: %v", err)
	}

	targetUser := &models.User{
		Username: "legacy-admin",
		Email:    "legacy-admin@example.com",
		IsAdmin:  true,
		IsActive: true,
	}
	if err := targetUser.SetPassword("secret123"); err != nil {
		t.Fatalf("failed to hash target user password: %v", err)
	}
	if err := db.Create(targetUser).Error; err != nil {
		t.Fatalf("failed to create target user: %v", err)
	}

	body := []byte(`{"is_admin":false}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/users/"+targetUser.ID, bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: targetUser.ID}}
	ctx.Set("user", currentUser)

	handler.UpdateUser(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}
