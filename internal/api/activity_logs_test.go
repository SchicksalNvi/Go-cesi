package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"superview/internal/models"
	"superview/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// 自动迁移
	err = db.AutoMigrate(&models.ActivityLog{})
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}

func TestGetActivityLogs(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewActivityLogService(db)
	api := NewActivityLogsAPI(service)

	// 创建测试数据
	testLogs := []*models.ActivityLog{
		{
			Level:     "INFO",
			Message:   "Test log 1",
			Action:    "test_action",
			Resource:  "test",
			Target:    "test_target",
			Username:  "admin",
			IPAddress: "127.0.0.1",
		},
		{
			Level:     "ERROR",
			Message:   "Test log 2",
			Action:    "test_action",
			Resource:  "test",
			Target:    "test_target",
			Username:  "user",
			IPAddress: "127.0.0.2",
		},
	}

	for _, log := range testLogs {
		db.Create(log)
	}

	// 设置 Gin 测试模式
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/activity-logs", api.GetActivityLogs)

	// 测试获取所有日志
	t.Run("Get all logs", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/activity-logs?page=1&page_size=20", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "success", response["status"])
	})

	// 测试筛选
	t.Run("Filter by level", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/activity-logs?level=ERROR&page=1&page_size=20", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "success", response["status"])
	})

	t.Run("Filter by ISO time range", func(t *testing.T) {
		start := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
		end := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
		req, _ := http.NewRequest("GET", "/activity-logs?start_time="+url.QueryEscape(start)+"&end_time="+url.QueryEscape(end)+"&page=1&page_size=20", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "success", response["status"])
	})

	t.Run("Reject invalid time range filter", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/activity-logs?start_time=not-a-time&page=1&page_size=20", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "error", response["status"])
	})

	// 测试分页
	t.Run("Pagination", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/activity-logs?page=1&page_size=1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		data := response["data"].(map[string]interface{})
		pagination := data["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])
		assert.Equal(t, float64(1), pagination["page_size"])
	})
}

func TestGetRecentLogs(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewActivityLogService(db)
	api := NewActivityLogsAPI(service)

	// 创建测试数据
	for i := 0; i < 5; i++ {
		db.Create(&models.ActivityLog{
			Level:     "INFO",
			Message:   "Test log",
			Action:    "test",
			Resource:  "test",
			Target:    "test",
			Username:  "admin",
			IPAddress: "127.0.0.1",
		})
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/activity-logs/recent", api.GetRecentLogs)

	t.Run("Get recent logs with default limit", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/activity-logs/recent", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "success", response["status"])
	})

	t.Run("Get recent logs with custom limit", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/activity-logs/recent?limit=3", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "success", response["status"])
	})
}

func TestExportLogs(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewActivityLogService(db)
	api := NewActivityLogsAPI(service)

	// 创建测试数据
	db.Create(&models.ActivityLog{
		Level:     "INFO",
		Message:   "Test log",
		Action:    "test",
		Resource:  "test",
		Target:    "test",
		Username:  "admin",
		IPAddress: "127.0.0.1",
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/activity-logs/export", api.ExportLogs)

	t.Run("Export logs as CSV", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/activity-logs/export", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/csv", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
		assert.Contains(t, w.Body.String(), "ID,Created At,Level")
	})

	t.Run("Export logs with invalid time filter", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/activity-logs/export?start_time=bad-value", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestGetLogStatistics(t *testing.T) {
	db := setupTestDB(t)
	service := services.NewActivityLogService(db)
	api := NewActivityLogsAPI(service)

	// 创建测试数据
	levels := []string{"INFO", "WARNING", "ERROR"}
	for _, level := range levels {
		for i := 0; i < 3; i++ {
			db.Create(&models.ActivityLog{
				Level:     level,
				Message:   "Test log",
				Action:    "test",
				Resource:  "test",
				Target:    "test",
				Username:  "admin",
				IPAddress: "127.0.0.1",
			})
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/activity-logs/statistics", api.GetLogStatistics)

	t.Run("Get statistics", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/activity-logs/statistics?days=7", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "success", response["status"])

		data := response["data"].(map[string]interface{})
		assert.NotNil(t, data["total_logs"])
	})
}
