package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"superview/internal/models"
	"superview/internal/services"
	"superview/internal/validation"

	"github.com/gin-gonic/gin"
)

// ImportRecord 导入记录
type ImportRecord struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	Type        string    `json:"type"`
	Status      string    `json:"status"` // processing, completed, failed
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	FilePath    string    `json:"-"` // 不返回给前端
}

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

// generateID 生成唯一ID
func generateID() string {
	return fmt.Sprintf("import_%d", time.Now().UnixNano())
}

// processUserImport 处理用户数据导入
func (api *DataManagementAPI) processUserImport(filePath string) error {
	// 读取JSON文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	// 解析用户数据
	var users []map[string]interface{}
	if err := json.Unmarshal(data, &users); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}

	// 验证和处理用户数据
	for i, user := range users {
		if user["username"] == nil || user["username"] == "" {
			return fmt.Errorf("user at index %d missing username", i)
		}
		// 这里应该调用用户服务创建用户
		// userService.CreateUser(user)
	}

	return nil
}

// processSettingsImport 处理设置数据导入
func (api *DataManagementAPI) processSettingsImport(filePath string) error {
	// 读取JSON文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	// 解析设置数据
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}

	// 验证和处理设置数据
	for key, value := range settings {
		if key == "" {
			return fmt.Errorf("empty setting key found")
		}
		// 这里应该调用设置服务更新设置
		// settingsService.UpdateSetting(key, value)
		fmt.Printf("Would import setting: %s = %v\n", key, value)
	}

	return nil
}

// ExportDataRequest 导出数据请求
type ExportDataRequest struct {
	ExportType string `json:"export_type" binding:"required,oneof=users logs configs processes all"`
	Format     string `json:"format" binding:"required,oneof=json csv xlsx"`
}

// CreateBackupRequest 创建备份请求
type CreateBackupRequest struct {
	Name        string `json:"name" binding:"required,max=100"`
	Description string `json:"description" binding:"max=500"`
	BackupType  string `json:"backup_type" binding:"required,oneof=full incremental config_only"`
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

	// 获取当前用户ID
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "User not authenticated"})
		return
	}

	// 执行导出
	exportRecord, err := api.dataService.ExportData(req.ExportType, req.Format, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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

// CreateBackup 创建备份
// @Summary 创建备份
// @Description 创建系统数据备份
// @Tags 数据管理
// @Accept json
// @Produce json
// @Param request body CreateBackupRequest true "备份请求"
// @Success 200 {object} models.BackupRecord
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/data-management/backups [post]
func (api *DataManagementAPI) CreateBackup(c *gin.Context) {
	var req CreateBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// 获取当前用户ID
	userID, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "User not authenticated"})
		return
	}

	// 创建备份
	backupRecord, err := api.dataService.CreateBackup(req.BackupType, req.Name, req.Description, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, backupRecord)

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Created backup: %s (%s)", req.Name, req.BackupType)
		api.activityLogService.LogWithContext(c, "INFO", "create_backup", "data_management", req.Name, msg, nil)
	}
}

