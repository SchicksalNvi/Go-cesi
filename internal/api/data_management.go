package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"superview/internal/models"
	"superview/internal/services"
	"superview/internal/validation"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// DataManagementAPI 数据管理API
type DataManagementAPI struct {
	dataService        *services.DataManagementService
	activityLogService *services.ActivityLogService
}

// NewDataManagementAPI 创建数据管理API实例
func NewDataManagementAPI(activityLogService ...*services.ActivityLogService) *DataManagementAPI {
	api := &DataManagementAPI{
		dataService: services.NewDataManagementService(),
	}
	if len(activityLogService) > 0 {
		api.activityLogService = activityLogService[0]
	}
	return api
}

// ExportDataRequest 导出数据请求
type ExportDataRequest struct {
	ExportType string `json:"export_type" binding:"required,oneof=users logs configs processes all"`
	Format     string `json:"format" binding:"required,oneof=json csv"`
}

var supportedExportFormats = map[string]map[string]bool{
	models.ExportTypeUsers: {
		models.ExportFormatJSON: true,
		models.ExportFormatCSV:  true,
	},
	models.ExportTypeLogs: {
		models.ExportFormatJSON: true,
		models.ExportFormatCSV:  true,
	},
	models.ExportTypeConfigs: {
		models.ExportFormatJSON: true,
		models.ExportFormatCSV:  true,
	},
	models.ExportTypeProcesses: {
		models.ExportFormatJSON: true,
	},
	models.ExportTypeAll: {
		models.ExportFormatJSON: true,
	},
}

var supportedImportTypes = map[string]bool{
	models.ImportTypeConfigs: true,
}

const (
	maxImportFileSize     int64 = 32 << 20 // 32 MiB
	maxImportRequestSize        = maxImportFileSize + (1 << 20)
	importMultipartMemory int64 = 1 << 20 // 超过 1 MiB 的 multipart 内容落临时文件
)

var errImportTooLarge = errors.New("import file exceeds the 32 MiB limit")

type configImportDecodeState struct {
	configurations     []interface{}
	configAlias        []interface{}
	environmentVars    []interface{}
	hasConfigurations  bool
	hasConfigAlias     bool
	hasEnvironmentVars bool
}

func normalizeConfigImportPayload(data []byte) (map[string]interface{}, int, string, error) {
	return decodeConfigImportPayload(bytes.NewReader(data))
}

func decodeConfigImportPayload(reader io.Reader) (map[string]interface{}, int, string, error) {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()

	firstToken, err := decoder.Token()
	if err != nil {
		return nil, 0, "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	delimiter, ok := firstToken.(json.Delim)
	if !ok {
		return nil, 0, "", fmt.Errorf("unsupported import payload structure")
	}

	var state configImportDecodeState
	switch delimiter {
	case '[':
		configs, err := decodeConfigImportArray(decoder)
		if err != nil {
			return nil, 0, "", fmt.Errorf("failed to parse JSON: %w", err)
		}
		state.configurations = configs
		state.hasConfigurations = true
	case '{':
		state, err = decodeConfigImportObject(decoder)
		if err != nil {
			return nil, 0, "", fmt.Errorf("failed to parse JSON: %w", err)
		}
	default:
		return nil, 0, "", fmt.Errorf("unsupported import payload structure")
	}

	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, 0, "", fmt.Errorf("failed to parse JSON: multiple JSON values are not allowed")
		}
		return nil, 0, "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	payload := make(map[string]interface{})
	configs := state.configurations
	if !state.hasConfigurations && state.hasConfigAlias {
		configs = state.configAlias
	}
	if state.hasConfigurations || state.hasConfigAlias {
		payload["configurations"] = configs
	}
	if state.hasEnvironmentVars {
		payload["environment_variables"] = state.environmentVars
	}

	configCount := len(configs)
	envVarCount := len(state.environmentVars)
	totalRecords := configCount + envVarCount
	if totalRecords == 0 {
		return nil, 0, "", fmt.Errorf("no configurations or environment variables found in import file")
	}

	summary := fmt.Sprintf("Prepared %d configurations and %d environment variables for import", configCount, envVarCount)
	return payload, totalRecords, summary, nil
}

