package services

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"superview/internal/database"
	"superview/internal/logger"
	"superview/internal/models"
	"gorm.io/gorm"
)

// dataJobSlots 限制并发的导出/备份后台任务数，防止无界 goroutine 耗尽内存/磁盘/DB 连接。
var dataJobSlots = make(chan struct{}, 2)

// runDataJob 在受限并发的后台 goroutine 中运行任务，并从 panic 中恢复。
func runDataJob(name, id string, fn func()) {
	go func() {
		dataJobSlots <- struct{}{}
		defer func() {
			<-dataJobSlots
			if r := recover(); r != nil {
				logger.Error("data management job panicked",
					zap.String("job", name),
					zap.String("id", id),
					zap.Any("panic", r))
			}
		}()
		fn()
	}()
}

// DataManagementService 数据管理服务
type DataManagementService struct {
	DB *gorm.DB
}

// NewDataManagementService 创建数据管理服务实例
func NewDataManagementService() *DataManagementService {
	return &DataManagementService{
		DB: database.DB,
	}
}

// applyUpdates 应用 GORM 更新并在失败时记录日志（避免静默吞掉状态更新错误）。
func (s *DataManagementService) applyUpdates(record interface{}, updates map[string]interface{}, ctx string) {
	if err := s.DB.Model(record).Updates(updates).Error; err != nil {
		logger.Error("failed to update record",
			zap.String("ctx", ctx),
			zap.Error(err))
	}
}

// CreateImportRecord 创建导入记录
func (s *DataManagementService) CreateImportRecord(importType, sourceFile string, fileSize int64, createdBy string) (*models.DataImportRecord, error) {
	record := &models.DataImportRecord{
		ID:         uuid.New().String(),
		Name:       fmt.Sprintf("%s_import_%s", importType, time.Now().Format("20060102_150405")),
		ImportType: importType,
		SourceFile: sourceFile,
		FileSize:   fileSize,
		Status:     models.StatusPending,
		CreatedBy:  createdBy,
		CreatedAt:  time.Now(),
	}

	if err := s.DB.Create(record).Error; err != nil {
		return nil, fmt.Errorf("failed to create import record: %v", err)
	}

	return record, nil
}

// ExportData 导出数据
func (s *DataManagementService) ExportData(exportType, format, createdBy string) (*models.DataExportRecord, error) {
	// 创建导出记录
	exportRecord := &models.DataExportRecord{
		ID:         uuid.New().String(),
		Name:       fmt.Sprintf("%s_export_%s", exportType, time.Now().Format("20060102_150405")),
		ExportType: exportType,
		Format:     format,
		Status:     models.StatusPending,
		CreatedBy:  createdBy,
		CreatedAt:  time.Now(),
	}

	if err := s.DB.Create(exportRecord).Error; err != nil {
		return nil, fmt.Errorf("failed to create export record: %v", err)
	}

	// 异步执行导出（受限并发 + panic 恢复）
	runDataJob("export", exportRecord.ID, func() { s.performExport(exportRecord) })

	return exportRecord, nil
}

// performExport 执行数据导出
func (s *DataManagementService) performExport(record *models.DataExportRecord) {
	// 更新状态为运行中
	s.applyUpdates(record, map[string]interface{}{
		"status": models.StatusRunning,
	}, "performExport:running")

	// 创建导出目录
	exportDir := "data/exports"
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		s.updateExportStatus(record, models.StatusFailed, fmt.Sprintf("Failed to create export directory: %v", err))
		return
	}

	// 生成文件路径
	fileName := fmt.Sprintf("%s.%s", record.Name, record.Format)
	filePath := filepath.Join(exportDir, fileName)

	var err error
	var recordCount int

	// 根据导出类型和格式执行导出
	switch record.ExportType {
	case models.ExportTypeUsers:
		recordCount, err = s.exportUsers(filePath, record.Format)
	case models.ExportTypeLogs:
		recordCount, err = s.exportLogs(filePath, record.Format)
	case models.ExportTypeConfigs:
		recordCount, err = s.exportConfigs(filePath, record.Format)
	case models.ExportTypeProcesses:
		recordCount, err = s.exportProcesses(filePath, record.Format)
	case models.ExportTypeAll:
		recordCount, err = s.exportAll(filePath, record.Format)
	default:
		err = fmt.Errorf("unsupported export type: %s", record.ExportType)
	}

	if err != nil {
		s.updateExportStatus(record, models.StatusFailed, err.Error())
		return
	}

	// 获取文件大小
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		s.updateExportStatus(record, models.StatusFailed, fmt.Sprintf("Failed to get file info: %v", err))
		return
	}

	// 更新导出记录
	now := time.Now()
	s.applyUpdates(record, map[string]interface{}{
		"file_path":    filePath,
		"file_size":    fileInfo.Size(),
		"record_count": recordCount,
		"status":       models.StatusCompleted,
		"completed_at": &now,
	}, "performExport:completed")
}

