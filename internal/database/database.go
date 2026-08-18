package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"superview/internal/models"
	"go.uber.org/zap"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	DB *gorm.DB
	healthCheckTicker *time.Ticker
	healthCheckStop chan struct{}
	healthCheckMutex sync.RWMutex
	isHealthy bool = true
)

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	MaxOpenConns       int           // 最大打开连接数
	MaxIdleConns       int           // 最大空闲连接数
	ConnMaxLifetime    time.Duration // 连接最大生命周期
	ConnMaxIdleTime    time.Duration // 连接最大空闲时间
	QueryTimeout       time.Duration // 查询超时时间
	HealthCheckEnabled bool          // 是否启用健康检查
	HealthCheckInterval time.Duration // 健康检查间隔
	TransactionTimeout time.Duration // 事务超时时间
}

// GetDefaultConfig 获取默认数据库配置
// SQLite WAL 模式支持「多读单写」：多个读连接可并发，写操作串行。
// 放开连接数以避免一个慢查询（如大数据导出）阻塞所有请求；
// 写锁竞争由 DSN 的 _busy_timeout 处理（等待而非立即报 "database is locked"）。
func GetDefaultConfig() *DatabaseConfig {
	return &DatabaseConfig{
		MaxOpenConns:        25,                // WAL 模式下允许并发读，避免单连接串行化
		MaxIdleConns:        5,                 // 保持空闲连接复用
		ConnMaxLifetime:     5 * time.Minute,   // 连接最大生命周期
		ConnMaxIdleTime:     1 * time.Minute,   // 空闲连接回收时间
		QueryTimeout:        30 * time.Second,  // 查询超时30秒
		HealthCheckEnabled:  true,              // 启用健康检查
		HealthCheckInterval: 30 * time.Second,  // 每30秒检查一次
		TransactionTimeout:  60 * time.Second,  // 事务超时60秒
	}
}

func InitDB() error {
	return InitDBWithConfig(GetDefaultConfig())
}

// resolveGormLogLevel 根据环境变量 DB_LOG_LEVEL 决定 GORM 日志级别，默认 Warn。
func resolveGormLogLevel() logger.LogLevel {
	switch strings.ToLower(os.Getenv("DB_LOG_LEVEL")) {
	case "silent":
		return logger.Silent
	case "error":
		return logger.Error
	case "info":
		return logger.Info
	default:
		return logger.Warn
	}
}

func InitDBWithConfig(config *DatabaseConfig) error {
	// 获取当前工作目录作为项目根目录
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}

	// 确保数据库目录存在
	dataDir := filepath.Join(projectRoot, "data")
	zap.L().Info("Database directory", zap.String("path", dataDir))
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %v", err)
	}

	// 配置GORM日志
	// 默认 Warn：避免在生产环境把每一条 SQL 都打到日志（CPU + 磁盘开销）。
	// 可通过环境变量 DB_LOG_LEVEL=silent|error|warn|info 覆盖（开发期可设为 info）。
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(resolveGormLogLevel()),
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
		PrepareStmt: true, // 启用预编译语句缓存
	}

	// 连接SQLite数据库
	dbPath := filepath.Join(dataDir, "superview.db")
	// _busy_timeout: 写锁竞争时等待而非立即报 "database is locked"（配合多读连接）。
	// 不使用 cache=shared：多连接下 shared cache 会引入表级锁，WAL + 私有 cache 才支持多读并发。
	dsn := fmt.Sprintf("%s?mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=1&_busy_timeout=5000", dbPath)
	db, err := gorm.Open(sqlite.Open(dsn), gormConfig)
	if err != nil {
		return fmt.Errorf("failed to connect database: %v", err)
	}

	// 获取底层sql.DB实例并配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}

	// 配置连接池
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// 测试数据库连接
	ctx, cancel := context.WithTimeout(context.Background(), config.QueryTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	// 在迁移前修复 system_settings 表的空 category 字段
	if err := fixEmptyCategories(db); err != nil {
		zap.L().Warn("Failed to fix empty categories", zap.Error(err))
	}

	// 自动迁移数据库模式（SystemSettings 单独处理，避免 GORM 迁移问题）
	err = db.AutoMigrate(
		&models.User{},
		&models.ActivityLog{},
		&models.Role{},
		&models.Permission{},
		&models.UserRole{},
		&models.RolePermission{},
		&models.NodeAccess{},
		&models.AlertRule{},
		&models.Alert{},
		&models.NotificationChannel{},
		&models.Notification{},
		&models.AlertRuleNotificationChannel{},
		&models.SystemMetric{},
		&models.ProcessGroup{},
		&models.ProcessGroupItem{},
		&models.ProcessDependency{},
		&models.ScheduledTask{},
		&models.TaskExecution{},
		&models.ProcessTemplate{},
		&models.ProcessBackup{},
		&models.ProcessMetrics{},
		&models.Configuration{},
		&models.EnvironmentVariable{},
		&models.ConfigurationHistory{},
		&models.ConfigurationBackup{},
		&models.ConfigurationTemplate{},
		&models.ConfigurationValidation{},
		&models.ConfigurationAudit{},
		&models.LogEntry{},
		&models.LogAnalysisRule{},
		&models.LogStatistics{},
		&models.LogAlert{},
		&models.LogFilter{},
		&models.LogExport{},
		&models.LogRetentionPolicy{},
		&models.BackupRecord{},
		&models.DataExportRecord{},
		&models.DataImportRecord{},
		// &models.SystemSettings{}, // 手动管理，避免 GORM 迁移问题
		&models.UserPreferences{},
		&models.WebhookConfig{},
		&models.WebhookLog{},
		&models.DiscoveryTask{},
		&models.DiscoveryResult{},
	)
	if err != nil {
		return fmt.Errorf("failed to migrate models: %v", err)
	}

	// 执行自定义迁移
	if err := runCustomMigrations(db); err != nil {
		return fmt.Errorf("failed to run custom migrations: %v", err)
	}

	DB = db

	// 启动健康检查
	if config.HealthCheckEnabled {
		StartHealthCheck(config.HealthCheckInterval)
	}

	zap.L().Info("Database initialized successfully",
		zap.Int("max_open_conns", config.MaxOpenConns),
		zap.Int("max_idle_conns", config.MaxIdleConns))
	return nil
}