func decodeConfigImportObject(decoder *json.Decoder) (configImportDecodeState, error) {
	var state configImportDecodeState
	var nestedState *configImportDecodeState

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return state, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return state, fmt.Errorf("object key is not a string")
		}

		switch key {
		case "configurations":
			state.configurations, err = decodeConfigImportArrayValue(decoder, key)
			state.hasConfigurations = err == nil
		case "configs":
			state.configAlias, err = decodeConfigImportArrayValue(decoder, key)
			state.hasConfigAlias = err == nil
		case "environment_variables":
			state.environmentVars, err = decodeConfigImportArrayValue(decoder, key)
			state.hasEnvironmentVars = err == nil
		case "data":
			var nested configImportDecodeState
			nested, ok, err = decodeNestedConfigImportObject(decoder)
			if ok {
				nestedState = &nested
			}
		default:
			err = skipJSONValue(decoder)
		}
		if err != nil {
			return state, err
		}
	}

	endToken, err := decoder.Token()
	if err != nil {
		return state, err
	}
	if endToken != json.Delim('}') {
		return state, fmt.Errorf("expected end of object")
	}
	if nestedState != nil {
		return *nestedState, nil
	}
	return state, nil
}

func decodeNestedConfigImportObject(decoder *json.Decoder) (configImportDecodeState, bool, error) {
	firstToken, err := decoder.Token()
	if err != nil {
		return configImportDecodeState{}, false, err
	}
	delimiter, ok := firstToken.(json.Delim)
	if ok && delimiter == '{' {
		state, err := decodeConfigImportObject(decoder)
		return state, true, err
	}
	if err := skipJSONToken(decoder, firstToken); err != nil {
		return configImportDecodeState{}, false, err
	}
	return configImportDecodeState{}, false, nil
}

func decodeConfigImportArrayValue(decoder *json.Decoder, field string) ([]interface{}, error) {
	firstToken, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := firstToken.(json.Delim)
	if !ok || delimiter != '[' {
		if err := skipJSONToken(decoder, firstToken); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s must be an array", field)
	}
	return decodeConfigImportArray(decoder)
}

func decodeConfigImportArray(decoder *json.Decoder) ([]interface{}, error) {
	items := make([]interface{}, 0)
	for decoder.More() {
		var item interface{}
		if err := decoder.Decode(&item); err != nil {
			return nil, err
		}
		// M-33: UseNumber 解码出的 json.Number 在后续 json.Marshal 时会变成带引号字符串,
		// 破坏数字类型配置值。此处统一还原为 float64/int64。
		items = append(items, convertJSONNumberValues(item))
	}

	endToken, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if endToken != json.Delim(']') {
		return nil, fmt.Errorf("expected end of array")
	}
	return items, nil
}

// convertJSONNumberValues 将 json.Number 递归还原为 float64(整数还原为 int64),
// 避免 UseNumber 解码后数字被序列化为带引号字符串(M-33)。
func convertJSONNumberValues(v interface{}) interface{} {
	switch val := v.(type) {
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return i
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		return val.String()
	case map[string]interface{}:
		for k, mv := range val {
			val[k] = convertJSONNumberValues(mv)
		}
		return val
	case []interface{}:
		for i, mv := range val {
			val[i] = convertJSONNumberValues(mv)
		}
		return val
	default:
		return v
	}
}

func skipJSONValue(decoder *json.Decoder) error {
	firstToken, err := decoder.Token()
	if err != nil {
		return err
	}
	return skipJSONToken(decoder, firstToken)
}

func skipJSONToken(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delimiter {
	case '{':
		for decoder.More() {
			if _, err := decoder.Token(); err != nil {
				return err
			}
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	default:
		return nil
	}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (api *DataManagementAPI) processConfigImport(ctx context.Context, filePath, userID string, overwriteExisting bool) (int, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return 0, "", fmt.Errorf("failed to inspect file: %w", err)
	}
	if fileInfo.Size() > maxImportFileSize {
		return 0, "", errImportTooLarge
	}

	payload, totalRecords, validationLog, err := decodeConfigImportPayload(contextReader{ctx: ctx, reader: file})
	if err != nil {
		return 0, "", err
	}
	if err := ctx.Err(); err != nil {
		return totalRecords, validationLog, err
	}

	configService := services.NewConfigurationService(api.dataService.DB.WithContext(ctx))
	options := map[string]interface{}{
		"overwrite_existing": overwriteExisting,
		"import_configs":     true,
		"import_env_vars":    true,
	}

	if err := configService.ImportConfigurations(payload, userID, options); err != nil {
		return totalRecords, validationLog, err
	}
	if err := ctx.Err(); err != nil {
		return totalRecords, validationLog, err
	}

	return totalRecords, validationLog, nil
}

func parseImportUpload(c *gin.Context, maxFileSize int64) (*multipart.FileHeader, error) {
	maxRequestSize := maxFileSize + (maxImportRequestSize - maxImportFileSize)
	if c.Request.ContentLength > maxRequestSize {
		return nil, errImportTooLarge
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestSize)
	if err := c.Request.ParseMultipartForm(importMultipartMemory); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, errImportTooLarge
		}
		return nil, err
	}

	file, err := c.FormFile("file")
	if err != nil {
		return nil, err
	}
	if file.Size > maxFileSize {
		return nil, errImportTooLarge
	}
	return file, nil
}