// exportUsers 导出用户数据
func (s *DataManagementService) exportUsers(filePath, format string) (int, error) {
	var users []models.User
	if err := s.DB.Preload("Roles").Find(&users).Error; err != nil {
		return 0, err
	}

	switch format {
	case models.ExportFormatJSON:
		return len(users), s.exportToJSON(filePath, users)
	case models.ExportFormatCSV:
		return len(users), s.exportUsersToCSV(filePath, users)
	default:
		return 0, fmt.Errorf("unsupported format: %s", format)
	}
}

// maxExportLogs 单次日志导出的最大条数上限。
const maxExportLogs = 10000

// exportLogs 导出日志数据
func (s *DataManagementService) exportLogs(filePath, format string) (int, error) {
	var logs []models.ActivityLog
	if err := s.DB.Order("created_at DESC").Limit(maxExportLogs).Find(&logs).Error; err != nil {
		return 0, err
	}
	if len(logs) == maxExportLogs {
		logger.Warn("activity log export truncated to limit; older logs not included",
			zap.Int("limit", maxExportLogs))
	}

	switch format {
	case models.ExportFormatJSON:
		return len(logs), s.exportToJSON(filePath, logs)
	case models.ExportFormatCSV:
		return len(logs), s.exportLogsToCSV(filePath, logs)
	default:
		return 0, fmt.Errorf("unsupported format: %s", format)
	}
}

// exportConfigs 导出配置数据
func (s *DataManagementService) exportConfigs(filePath, format string) (int, error) {
	var configs []models.Configuration
	if err := s.DB.Find(&configs).Error; err != nil {
		return 0, err
	}

	switch format {
	case models.ExportFormatJSON:
		return len(configs), s.exportToJSON(filePath, configs)
	case models.ExportFormatCSV:
		return len(configs), s.exportConfigsToCSV(filePath, configs)
	default:
		return 0, fmt.Errorf("unsupported format: %s", format)
	}
}

// exportProcesses 导出进程数据
func (s *DataManagementService) exportProcesses(filePath, format string) (int, error) {
	// 这里应该从supervisor获取进程数据，暂时返回空数据
	processes := []map[string]interface{}{}

	switch format {
	case models.ExportFormatJSON:
		return len(processes), s.exportToJSON(filePath, processes)
	default:
		return 0, fmt.Errorf("unsupported format: %s", format)
	}
}

// exportAll 导出所有数据
func (s *DataManagementService) exportAll(filePath, format string) (int, error) {
	if format != models.ExportFormatJSON {
		return 0, fmt.Errorf("full export only supports JSON format")
	}

	// 收集所有数据
	allData := make(map[string]interface{})
	totalRecords := 0

	// 用户数据
	var users []models.User
	if err := s.DB.Preload("Roles").Find(&users).Error; err == nil {
		allData["users"] = users
		totalRecords += len(users)
	}

	// 日志数据
	var logs []models.ActivityLog
	if err := s.DB.Order("created_at DESC").Limit(maxExportLogs).Find(&logs).Error; err == nil {
		if len(logs) == maxExportLogs {
			logger.Warn("activity log export truncated to limit; older logs not included",
				zap.Int("limit", maxExportLogs))
		}
		allData["logs"] = logs
		totalRecords += len(logs)
	}

	// 配置数据
	var configs []models.Configuration
	if err := s.DB.Find(&configs).Error; err == nil {
		allData["configs"] = configs
		totalRecords += len(configs)
	}

	// 系统设置
	var settings []models.SystemSettings
	if err := s.DB.Find(&settings).Error; err == nil {
		allData["settings"] = settings
		totalRecords += len(settings)
	}

	allData["export_time"] = time.Now()
	allData["version"] = "1.0"

	return totalRecords, s.exportToJSON(filePath, allData)
}

