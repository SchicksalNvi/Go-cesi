package services

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"superview/internal/database"
	"superview/internal/logger"
	"superview/internal/models"
)

const maxConcurrentDataJobs = 2

// secretMask 用于掩码导出/备份中的秘密配置值(H-09)。
const secretMask = "********"

// isSecretSettingKey 判断系统设置键是否为敏感项,导出时需掩码(H-09)。
func isSecretSettingKey(key string) bool {
	lower := strings.ToLower(key)
	for _, secretPart := range []string{"password", "secret", "token", "api_key", "apikey", "smtp_pass", "webhook"} {
		if strings.Contains(lower, secretPart) {
			return true
		}
	}
	return false
}

var (
	// ErrDataJobCapacity 表示共享的数据管理后台任务槽位已满。
	ErrDataJobCapacity = errors.New("data management job capacity reached")

	// dataJobSlots 由导出、导入和备份共享。槽位在启动 goroutine 前获取，
	// 因而最多只会存在 maxConcurrentDataJobs 个后台任务 goroutine。
	dataJobSlots   = make(chan struct{}, maxConcurrentDataJobs)
	dataJobCancels sync.Map
)

func dataJobKey(name, id string) string {
	return name + ":" + id
}

// runDataJob 在共享并发上限内启动可取消的后台任务，并从 panic 中恢复。
func runDataJob(name, id string, fn func(context.Context)) error {
	select {
	case dataJobSlots <- struct{}{}:
	default:
		return ErrDataJobCapacity
	}

	ctx, cancel := context.WithCancel(context.Background())
	key := dataJobKey(name, id)
	dataJobCancels.Store(key, cancel)

	go func() {
		defer func() {
			cancel()
			dataJobCancels.Delete(key)
			<-dataJobSlots
			if r := recover(); r != nil {
				logger.Error("data management job panicked",
					zap.String("job", name),
					zap.String("id", id),
					zap.Any("panic", r))
			}
		}()
		fn(ctx)
	}()

	return nil
}