func saveImportUpload(file *multipart.FileHeader, uploadDir, importType string, maxFileSize int64) (string, int64, error) {
	source, err := file.Open()
	if err != nil {
		return "", 0, err
	}
	defer source.Close()

	// L-12: 文件名携带 import_type 前缀,便于从导入记录还原原始文件类型
	filePath := filepath.Join(uploadDir, importType+"_"+uuid.New().String()+".json")
	destination, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", 0, err
	}

	saved := false
	defer func() {
		if !saved {
			_ = os.Remove(filePath)
		}
	}()

	written, copyErr := io.Copy(destination, io.LimitReader(source, maxFileSize+1))
	closeErr := destination.Close()
	if written > maxFileSize {
		return "", written, errImportTooLarge
	}
	if copyErr != nil {
		return "", written, copyErr
	}
	if closeErr != nil {
		return "", written, closeErr
	}

	saved = true
	return filePath, written, nil
}

// ExportData 导出数据
// @Summary 导出数据
// @Description 导出指定类型的数据为指定格式
// @Tags 数据管理
// @Accept json
// @Produce json
// @Param request body ExportDataRequest true "导出请求"
// @Success 200 {object} models.DataExportRecord
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/data-management/export [post]
func (api *DataManagementAPI) ExportData(c *gin.Context) {
	var req ExportDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if !supportedExportFormats[req.ExportType][req.Format] {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("format %s is not supported for export type %s", req.Format, req.ExportType),
		})
		return
	}

	// 获取当前用户ID
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "User not authenticated"})
		return
	}

	// 执行导出
	exportRecord, err := api.dataService.ExportData(req.ExportType, req.Format, userID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, services.ErrDataJobCapacity) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, exportRecord)

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Exported data: type=%s format=%s", req.ExportType, req.Format)
		api.activityLogService.LogWithContext(c, "INFO", "export_data", "data_management", req.ExportType, msg, nil)
	}
}

// GetExportRecords 获取导出记录列表
// @Summary 获取导出记录列表
// @Description 获取数据导出记录列表，支持分页
// @Tags 数据管理
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} PaginatedResponse{data=[]models.DataExportRecord}
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/data-management/exports [get]
func (api *DataManagementAPI) GetExportRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// 验证分页参数
	validator := validation.NewValidator()
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")
	pageNum, limitNum := validator.ValidatePagination(pageStr, pageSizeStr)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid pagination parameters"})
		return
	}
	page = pageNum
	pageSize = limitNum

	// 获取当前用户
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "User not found"})
		return
	}

	currentUser := user.(*models.User)
	var userID string
	// 如果不是管理员，只能查看自己的记录
	if !currentUser.IsSuperAdmin() {
		userID = currentUser.ID
	}

	records, total, err := api.dataService.GetExportRecords(page, pageSize, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, PaginatedResponse{
		Data:       records,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// DownloadExportFile 下载导出文件
// @Summary 下载导出文件
// @Description 下载指定的导出文件
// @Tags 数据管理
// @Param id path string true "导出记录ID"
// @Success 200 {file} file
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/data-management/exports/{id}/download [get]
func (api *DataManagementAPI) DownloadExportFile(c *gin.Context) {
	id := c.Param("id")

	// 获取导出记录
	var record models.DataExportRecord
	if err := api.dataService.DB.First(&record, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Export record not found"})
		return
	}

	// 检查文件是否存在
	if record.FilePath == "" || record.Status != models.StatusCompleted {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Export file not available"})
		return
	}

	if _, err := os.Stat(record.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Export file not found"})
		return
	}

	// 检查权限
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "User not found"})
		return
	}

	currentUser := user.(*models.User)
	if !currentUser.IsSuperAdmin() && record.CreatedBy != currentUser.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied"})
		return
	}

	// 设置下载响应头
	fileName := filepath.Base(record.FilePath)
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Header("Content-Type", "application/octet-stream")

	// 发送文件
	c.File(record.FilePath)
}