// WithTransaction 执行事务操作
func WithTransaction(fn func(*gorm.DB) error) error {
	return DB.Transaction(fn)
}

// WithTransactionContext 带上下文的事务操作
func WithTransactionContext(ctx context.Context, fn func(*gorm.DB) error) error {
	return DB.WithContext(ctx).Transaction(fn)
}

// WithTransactionTimeout 带超时的事务操作
func WithTransactionTimeout(timeout time.Duration, fn func(*gorm.DB) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return WithTransactionContext(ctx, fn)
}

// StartHealthCheck 启动数据库健康检查
func StartHealthCheck(interval time.Duration) {
	StopHealthCheck() // 确保之前的检查已停止
	
	healthCheckStop = make(chan struct{})
	healthCheckTicker = time.NewTicker(interval)
	
	go func() {
		for {
			select {
			case <-healthCheckTicker.C:
				performHealthCheck()
			case <-healthCheckStop:
				return
			}
		}
	}()
	
	zap.L().Info("Database health check started", zap.Duration("interval", interval))
}

// StopHealthCheck 停止数据库健康检查
func StopHealthCheck() {
	if healthCheckTicker != nil {
		healthCheckTicker.Stop()
		healthCheckTicker = nil
	}
	if healthCheckStop != nil {
		select {
		case <-healthCheckStop:
			// 已经关闭
		default:
			close(healthCheckStop)
		}
		healthCheckStop = nil
	}
}

// performHealthCheck 执行健康检查
func performHealthCheck() {
	err := HealthCheck()
	
	healthCheckMutex.Lock()
	defer healthCheckMutex.Unlock()
	
	if err != nil {
		if isHealthy {
			zap.L().Warn("Database health check failed", zap.Error(err))
			isHealthy = false
		}
	} else {
		if !isHealthy {
			zap.L().Info("Database health check recovered")
			isHealthy = true
		}
	}
}

// IsHealthy 检查数据库是否健康
func IsHealthy() bool {
	healthCheckMutex.RLock()
	defer healthCheckMutex.RUnlock()
	return isHealthy
}

// GetHealthStatus 获取详细的健康状态信息
func GetHealthStatus() map[string]interface{} {
	healthCheckMutex.RLock()
	healthy := isHealthy
	healthCheckMutex.RUnlock()
	
	stats, err := GetConnectionStats()
	if err != nil {
		return map[string]interface{}{
			"healthy": false,
			"error":   err.Error(),
		}
	}
	
	return map[string]interface{}{
		"healthy":           healthy,
		"open_connections":  stats.OpenConnections,
		"in_use":           stats.InUse,
		"idle":             stats.Idle,
		"wait_count":       stats.WaitCount,
		"wait_duration":    stats.WaitDuration.String(),
		"max_idle_closed":  stats.MaxIdleClosed,
		"max_lifetime_closed": stats.MaxLifetimeClosed,
	}
}

