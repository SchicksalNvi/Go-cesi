package api

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)
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

type oneByteReader struct {
	reader io.Reader
}

func (r oneByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.reader.Read(p)
}

func TestDecodeConfigImportPayloadSupportsChunkedReader(t *testing.T) {
	reader := oneByteReader{reader: strings.NewReader(`{"data":{"configs":[{"key":"site_name","value":"demo"}]}}`)}

	payload, total, _, err := decodeConfigImportPayload(reader)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	configs, ok := payload["configurations"].([]interface{})
	if !ok || len(configs) != 1 {
		t.Fatal("expected one streamed configuration entry")
	}
}

func TestDecodeConfigImportPayloadRejectsTrailingJSON(t *testing.T) {
	_, _, _, err := decodeConfigImportPayload(strings.NewReader(`[{"key":"site_name"}] {}`))
	if err == nil {
		t.Fatal("expected trailing JSON to be rejected")
	}
}

func TestParseImportUploadRejectsFileOverLimit(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "configs.json")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write([]byte("12345")); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/data-management/imports", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	defer func() {
		if ctx.Request.MultipartForm != nil {
			_ = ctx.Request.MultipartForm.RemoveAll()
		}
	}()

	_, err = parseImportUpload(ctx, 4)
	if !errors.Is(err, errImportTooLarge) {
		t.Fatalf("expected errImportTooLarge, got %v", err)
	}
}

func TestImportDataRejectsOversizedRequestBeforeParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewDataManagementAPI()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "configs.json")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write([]byte(`[]`)); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/data-management/imports", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Request.ContentLength = maxImportRequestSize + 1

	handler.ImportData(ctx)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d", recorder.Code)
	}
}

func TestDecodeConfigImportPayloadSupportsConfigsAlias(t *testing.T) {
	// L-11: 顶层 configs 别名分支(旧导出格式 { "configs": [...] })
	payload, total, _, err := decodeConfigImportPayload(strings.NewReader(`{"configs":[{"key":"site_name","value":"demo"}]}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	configs, ok := payload["configurations"].([]interface{})
	if !ok || len(configs) != 1 {
		t.Fatal("expected configs alias to be normalized into configurations")
	}
}

func TestDecodeConfigImportPayloadMissingEnvironmentVars(t *testing.T) {
	// L-11: environment_variables 缺失时不应报错,且不应出现在 payload 中
	payload, total, _, err := decodeConfigImportPayload(strings.NewReader(`{"configurations":[{"key":"site_name","value":"demo"}]}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if _, ok := payload["environment_variables"]; ok {
		t.Fatal("expected environment_variables to be absent when not provided")
	}
}

func TestDecodeConfigImportPayloadNestedDataArray(t *testing.T) {
	// L-11: 对象内嵌套 data 为非对象(数组)时应返回明确错误而非 panic
	_, _, _, err := decodeConfigImportPayload(strings.NewReader(`{"data":[{"key":"site_name"}]}`))
	if err == nil {
		t.Fatal("expected error for non-object nested data")
	}
}

func TestDecodeConfigImportPayloadPreservesNumericTypes(t *testing.T) {
	// M-33 回归:UseNumber 解码后数字必须还原为数值类型,不能变成带引号字符串
	payload, _, _, err := decodeConfigImportPayload(strings.NewReader(`[{"key":"max_connections","value":100,"port":8081}]`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	configs, ok := payload["configurations"].([]interface{})
	if !ok || len(configs) != 1 {
		t.Fatal("expected one configuration entry")
	}
	cfg, ok := configs[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected configuration entry to be a map")
	}
	// value 100 应保留为数字类型(json.Number 已转换为 int64/float64)
	if _, isNumber := cfg["value"].(float64); !isNumber {
		if _, isInt := cfg["value"].(int64); !isInt {
			t.Fatalf("expected numeric value preserved, got %T: %v", cfg["value"], cfg["value"])
		}
	}
}

func TestSaveImportUploadIncludesImportTypePrefix(t *testing.T) {
	// L-12: 落盘文件名应携带 import_type 前缀
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "configs.json")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write([]byte(`[{"key":"a","value":"b"}]`)); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/data-management/imports", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Request.ParseMultipartForm(1 << 20)
	defer func() {
		if ctx.Request.MultipartForm != nil {
			_ = ctx.Request.MultipartForm.RemoveAll()
		}
	}()

	file, err := ctx.FormFile("file")
	if err != nil {
		t.Fatalf("failed to get form file: %v", err)
	}

	uploadDir := t.TempDir()
	filePath, _, err := saveImportUpload(file, uploadDir, "configs", 1<<20)
	if err != nil {
		t.Fatalf("failed to save import upload: %v", err)
	}
	base := filepath.Base(filePath)
	if !strings.HasPrefix(base, "configs_") {
		t.Fatalf("expected file name to carry import_type prefix, got %q", base)
	}
}