// DeleteExportRecord 删除导出记录
// @Summary 删除导出记录
// @Description 删除指定的导出记录和文件
// @Tags 数据管理
// @Param id path string true "导出记录ID"
// @Success 200 {object} SuccessResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/data-management/exports/{id} [delete]
func (api *DataManagementAPI) DeleteExportRecord(c *gin.Context) {
	id := c.Param("id")

	if err := api.dataService.DeleteExportRecord(id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Deleted export record: %s", id)
		api.activityLogService.LogWithContext(c, "WARNING", "delete_export", "data_management", id, msg, nil)
	}

	c.JSON(http.StatusOK, SuccessResponse{Message: "Export record deleted successfully"})
}

// ImportData 导入数据
// @Summary 导入数据
// @Description 从文件导入数据到系统
// @Tags 数据管理
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "导入文件"
// @Param import_type formData string true "导入类型" Enums(configs)
// @Param overwrite_existing formData boolean false "是否覆盖已存在的配置"
// @Success 200 {object} models.DataImportRecord
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 413 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /api/data-management/import [post]
func (api *DataManagementAPI) ImportData(c *gin.Context) {
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()

	// 在解析 multipart 前限制整个请求体；随后再次校验单文件大小。
	file, err := parseImportUpload(c, maxImportFileSize)
	if err != nil {
		if errors.Is(err, errImportTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: errImportTooLarge.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "No file uploaded"})
		return
	}

	importType := c.PostForm("import_type")
	if importType == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Import type is required"})
		return
	}

	if !supportedImportTypes[importType] {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("import type %s is not supported", importType),
		})
		return
	}

	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "User not authenticated"})
		return
	}

	overwriteExisting := strings.EqualFold(c.DefaultPostForm("overwrite_existing", "false"), "true")

	// 保存上传文件
	uploadDir := "data/uploads"
	if err := os.MkdirAll(uploadDir, 0700); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create upload directory"})
		return
	}

	filePath, fileSize, err := saveImportUpload(file, uploadDir, importType, maxImportFileSize)
	if err != nil {
		if errors.Is(err, errImportTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: errImportTooLarge.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to save uploaded file"})
		return
	}

	importRecord, err := api.dataService.CreateImportRecord(importType, filePath, fileSize, userID)
	if err != nil {
		_ = os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	responseRecord := *importRecord
	if err := api.dataService.StartImport(importRecord, func(ctx context.Context) (int, string, error) {
		return api.processConfigImport(ctx, filePath, userID, overwriteExisting)
	}); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, services.ErrDataJobCapacity) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, &responseRecord)

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Imported data: file=%s type=%s", file.Filename, importType)
		api.activityLogService.LogWithContext(c, "INFO", "import_data", "data_management", file.Filename, msg, nil)
	}
}

// GetImportRecords 获取导入记录列表
func (api *DataManagementAPI) GetImportRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	validator := validation.NewValidator()
	pageNum, limitNum := validator.ValidatePagination(c.DefaultQuery("page", "1"), c.DefaultQuery("page_size", "20"))
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid pagination parameters"})
		return
	}
	page = pageNum
	pageSize = limitNum

	records, total, err := api.dataService.GetImportRecords(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, PaginatedResponse{
		Data:       records,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

// DeleteImportRecord 删除导入记录
func (api *DataManagementAPI) DeleteImportRecord(c *gin.Context) {
	id := c.Param("id")

	if err := api.dataService.DeleteImportRecord(id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Deleted import record: %s", id)
		api.activityLogService.LogWithContext(c, "WARNING", "delete_import", "data_management", id, msg, nil)
	}

	c.JSON(http.StatusOK, SuccessResponse{Message: "Import record deleted successfully"})
}

// RegisterDataManagementRoutes 注册数据管理路由
func RegisterDataManagementRoutes(router *gin.RouterGroup) {
	api := NewDataManagementAPI()

	// 数据导出相关路由
	router.POST("/export", api.ExportData)
	router.GET("/exports", api.GetExportRecords)
	router.GET("/exports/:id/download", api.DownloadExportFile)
	router.DELETE("/exports/:id", api.DeleteExportRecord)

	// 数据导入相关路由
	router.GET("/imports", api.GetImportRecords)
	router.DELETE("/imports/:id", api.DeleteImportRecord)
	router.POST("/import", api.ImportData)
	router.POST("/imports", api.ImportData)
}