// exportToJSON 导出为JSON格式
func (s *DataManagementService) exportToJSON(filePath string, data interface{}) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// exportUsersToCSV 导出用户数据为CSV格式
func (s *DataManagementService) exportUsersToCSV(filePath string, users []models.User) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入标题行
	headers := []string{"ID", "Username", "Email", "FullName", "IsActive", "IsAdmin", "LastLogin", "CreatedAt", "Roles"}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// 写入数据行
	for _, user := range users {
		roleNames := make([]string, len(user.Roles))
		for i, role := range user.Roles {
			roleNames[i] = role.Name
		}

		lastLogin := ""
		if user.LastLogin != nil {
			lastLogin = user.LastLogin.Format("2006-01-02 15:04:05")
		}

		row := []string{
			user.ID,
			user.Username,
			user.Email,
			user.FullName,
			strconv.FormatBool(user.IsActive),
			strconv.FormatBool(user.IsAdmin),
			lastLogin,
			user.CreatedAt.Format("2006-01-02 15:04:05"),
			strings.Join(roleNames, ";"),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// exportLogsToCSV 导出日志数据为CSV格式
func (s *DataManagementService) exportLogsToCSV(filePath string, logs []models.ActivityLog) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入标题行
	headers := []string{"ID", "UserID", "Action", "Resource", "Details", "IPAddress", "UserAgent", "CreatedAt"}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// 写入数据行
	for _, log := range logs {
		row := []string{
			strconv.FormatUint(uint64(log.ID), 10),
			log.UserID,
			log.Action,
			log.Resource,
			log.Details,
			log.IPAddress,
			log.UserAgent,
			log.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// exportConfigsToCSV 导出配置数据为CSV格式
func (s *DataManagementService) exportConfigsToCSV(filePath string, configs []models.Configuration) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入标题行
	headers := []string{"ID", "NodeID", "Name", "Description", "ConfigType", "IsActive", "CreatedAt", "UpdatedAt"}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// 写入数据行
	for _, config := range configs {
		nodeIDStr := ""
		if config.NodeID != nil {
			nodeIDStr = strconv.FormatUint(uint64(*config.NodeID), 10)
		}
		row := []string{
			strconv.FormatUint(uint64(config.ID), 10),
			nodeIDStr,
			config.Key,
			config.Description,
			config.Type,
			strconv.FormatBool(config.IsRequired),
			config.CreatedAt.Format("2006-01-02 15:04:05"),
			config.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// updateExportStatus 更新导出状态
func (s *DataManagementService) updateExportStatus(record *models.DataExportRecord, status, errorMsg string) {
	updates := map[string]interface{}{
		"status": status,
	}
	if errorMsg != "" {
		updates["error_msg"] = errorMsg
	}
	if status == models.StatusCompleted || status == models.StatusFailed {
		now := time.Now()
		updates["completed_at"] = &now
	}
	s.applyUpdates(record, updates, "updateExportStatus")
}

// GetExportRecords 获取导出记录列表
func (s *DataManagementService) GetExportRecords(page, pageSize int, userID string) ([]models.DataExportRecord, int64, error) {
	var records []models.DataExportRecord
	var total int64

	query := s.DB.Model(&models.DataExportRecord{}).Preload("Creator")

	// 如果不是管理员，只能查看自己的记录
	if userID != "" {
		query = query.Where("created_by = ?", userID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// DeleteExportRecord 删除导出记录
func (s *DataManagementService) DeleteExportRecord(id string) error {
	var record models.DataExportRecord
	if err := s.DB.First(&record, "id = ?", id).Error; err != nil {
		return err
	}

	// 删除文件
	if record.FilePath != "" {
		os.Remove(record.FilePath)
	}

	// 删除记录
	return s.DB.Delete(&record).Error
}

// GetImportRecords 获取导入记录列表
func (s *DataManagementService) GetImportRecords(page, pageSize int) ([]models.DataImportRecord, int64, error) {
	var records []models.DataImportRecord
	var total int64

	query := s.DB.Model(&models.DataImportRecord{}).Preload("Creator")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// UpdateImportStatus 更新导入状态
func (s *DataManagementService) UpdateImportStatus(record *models.DataImportRecord, status string, totalRecords, successCount, failureCount int, errorMsg, validationLog string) error {
	updates := map[string]interface{}{
		"status":         status,
		"total_records":  totalRecords,
		"success_count":  successCount,
		"failure_count":  failureCount,
		"validation_log": validationLog,
	}
	if errorMsg != "" {
		updates["error_msg"] = errorMsg
	}
	if status == models.StatusCompleted || status == models.StatusFailed || status == models.StatusPartial {
		now := time.Now()
		updates["completed_at"] = &now
	}
	return s.DB.Model(record).Updates(updates).Error
}

// DeleteImportRecord 删除导入记录
func (s *DataManagementService) DeleteImportRecord(id string) error {
	var record models.DataImportRecord
	if err := s.DB.First(&record, "id = ?", id).Error; err != nil {
		return err
	}

	if record.SourceFile != "" {
		_ = os.Remove(record.SourceFile)
	}

	return s.DB.Delete(&record).Error
}

// CreateBackup 创建备份
func (s *DataManagementService) CreateBackup(backupType, name, description, createdBy string) (*models.BackupRecord, error) {
	backupRecord := &models.BackupRecord{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		BackupType:  backupType,
		Status:      models.StatusPending,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
	}

	if err := s.DB.Create(backupRecord).Error; err != nil {
		return nil, fmt.Errorf("failed to create backup record: %v", err)
	}

	// 异步执行备份（受限并发 + panic 恢复）
	runDataJob("backup", backupRecord.ID, func() { s.performBackup(backupRecord) })

	return backupRecord, nil
}

// performBackup 执行备份
func (s *DataManagementService) performBackup(record *models.BackupRecord) {
	// 更新状态为运行中
	s.applyUpdates(record, map[string]interface{}{
		"status": models.StatusRunning,
	}, "performBackup:running")

	// 创建备份目录
	backupDir := "data/backups"
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		s.updateBackupStatus(record, models.StatusFailed, fmt.Sprintf("Failed to create backup directory: %v", err))
		return
	}

	// 生成备份文件路径
	backupFileName := fmt.Sprintf("%s_%s.zip", record.Name, time.Now().Format("20060102_150405"))
	backupFilePath := filepath.Join(backupDir, backupFileName)

	var err error
	switch record.BackupType {
	case models.BackupTypeFull:
		err = s.createFullBackup(backupFilePath)
	case models.BackupTypeConfigOnly:
		err = s.createConfigBackup(backupFilePath)
	default:
		err = fmt.Errorf("unsupported backup type: %s", record.BackupType)
	}

	if err != nil {
		s.updateBackupStatus(record, models.StatusFailed, err.Error())
		return
	}

	// 获取文件大小
	fileInfo, err := os.Stat(backupFilePath)
	if err != nil {
		s.updateBackupStatus(record, models.StatusFailed, fmt.Sprintf("Failed to get backup file info: %v", err))
		return
	}

	// 更新备份记录
	now := time.Now()
	s.applyUpdates(record, map[string]interface{}{
		"file_path":    backupFilePath,
		"file_size":    fileInfo.Size(),
		"status":       models.StatusCompleted,
		"completed_at": &now,
	}, "performBackup:completed")
}

// createFullBackup 创建完整备份
func (s *DataManagementService) createFullBackup(backupFilePath string) error {
	// 创建ZIP文件
	zipFile, err := os.Create(backupFilePath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 备份数据库文件
	if err := s.addFileToZip(zipWriter, "data/superview.db", "superview.db"); err != nil {
		return fmt.Errorf("failed to backup database: %v", err)
	}

	// 备份配置文件（不存在时忽略，其它 I/O 错误记录但不中断备份）
	if err := s.addFileToZip(zipWriter, "config.toml", "config.toml"); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to add config.toml to backup", zap.Error(err))
	}

	// 备份日志文件
	logDir := "logs"
	if _, err := os.Stat(logDir); err == nil {
		filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				relPath, _ := filepath.Rel(".", path)
				s.addFileToZip(zipWriter, path, relPath)
			}
			return nil
		})
	}

	return nil
}

// createConfigBackup 创建配置备份
func (s *DataManagementService) createConfigBackup(backupFilePath string) error {
	// 导出配置数据为JSON
	tempDir := "data/temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "configurations.json")
	if _, err := s.exportConfigs(configFile, models.ExportFormatJSON); err != nil {
		return err
	}

	// 创建ZIP文件
	zipFile, err := os.Create(backupFilePath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 添加配置文件到ZIP
	return s.addFileToZip(zipWriter, configFile, "configurations.json")
}

// addFileToZip 添加文件到ZIP
func (s *DataManagementService) addFileToZip(zipWriter *zip.Writer, filePath, zipPath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	zipFile, err := zipWriter.Create(zipPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(zipFile, file)
	return err
}

// updateBackupStatus 更新备份状态
func (s *DataManagementService) updateBackupStatus(record *models.BackupRecord, status, errorMsg string) {
	updates := map[string]interface{}{
		"status": status,
	}
	if errorMsg != "" {
		updates["error_msg"] = errorMsg
	}
	if status == models.StatusCompleted || status == models.StatusFailed {
		now := time.Now()
		updates["completed_at"] = &now
	}
	s.applyUpdates(record, updates, "updateBackupStatus")
}

// GetBackupRecords 获取备份记录列表
func (s *DataManagementService) GetBackupRecords(page, pageSize int) ([]models.BackupRecord, int64, error) {
	var records []models.BackupRecord
	var total int64

	query := s.DB.Model(&models.BackupRecord{}).Preload("Creator")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// DeleteBackupRecord 删除备份记录
func (s *DataManagementService) DeleteBackupRecord(id string) error {
	var record models.BackupRecord
	if err := s.DB.First(&record, "id = ?", id).Error; err != nil {
		return err
	}

	// 删除备份文件
	if record.FilePath != "" {
		os.Remove(record.FilePath)
	}

	// 删除记录
	return s.DB.Delete(&record).Error
}