// HealthCheck 数据库健康检查
func HealthCheck() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return sqlDB.PingContext(ctx)
}

// GetConnectionStats 获取连接池统计信息
func GetConnectionStats() (sql.DBStats, error) {
	sqlDB, err := DB.DB()
	if err != nil {
		return sql.DBStats{}, fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}
	return sqlDB.Stats(), nil
}

// Close 关闭数据库连接
func Close() error {
	// 停止健康检查
	StopHealthCheck()
	
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %v", err)
	}
	return sqlDB.Close()
}

// execMigration 执行一条尽力而为的迁移语句，失败时记录日志而非静默忽略。
func execMigration(db *gorm.DB, step, sql string, args ...interface{}) {
	if err := db.Exec(sql, args...).Error; err != nil {
		zap.L().Warn("migration step failed", zap.String("step", step), zap.Error(err))
	}
}

// fixEmptyCategories 手动管理 system_settings 表，避免 GORM AutoMigrate 的问题
func fixEmptyCategories(db *gorm.DB) error {
	// 检查表是否存在
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='system_settings'").Scan(&count).Error; err != nil {
		return err
	}

	// 表不存在，创建新表
	if count == 0 {
		zap.L().Info("Creating system_settings table")
		createSQL := `CREATE TABLE system_settings (
			id TEXT PRIMARY KEY,
			category TEXT DEFAULT 'general',
			key TEXT NOT NULL,
			value TEXT,
			value_type TEXT DEFAULT 'string',
			description TEXT,
			is_public NUMERIC DEFAULT false,
			updated_by TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`
		if err := db.Exec(createSQL).Error; err != nil {
			return err
		}
		execMigration(db, "create idx_category_key", "CREATE UNIQUE INDEX IF NOT EXISTS idx_category_key ON system_settings(category, key)")
		execMigration(db, "create idx_system_settings_deleted_at", "CREATE INDEX IF NOT EXISTS idx_system_settings_deleted_at ON system_settings(deleted_at)")
		zap.L().Info("Created system_settings table")
		return nil
	}

	// 表存在，检查是否需要重建（有外键约束）
	var tableSQL string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='system_settings'").Scan(&tableSQL).Error; err != nil {
		return err
	}

	if !strings.Contains(tableSQL, "FOREIGN KEY") {
		// 没有外键，只需修复空值
		execMigration(db, "fix empty categories", "UPDATE system_settings SET category = 'general' WHERE category IS NULL OR category = ''")
		return nil
	}

	zap.L().Info("Rebuilding system_settings table to remove foreign key constraints")

	// 备份数据
	type SettingBackup struct {
		ID          string
		Category    string
		Key         string
		Value       string
		ValueType   string
		Description string
		IsPublic    bool
		UpdatedBy   *string
		CreatedAt   time.Time
		UpdatedAt   time.Time
	}

	var backups []SettingBackup
	if err := db.Raw(`SELECT id, COALESCE(category, 'general') as category, key, value, 
		COALESCE(value_type, 'string') as value_type, description, is_public, updated_by, 
		created_at, updated_at FROM system_settings WHERE deleted_at IS NULL`).Scan(&backups).Error; err != nil {
		zap.L().Warn("Failed to backup system_settings", zap.Error(err))
		return nil
	}

	// 禁用外键检查(SQLite 不允许在事务内修改该 pragma,因此必须在事务外执行)
	if err := db.Exec("PRAGMA foreign_keys=OFF").Error; err != nil {
		return err
	}
	defer func() {
		if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
			zap.L().Warn("Failed to re-enable foreign_keys", zap.Error(err))
		}
	}()

	// 在事务内重建表:任何步骤失败都会回滚整个操作,避免 DROP/CREATE/INSERT
	// 中途失败导致 system_settings 表为空。
	return db.Transaction(func(tx *gorm.DB) error {
		// 重建表
		if err := tx.Exec("DROP TABLE IF EXISTS system_settings").Error; err != nil {
			return err
		}

		createSQL := `CREATE TABLE system_settings (
			id TEXT PRIMARY KEY,
			category TEXT DEFAULT 'general',
			key TEXT NOT NULL,
			value TEXT,
			value_type TEXT DEFAULT 'string',
			description TEXT,
			is_public NUMERIC DEFAULT false,
			updated_by TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`
		if err := tx.Exec(createSQL).Error; err != nil {
			return err
		}

		// 恢复数据
		for _, b := range backups {
			if b.Key == "" {
				continue
			}
			if err := tx.Exec(`INSERT INTO system_settings (id, category, key, value, value_type, description, is_public, updated_by, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				b.ID, b.Category, b.Key, b.Value, b.ValueType, b.Description, b.IsPublic, b.UpdatedBy, b.CreatedAt, b.UpdatedAt).Error; err != nil {
				return err
			}
		}

		if err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_category_key ON system_settings(category, key)").Error; err != nil {
			return err
		}
		if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_system_settings_deleted_at ON system_settings(deleted_at)").Error; err != nil {
			return err
		}

		zap.L().Info("Successfully rebuilt system_settings table", zap.Int("records", len(backups)))
		return nil
	})
}

// runCustomMigrations 执行自定义数据库迁移
func runCustomMigrations(db *gorm.DB) error {
	// 删除旧的全局唯一索引（如果存在）
	if err := db.Exec("DROP INDEX IF EXISTS idx_alert_unique").Error; err != nil {
		zap.L().Warn("Failed to drop old index idx_alert_unique", zap.Error(err))
	}
	
	// 创建条件唯一索引：仅对 status='active' 的记录生效
	// 这允许同一个 (rule_id, node_name, process_name) 组合在不同状态下存在多条记录
	// 但在 active 状态下只能有一条记录
	createIndexSQL := `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_unique_active 
		ON alerts(rule_id, node_name, process_name, status) 
		WHERE status = 'active'
	`
	if err := db.Exec(createIndexSQL).Error; err != nil {
		return fmt.Errorf("failed to create conditional unique index: %v", err)
	}
	
	// 修复 system_settings 表的外键约束问题
	// SQLite 不支持直接删除外键，需要重建表
	if err := fixSystemSettingsForeignKey(db); err != nil {
		zap.L().Warn("Failed to fix system_settings foreign key", zap.Error(err))
	}
	
	zap.L().Info("Custom migrations completed successfully")
	return nil
}

// fixSystemSettingsForeignKey 修复 system_settings 表的外键约束
func fixSystemSettingsForeignKey(db *gorm.DB) error {
	// 检查表是否存在
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='system_settings'").Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil // 表不存在，跳过
	}
	
	// 检查是否需要迁移（检查外键约束是否存在）
	var fkCount int64
	if err := db.Raw("SELECT COUNT(*) FROM pragma_foreign_key_list('system_settings') WHERE \"table\"='users' AND \"from\"='updated_by'").Scan(&fkCount).Error; err != nil {
		return err
	}
	if fkCount == 0 {
		return nil // 外键不存在，跳过
	}
	
	zap.L().Info("Migrating system_settings table to remove foreign key constraint")
	
	// SQLite 重建表的步骤
	// 1. 禁用外键检查
	if err := db.Exec("PRAGMA foreign_keys=OFF").Error; err != nil {
		return err
	}
	
	// 2. 开始事务
	tx := db.Begin()
	var deferredErr error
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			if deferredErr == nil {
				deferredErr = fmt.Errorf("recovered from panic during FK migration: %v", r)
			}
		}
	}()
	
	// 3. 重命名旧表
	if deferredErr != nil {
		return deferredErr
	}
	if err := tx.Exec("ALTER TABLE system_settings RENAME TO system_settings_old").Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// 4. 创建新表（没有外键约束，category 允许 NULL 但有默认值）
	createTableSQL := `
		CREATE TABLE system_settings (
			id TEXT PRIMARY KEY,
			category TEXT DEFAULT 'general',
			key TEXT NOT NULL,
			value TEXT,
			value_type TEXT DEFAULT 'string',
			description TEXT,
			is_public NUMERIC DEFAULT false,
			updated_by TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`
	if err := tx.Exec(createTableSQL).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// 4.5. 更新旧表中空的 category 字段
	if err := tx.Exec("UPDATE system_settings_old SET category = 'general' WHERE category IS NULL OR category = ''").Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// 5. 复制数据
	copyDataSQL := `
		INSERT INTO system_settings 
		SELECT id, category, key, value, value_type, description, is_public, updated_by, created_at, updated_at, deleted_at 
		FROM system_settings_old
	`
	if err := tx.Exec(copyDataSQL).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// 6. 删除旧表
	if err := tx.Exec("DROP TABLE system_settings_old").Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// 7. 重新创建索引
	if err := tx.Exec("CREATE UNIQUE INDEX idx_category_key ON system_settings(category, key)").Error; err != nil {
		tx.Rollback()
		return err
	}
	
	if err := tx.Exec("CREATE INDEX idx_system_settings_deleted_at ON system_settings(deleted_at)").Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// 8. 提交事务
	if err := tx.Commit().Error; err != nil {
		return err
	}
	
	// 9. 重新启用外键检查
	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		return err
	}
	
	zap.L().Info("Successfully migrated system_settings table")
	return nil
}
