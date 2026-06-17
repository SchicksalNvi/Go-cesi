package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCreateBackupRejectsUnsupportedIncrementalType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewDataManagementAPI()

	body := []byte(`{"name":"nightly","backup_type":"incremental"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/data-management/backups", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("user_id", "user-1")

	handler.CreateBackup(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestExportDataRejectsUnsupportedXLSXFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewDataManagementAPI()

	body := []byte(`{"export_type":"users","format":"xlsx"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/data-management/exports", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("user_id", "user-1")

	handler.ExportData(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestExportDataRejectsUnsupportedFormatForProcesses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewDataManagementAPI()

	body := []byte(`{"export_type":"processes","format":"csv"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/data-management/exports", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("user_id", "user-1")

	handler.ExportData(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestImportDataRejectsUnsupportedUsersImport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewDataManagementAPI()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("import_type", "users"); err != nil {
		t.Fatalf("failed to write import_type: %v", err)
	}
	part, err := writer.CreateFormFile("file", "users.json")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write([]byte(`[]`)); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/data-management/imports", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Set("user_id", "user-1")

	handler.ImportData(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestNormalizeConfigImportPayloadSupportsConfigurationArray(t *testing.T) {
	payload, total, validationLog, err := normalizeConfigImportPayload([]byte(`[{"key":"site_name","value":"demo"}]`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	configs, ok := payload["configurations"].([]interface{})
	if !ok || len(configs) != 1 {
		t.Fatal("expected one configuration entry")
	}
	if validationLog == "" {
		t.Fatal("expected validation log to be populated")
	}
}

func TestNormalizeConfigImportPayloadSupportsConfigurationExportShape(t *testing.T) {
	payload, total, _, err := normalizeConfigImportPayload([]byte(`{"configurations":[{"key":"site_name","value":"demo"}],"environment_variables":[{"name":"API_KEY","value":"123"}]}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if _, ok := payload["configurations"]; !ok {
		t.Fatal("expected configurations to be preserved")
	}
	if _, ok := payload["environment_variables"]; !ok {
		t.Fatal("expected environment_variables to be preserved")
	}
}
