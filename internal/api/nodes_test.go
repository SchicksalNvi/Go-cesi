package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"superview/internal/models"
	"superview/internal/supervisor"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupNodesAPITestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&models.Node{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TestFilterNodesByAccess(t *testing.T) {
	nodes := []*supervisor.Node{
		{Name: "node-a"},
		{Name: "node-b"},
	}
	nodeIDsByName := map[string]uint{
		"node-a": 1,
		"node-b": 2,
	}
	user := &models.User{
		NodeAccess: []models.NodeAccess{
			{NodeID: 1, CanRead: true},
		},
	}

	filtered := filterNodesByAccess(nodes, nodeIDsByName, user)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 node after filtering, got %d", len(filtered))
	}
	if filtered[0].Name != "node-a" {
		t.Fatalf("expected node-a to remain, got %s", filtered[0].Name)
	}
}

func TestAuthorizeNodeAccessRejectsUnauthorizedNode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupNodesAPITestDB(t)
	node := &models.Node{
		Name: "node-a",
		Host: "127.0.0.1",
		Port: 9001,
	}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	api := NewNodesAPI(nil, db)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/nodes/node-a", nil)
	ctx.Params = gin.Params{{Key: "node_name", Value: "node-a"}}
	ctx.Set("user", &models.User{
		NodeAccess: []models.NodeAccess{
			{NodeID: node.ID, CanRead: false},
		},
	})

	if _, ok := api.authorizeNodeAccess(ctx, "read"); ok {
		t.Fatal("expected authorization to fail")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}

func TestAuthorizeNodeAccessAllowsScopedNode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupNodesAPITestDB(t)
	node := &models.Node{
		Name: "node-a",
		Host: "127.0.0.1",
		Port: 9001,
	}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	api := NewNodesAPI(nil, db)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/nodes/node-a", nil)
	ctx.Params = gin.Params{{Key: "node_name", Value: "node-a"}}
	ctx.Set("user", &models.User{
		NodeAccess: []models.NodeAccess{
			{NodeID: node.ID, CanRead: true},
		},
	})

	authorizedNode, ok := api.authorizeNodeAccess(ctx, "read")
	if !ok {
		t.Fatal("expected authorization to succeed")
	}
	if authorizedNode.ID != node.ID {
		t.Fatalf("expected node ID %d, got %d", node.ID, authorizedNode.ID)
	}
}