// GetBackupRecords 获取备份记录列表
// @Summary 获取备份记录列表
// @Description 获取系统备份记录列表，支持分页
// @Tags 数据管理
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} PaginatedResponse{data=[]models.BackupRecord}
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/data-management/backups [get]
func (api *DataManagementAPI) GetBackupRecords(c *gin.Context) {
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

	records, total, err := api.dataService.GetBackupRecords(page, pageSize)
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

// DownloadBackupFile 下载备份文件
// @Summary 下载备份文件
// @Description 下载指定的备份文件
// @Tags 数据管理
// @Param id path string true "备份记录ID"
// @Success 200 {file} file
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/data-management/backups/{id}/download [get]
func (api *DataManagementAPI) DownloadBackupFile(c *gin.Context) {
	id := c.Param("id")

	// 获取备份记录
	var record models.BackupRecord
	if err := api.dataService.DB.First(&record, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Backup record not found"})
		return
	}

	// 检查文件是否存在
	if record.FilePath == "" || record.Status != models.StatusCompleted {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Backup file not available"})
		return
	}

	if _, err := os.Stat(record.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Backup file not found"})
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

// DeleteBackupRecord 删除备份记录
// @Summary 删除备份记录
// @Description 删除指定的备份记录和文件
// @Tags 数据管理
// @Param id path string true "备份记录ID"
// @Success 200 {object} SuccessResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/data-management/backups/{id} [delete]
func (api *DataManagementAPI) DeleteBackupRecord(c *gin.Context) {
	id := c.Param("id")

	if err := api.dataService.DeleteBackupRecord(id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Deleted backup record: %s", id)
		api.activityLogService.LogWithContext(c, "WARNING", "delete_backup", "data_management", id, msg, nil)
	}

	c.JSON(http.StatusOK, SuccessResponse{Message: "Backup record deleted successfully"})
}

// ImportData 导入数据
// @Summary 导入数据
// @Description 从文件导入数据到系统
// @Tags 数据管理
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "导入文件"
// @Param import_type formData string true "导入类型" Enums(users,configs,full_backup)
// @Success 200 {object} models.DataImportRecord
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/data-management/import [post]
func (api *DataManagementAPI) ImportData(c *gin.Context) {
	// 获取上传文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "No file uploaded"})
		return
	}

	importType := c.PostForm("import_type")
	if importType == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Import type is required"})
		return
	}

	// 验证导入类型
	validTypes := []string{models.ImportTypeUsers, models.ImportTypeConfigs, models.ImportTypeFullBackup}
	valid := false
	for _, validType := range validTypes {
		if importType == validType {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid import type"})
		return
	}

	// 保存上传文件
	uploadDir := "data/uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create upload directory"})
		return
	}

	filePath := filepath.Join(uploadDir, file.Filename)
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to save uploaded file"})
		return
	}

	// 创建导入记录
	importRecord := ImportRecord{
		ID:        generateID(),
		Filename:  file.Filename,
		Type:      importType,
		Status:    "processing",
		CreatedAt: time.Now(),
		FilePath:  filePath,
	}

	// 异步处理导入
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// 更新导入状态为失败
				importRecord.Status = "failed"
				importRecord.Error = fmt.Sprintf("Import failed: %v", r)
			}
		}()

		// 根据导入类型处理文件
		switch importType {
		case "users":
			err := api.processUserImport(filePath)
			if err != nil {
				importRecord.Status = "failed"
				importRecord.Error = err.Error()
			} else {
				importRecord.Status = "completed"
			}
		case "settings":
			err := api.processSettingsImport(filePath)
			if err != nil {
				importRecord.Status = "failed"
				importRecord.Error = err.Error()
			} else {
				importRecord.Status = "completed"
			}
		default:
			importRecord.Status = "failed"
			importRecord.Error = "Unsupported import type"
		}

		importRecord.CompletedAt = time.Now()
		// 这里应该保存到数据库，但为了简化，我们只记录日志
		fmt.Printf("Import %s completed with status: %s\n", importRecord.ID, importRecord.Status)
	}()

	c.JSON(http.StatusOK, gin.H{
		"message":   "File uploaded successfully, import is being processed",
		"import_id": importRecord.ID,
		"file":      file.Filename,
		"type":      importType,
		"status":    "processing",
	})

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Imported data: file=%s type=%s", file.Filename, importType)
		api.activityLogService.LogWithContext(c, "INFO", "import_data", "data_management", file.Filename, msg, nil)
	}
}

// RegisterDataManagementRoutes 注册数据管理路由
func RegisterDataManagementRoutes(router *gin.RouterGroup) {
	api := NewDataManagementAPI()

	// 数据导出相关路由
	router.POST("/export", api.ExportData)
	router.GET("/exports", api.GetExportRecords)
	router.GET("/exports/:id/download", api.DownloadExportFile)
	router.DELETE("/exports/:id", api.DeleteExportRecord)

	// 数据备份相关路由
	router.POST("/backups", api.CreateBackup)
	router.GET("/backups", api.GetBackupRecords)
	router.GET("/backups/:id/download", api.DownloadBackupFile)
	router.DELETE("/backups/:id", api.DeleteBackupRecord)

	// 数据导入相关路由
	router.POST("/import", api.ImportData)
}