func cancelDataJob(name, id string) {
	if cancel, ok := dataJobCancels.Load(dataJobKey(name, id)); ok {
		cancel.(context.CancelFunc)()
	}
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

// ImportProcessor 处理一个已落盘的导入文件。
type ImportProcessor func(ctx context.Context) (totalRecords int, validationLog string, err error)

// StartImport 将导入加入与导出、备份共享的受限后台任务槽位。
// 上传源文件采用即时清理策略：任务完成、失败、取消或无法入队时均尝试删除。
func (s *DataManagementService) StartImport(record *models.DataImportRecord, processor ImportProcessor) error {
	if record == nil {
		return errors.New("import record is required")
	}
	if processor == nil {
		return errors.New("import processor is required")
	}

	err := runDataJob("import", record.ID, func(ctx context.Context) {
		defer s.cleanupImportSource(record)
		defer func() {
			if r := recover(); r != nil {
				_ = s.UpdateImportStatus(record, models.StatusFailed, 0, 0, 0, fmt.Sprintf("import failed: %v", r), "")
			}
		}()

		if err := ctx.Err(); err != nil {
			_ = s.UpdateImportStatus(record, models.StatusFailed, 0, 0, 0, err.Error(), "")
			return
		}

		if err := s.UpdateImportStatus(record, models.StatusRunning, 0, 0, 0, "", ""); err != nil {
			logger.Error("failed to mark import as running",
				zap.String("id", record.ID),
				zap.Error(err))
		}

		totalRecords, validationLog, processErr := processor(ctx)
		if processErr != nil {
			_ = s.UpdateImportStatus(record, models.StatusFailed, totalRecords, 0, totalRecords, processErr.Error(), validationLog)
			return
		}

		_ = s.UpdateImportStatus(record, models.StatusCompleted, totalRecords, totalRecords, 0, "", validationLog)
	})
	if err != nil {
		_ = s.UpdateImportStatus(record, models.StatusFailed, 0, 0, 0, err.Error(), "")
		s.cleanupImportSource(record)
		return err
	}

	return nil
}

func (s *DataManagementService) cleanupImportSource(record *models.DataImportRecord) {
	if record.SourceFile == "" {
		return
	}

	if err := os.Remove(record.SourceFile); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to remove import source file",
			zap.String("id", record.ID),
			zap.String("path", record.SourceFile),
			zap.Error(err))
		return
	}

	if err := s.DB.Model(&models.DataImportRecord{}).
		Where("id = ?", record.ID).
		Update("source_file", "").Error; err != nil {
		logger.Error("failed to clear import source path",
			zap.String("id", record.ID),
			zap.Error(err))
	}
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

	// 异步执行导出（共享并发上限 + panic 恢复）
	if err := runDataJob("export", exportRecord.ID, func(context.Context) { s.performExport(exportRecord) }); err != nil {
		s.updateExportStatus(exportRecord, models.StatusFailed, err.Error())
		return nil, fmt.Errorf("failed to start export: %w", err)
	}

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

	// H-09: 导出时掩码秘密值,防止敏感配置(如 SMTP 密码、密钥)随导出泄露
	for i := range configs {
		if configs[i].IsSecret && configs[i].Value != "" {
			configs[i].Value = secretMask
		}
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
	if err := s.DB.Preload("Roles").Find(&users).Error; err != nil {
		return 0, fmt.Errorf("failed to export users: %w", err)
	}
	allData["users"] = users
	totalRecords += len(users)

	// 日志数据(M-01: 查询失败必须返回错误,不能静默跳过)
	var logs []models.ActivityLog
	if err := s.DB.Order("created_at DESC").Limit(maxExportLogs).Find(&logs).Error; err != nil {
		return 0, fmt.Errorf("failed to export logs: %w", err)
	}
	if len(logs) == maxExportLogs {
		logger.Warn("activity log export truncated to limit; older logs not included",
			zap.Int("limit", maxExportLogs))
	}
	allData["logs"] = logs
	totalRecords += len(logs)

	// 配置数据(H-09: 掩码秘密值)
	var configs []models.Configuration
	if err := s.DB.Find(&configs).Error; err != nil {
		return 0, fmt.Errorf("failed to export configs: %w", err)
	}
	for i := range configs {
		if configs[i].IsSecret && configs[i].Value != "" {
			configs[i].Value = secretMask
		}
	}
	allData["configs"] = configs
	totalRecords += len(configs)

	// 系统设置
	var settings []models.SystemSettings
	if err := s.DB.Find(&settings).Error; err != nil {
		return 0, fmt.Errorf("failed to export settings: %w", err)
	}
	for i := range settings {
		if isSecretSettingKey(settings[i].Key) && settings[i].Value != "" {
			settings[i].Value = secretMask
		}
	}
	allData["settings"] = settings
	totalRecords += len(settings)

	allData["export_time"] = time.Now()
	allData["version"] = "1.0"
	allData["truncated"] = len(logs) == maxExportLogs

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
// writeCSV 将表头与数据行写入 CSV 文件，统一处理文件/writer 样板与错误。
func writeCSV(filePath string, headers []string, rows [][]string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write(headers); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

// exportUsersToCSV 导出用户数据为CSV格式
func (s *DataManagementService) exportUsersToCSV(filePath string, users []models.User) error {
	headers := []string{"ID", "Username", "Email", "FullName", "IsActive", "IsAdmin", "LastLogin", "CreatedAt", "Roles"}
	rows := make([][]string, 0, len(users))
	for _, user := range users {
		roleNames := make([]string, len(user.Roles))
		for i, role := range user.Roles {
			roleNames[i] = role.Name
		}

		lastLogin := ""
		if user.LastLogin != nil {
			lastLogin = user.LastLogin.Format("2006-01-02 15:04:05")
		}

		rows = append(rows, []string{
			user.ID,
			user.Username,
			user.Email,
			user.FullName,
			strconv.FormatBool(user.IsActive),
			strconv.FormatBool(user.IsAdmin),
			lastLogin,
			user.CreatedAt.Format("2006-01-02 15:04:05"),
			strings.Join(roleNames, ";"),
		})
	}
	return writeCSV(filePath, headers, rows)
}

// exportLogsToCSV 导出日志数据为CSV格式
func (s *DataManagementService) exportLogsToCSV(filePath string, logs []models.ActivityLog) error {
	headers := []string{"ID", "UserID", "Action", "Resource", "Details", "IPAddress", "UserAgent", "CreatedAt"}
	rows := make([][]string, 0, len(logs))
	for _, log := range logs {
		rows = append(rows, []string{
			strconv.FormatUint(uint64(log.ID), 10),
			log.UserID,
			log.Action,
			log.Resource,
			log.Details,
			log.IPAddress,
			log.UserAgent,
			log.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return writeCSV(filePath, headers, rows)
}

// exportConfigsToCSV 导出配置数据为CSV格式
func (s *DataManagementService) exportConfigsToCSV(filePath string, configs []models.Configuration) error {
	headers := []string{"ID", "NodeID", "Name", "Description", "ConfigType", "IsActive", "CreatedAt", "UpdatedAt"}
	rows := make([][]string, 0, len(configs))
	for _, config := range configs {
		nodeIDStr := ""
		if config.NodeID != nil {
			nodeIDStr = strconv.FormatUint(uint64(*config.NodeID), 10)
		}
		rows = append(rows, []string{
			strconv.FormatUint(uint64(config.ID), 10),
			nodeIDStr,
			config.Key,
			config.Description,
			config.Type,
			strconv.FormatBool(config.IsRequired),
			config.CreatedAt.Format("2006-01-02 15:04:05"),
			config.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return writeCSV(filePath, headers, rows)
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

// listPaginatedRecords 通用分页列表查询：Preload Creator、按 created_at 倒序、
// 统一走 database.Paginate（含 pageSize 上限保护）。scope 可选，用于追加过滤条件。
func listPaginatedRecords[T any](db *gorm.DB, page, pageSize int, scope func(*gorm.DB) *gorm.DB) ([]T, int64, error) {
	var records []T
	var total int64

	query := db.Model(new(T)).Preload("Creator")
	if scope != nil {
		query = scope(query)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Scopes(database.Paginate(page, pageSize)).Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// GetExportRecords 获取导出记录列表
func (s *DataManagementService) GetExportRecords(page, pageSize int, userID string) ([]models.DataExportRecord, int64, error) {
	var scope func(*gorm.DB) *gorm.DB
	if userID != "" {
		// 非管理员只能查看自己的记录
		scope = func(q *gorm.DB) *gorm.DB { return q.Where("created_by = ?", userID) }
	}
	return listPaginatedRecords[models.DataExportRecord](s.DB, page, pageSize, scope)
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
	return listPaginatedRecords[models.DataImportRecord](s.DB, page, pageSize, nil)
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

	// M-34: 先取消作业,再短暂等待作业协程识别取消并完成其 deferred 清理
	// (移除导入源文件等),避免删除记录后作业仍回写状态。
	cancelDataJob("import", id)
	time.Sleep(150 * time.Millisecond)

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

	// 异步执行备份（共享并发上限 + panic 恢复）
	if err := runDataJob("backup", backupRecord.ID, func(context.Context) { s.performBackup(backupRecord) }); err != nil {
		s.updateBackupStatus(backupRecord, models.StatusFailed, err.Error())
		return nil, fmt.Errorf("failed to start backup: %w", err)
	}

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

	// M-10: 防止备份名称包含路径穿越字符,避免写文件到任意目录
	cleanName := filepath.Clean(record.Name)
	if cleanName != record.Name || strings.Contains(record.Name, "..") || strings.ContainsAny(record.Name, `/\`) {
		s.updateBackupStatus(record, models.StatusFailed, "backup name contains invalid path characters")
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
	// H-10: SQLite 运行在 WAL 模式,最新已提交事务可能仍驻留在 -wal 文件中。
	// 直接复制主库文件会产生不一致快照。先执行 checkpoint 将 WAL 合并回主库文件,
	// 再复制,确保备份包含全部已提交数据。
	if s.DB != nil {
		if err := s.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
			logger.Warn("WAL checkpoint before backup failed, backup may be inconsistent", zap.Error(err))
		}
	}

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

	// 备份配置文件(不存在时忽略,其它 I/O 错误记录但不中断备份)
	if err := s.addFileToZip(zipWriter, "config.toml", "config.toml"); err != nil && !os.IsNotExist(err) {
		logger.Warn("failed to add config.toml to backup", zap.Error(err))
	}

	// 备份日志文件
	logDir := "logs"
	if _, err := os.Stat(logDir); err == nil {
		// M-10: Walk 与 addFileToZip 的错误必须传播,不能静默忽略
		walkErr := filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				relPath, relErr := filepath.Rel(".", path)
				if relErr != nil {
					return relErr
				}
				if err := s.addFileToZip(zipWriter, path, relPath); err != nil {
					return err
				}
			}
			return nil
		})
		if walkErr != nil {
			logger.Warn("failed to add log files to backup", zap.Error(walkErr))
		}
	}

	return nil
}

// createConfigBackup 创建配置备份
func (s *DataManagementService) createConfigBackup(backupFilePath string) error {
	// M-10: 使用每次任务唯一的临时目录,避免并发配置备份互相覆盖
	tempDir := filepath.Join("data", "temp", uuid.New().String())
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
	return listPaginatedRecords[models.BackupRecord](s.DB, page, pageSize, nil)
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
