package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"superview/internal/api"
	"superview/internal/auth"
	"superview/internal/config"
	"superview/internal/database"
	"superview/internal/logger"
	"superview/internal/loggers"
	"superview/internal/metrics"
	"superview/internal/middleware"
	"superview/internal/models"
	"superview/internal/services"
	"superview/internal/supervisor"
	"superview/internal/websocket"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

type Config struct {
	Server struct {
		Port int `mapstructure:"port"`
	}
	Admin struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
		Email    string `mapstructure:"email"`
	} `mapstructure:"admin"`
	Nodes []struct {
		Name        string `mapstructure:"name"`
		Environment string `mapstructure:"environment"`
		Host        string `mapstructure:"host"`
		Port        int    `mapstructure:"port"`
		Username    string `mapstructure:"username"`
		Password    string `mapstructure:"password"`
	} `mapstructure:"nodes"`
}

// ensureAdminUser 确保管理员用户存在（仅首次初始化时创建，之后数据库为唯一真相源）
func ensureAdminUser(db *gorm.DB, config *Config) error {
	var count int64
	if err := db.Model(&models.User{}).Where("is_admin = ?", true).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check admin user: %v", err)
	}

	// 已存在管理员，不覆盖数据库（用户可能通过 Web UI 修改过）
	if count > 0 {
		return nil
	}

	adminUser := models.User{
		Username: config.Admin.Username,
		Email:    config.Admin.Email,
		IsAdmin:  true,
	}
	if err := adminUser.SetPassword(config.Admin.Password); err != nil {
		return fmt.Errorf("failed to set admin password: %v", err)
	}
	if err := db.Create(&adminUser).Error; err != nil {
		return fmt.Errorf("failed to create admin user: %v", err)
	}
	logger.Info("Default admin user created (seed from config)",
		zap.String("username", config.Admin.Username),
		zap.String("email", config.Admin.Email))
	return nil
}

func createAdminCommand(username, password, email string) {
	if err := database.InitDB(); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	db := database.DB

	adminUser := models.User{
		Username: username,
		Email:    email,
		IsAdmin:  true,
	}
	if err := adminUser.SetPassword(password); err != nil {
		logger.Fatal("Failed to set password", zap.Error(err))
	}

	if err := db.Create(&adminUser).Error; err != nil {
		logger.Fatal("Failed to create admin user", zap.Error(err))
	}
	logger.Info("Admin user created successfully",
		zap.String("username", username),
		zap.String("email", email))
}

// validateEnvironmentVariables 验证必要的环境变量
func validateEnvironmentVariables() error {
	requiredEnvVars := []string{
		"JWT_SECRET",
		"ADMIN_PASSWORD",
		"NODE_PASSWORD",
	}

	var missingVars []string
	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			missingVars = append(missingVars, envVar)
		}
	}

	if len(missingVars) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missingVars, ", "))
	}

	// 验证JWT_SECRET长度
	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters long for security")
	}

	return nil
}

// determineConfigPaths 确定配置文件路径
func determineConfigPaths() (mainPath, nodeListPath string) {
	// 优先使用 config/ 目录，如果不存在则回退到根目录
	mainPath = "config/config.toml"
	nodeListPath = "config/nodelist.toml"

	// 检查 config/ 目录是否存在
	if _, err := os.Stat("config"); os.IsNotExist(err) {
		// 回退到根目录的 config.toml（向后兼容）
		mainPath = "config.toml"
		nodeListPath = ""
		logger.Info("Using legacy config.toml format (config/ directory not found)")
	} else {
		logger.Info("Using config/ directory structure")
	}

	return mainPath, nodeListPath
}

func main() {
	// 加载.env文件 - 优先从 config 目录，回退到根目录
	envPaths := []string{"config/.env", ".env"}
	var envLoaded bool
	var lastErr error

	for _, envPath := range envPaths {
		if err := godotenv.Load(envPath); err == nil {
			envLoaded = true
			break
		} else {
			lastErr = err
		}
	}

	if !envLoaded {
		// .env文件不存在或加载失败时，继续执行（可能使用系统环境变量）
		fmt.Printf("Warning: .env file not found or failed to load: %v\n", lastErr)
	}

	// 初始化动态日志系统
	if err := logger.InitDynamicLogger(); err != nil {
		// 由于logger初始化失败，使用标准错误输出
		fmt.Fprintf(os.Stderr, "Failed to initialize dynamic logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// 记录日志系统启动信息
	logger.Info("Dynamic logging system initialized successfully")

	// 验证环境变量
	if err := validateEnvironmentVariables(); err != nil {
		logger.Fatal("Environment validation failed", zap.Error(err))
	}

	// 加载应用配置
	mainConfigPath, nodeListPath := determineConfigPaths()

	// 使用 ConfigLoader 加载配置
	loader := config.NewConfigLoader(mainConfigPath, nodeListPath)
	appConfig, err := loader.LoadWithDefaults()
	if err != nil {
		logger.Fatal("Failed to load application config", zap.Error(err))
	}
	logger.Info("Application config loaded successfully")

	// 验证应用配置,失败时快速退出,避免带着无效配置启动服务
	if err := config.NewValidator().Validate(appConfig); err != nil {
		logger.Fatal("Application config validation failed", zap.Error(err))
	}
	logger.Info("Application config validation passed")

	if len(os.Args) > 1 && os.Args[1] == "create-admin" {
		if len(os.Args) < 6 {
			logger.Fatal("Usage: go run cmd/main.go create-admin --username <username> --password <password> --email <email>")
		}
		// 解析命令行参数
		var username, password, email string
		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--username":
				if i+1 >= len(os.Args) { // L-06: 越界防护
					logger.Fatal("Missing value for --username")
				}
				username = os.Args[i+1]
				i++
			case "--password":
				if i+1 >= len(os.Args) { // L-06: 越界防护
					logger.Fatal("Missing value for --password")
				}
				password = os.Args[i+1]
				i++
			case "--email":
				if i+1 >= len(os.Args) { // L-06: 越界防护
					logger.Fatal("Missing value for --email")
				}
				email = os.Args[i+1]
				i++
			default:
				logger.Fatal("Unknown argument: " + os.Args[i])
			}
		}
		if username == "" || password == "" || email == "" {
			logger.Fatal("Missing required arguments")
		}
		createAdminCommand(username, password, email)
		return
	}

	// 加载节点配置
	nodeConfig, err := loadConfig()
	if err != nil {
		logger.Fatal("Failed to load node config", zap.Error(err))
	}
	logger.Info("Node config loaded",
		zap.Int("server_port", nodeConfig.Server.Port),
		zap.String("admin_username", nodeConfig.Admin.Username),
		zap.Int("nodes_count", len(nodeConfig.Nodes)))

	// 初始化数据库
	if err := database.InitDB(); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	db := database.DB

	// 确保管理员用户存在（仅首次 seed）
	if err := ensureAdminUser(db, nodeConfig); err != nil {
		logger.Fatal("Failed to ensure admin user", zap.Error(err))
	}

	// 启动性能监控
	if appConfig.Performance.MemoryMonitoringEnabled {
		middleware.StartMemoryMonitoring(appConfig.Performance.MemoryUpdateInterval)
		logger.Info("Memory monitoring started",
			zap.Duration("update_interval", appConfig.Performance.MemoryUpdateInterval))
	}

	if appConfig.Performance.MetricsCleanupEnabled {
		middleware.StartPerformanceCleanup(
			appConfig.Performance.MetricsResetInterval,
			appConfig.Performance.EndpointCleanupThreshold,
		)
		logger.Info("Performance metrics cleanup started",
			zap.Duration("reset_interval", appConfig.Performance.MetricsResetInterval),
			zap.Duration("cleanup_threshold", appConfig.Performance.EndpointCleanupThreshold))
	}

	// 初始化活动日志服务
	loggers.InitActivityLogService(db)
	activityLogService := services.NewActivityLogService(db)

	// 初始化Supervisor服务
	supervisorService := supervisor.NewSupervisorService()

	// 设置活动日志记录器到 supervisor
	supervisorService.SetActivityLogger(activityLogService)

	// 初始化系统角色和权限(幂等,已存在则跳过)
	roleService := services.NewRoleService(db)
	if err := roleService.InitializeSystemRoles(); err != nil {
		logger.Fatal("Failed to initialize system roles", zap.Error(err))
	}
	logger.Info("System roles initialized")

	permissionService := services.NewPermissionService(db)
	if err := permissionService.InitializeSystemPermissions(); err != nil {
		logger.Fatal("Failed to initialize system permissions", zap.Error(err))
	}
	logger.Info("System permissions initialized")

	// 为系统角色分配默认权限(超级管理员拥有全部权限)
	for _, roleName := range []string{
		models.RoleSuperAdmin,
		models.RoleEnvironmentAdmin,
		models.RoleNodeOperator,
		models.RoleReadOnlyUser,
	} {
		if err := permissionService.AssignDefaultPermissionsToRole(roleName); err != nil {
			logger.Warn("Failed to assign default permissions to role",
				zap.String("role", roleName), zap.Error(err))
		}
	}

	// 设置 WebSocket AllowedOrigins（从配置文件加载）
	if len(appConfig.WebSocket.AllowedOrigins) > 0 {
		websocket.SetAllowedOrigins(appConfig.WebSocket.AllowedOrigins)
		logger.Info("WebSocket allowed origins configured from config.toml",
			zap.Strings("origins", appConfig.WebSocket.AllowedOrigins))
	}

	// 初始化WebSocket Hub
	hub := websocket.NewHubWithDB(supervisorService, db)
	hub.SetSessionValidator(func(userID string, tokenVersion uint64) error {
		return auth.ValidateUserSession(db, userID, tokenVersion)
	})
	go hub.Run()

	// 初始化Alert服务和监控
	alertService := services.NewAlertService(db)
	alertMonitor := services.NewAlertMonitor(alertService, supervisorService, hub)
	alertMonitor.Start()
	logger.Info("Alert Monitor started")

	// 同步 nodelist 配置到数据库（配置作为种子，数据库是唯一真相源）
	logger.Info("Syncing nodelist config to database", zap.Int("config_nodes", len(nodeConfig.Nodes)))
	for _, node := range nodeConfig.Nodes {
		var count int64
		db.Model(&models.Node{}).Where("host = ? AND port = ?", node.Host, node.Port).Count(&count)
		if count > 0 {
			logger.Debug("Node already in database, skipping",
				zap.String("host", node.Host),
				zap.Int("port", node.Port))
			continue
		}
		dbNode := models.Node{
			Name:        node.Name,
			Host:        node.Host,
			Port:        node.Port,
			Username:    node.Username,
			Password:    node.Password,
			Environment: node.Environment,
			Status:      "configured",
		}
		if err := db.Create(&dbNode).Error; err != nil {
			logger.Error("Failed to seed node to database",
				zap.String("name", node.Name),
				zap.Error(err))
		} else {
			logger.Info("Seeded node to database",
				zap.String("name", node.Name),
				zap.String("host", node.Host),
				zap.Int("port", node.Port))
		}
	}

	// 从数据库加载所有节点到 SupervisorService（数据库是唯一真相源）
	var allNodes []models.Node
	if err := db.Find(&allNodes).Error; err != nil {
		logger.Error("Failed to load nodes from database", zap.Error(err))
	} else {
		logger.Info("Loading all nodes from database", zap.Int("count", len(allNodes)))
		for _, node := range allNodes {
			environment := node.Environment
			if environment == "" {
				environment = "default"
			}
			err := supervisorService.AddNode(
				node.Name,
				environment,
				node.Host,
				node.Port,
				node.Username,
				node.Password,
			)
			if err != nil {
				logger.Error("Failed to add node",
					zap.String("node_name", node.Name),
					zap.Error(err))
			} else {
				logger.Info("Node loaded", zap.String("name", node.Name),
					zap.String("host", node.Host), zap.Int("port", node.Port))
			}
		}
	}

	// 启动自动刷新和状态监控（从系统设置读取间隔）
	refreshInterval := getRefreshIntervalFromSettings(db)
	stopRefresh := supervisorService.StartAutoRefresh(refreshInterval)
	stopMonitoring := supervisorService.StartMonitoring(refreshInterval)

	// 设置 WebSocket Hub 的刷新间隔
	processRefreshInterval := getProcessRefreshIntervalFromSettings(db)
	hub.SetRefreshInterval(processRefreshInterval)

	logger.Info("Supervisor auto-refresh and state monitoring started",
		zap.Duration("interval", refreshInterval))
	logger.Info("WebSocket process refresh started",
		zap.Duration("interval", processRefreshInterval))

	// 启动配置监听器，当刷新间隔改变时重启自动刷新和监控
	watchCtx, watchCancel := context.WithCancel(context.Background())
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		watchRefreshIntervalChanges(watchCtx, db, supervisorService, hub, &stopRefresh, &stopMonitoring)
	}()

	// 设置Gin路由
	router := gin.Default()

	// 配置CORS（从配置文件加载，如果未配置则使用默认值）
	corsConfig := cors.DefaultConfig()
	if len(appConfig.CORS.AllowedOrigins) > 0 {
		corsConfig.AllowOrigins = appConfig.CORS.AllowedOrigins
		logger.Info("CORS allowed origins configured from config.toml",
			zap.Strings("origins", appConfig.CORS.AllowedOrigins))
	} else {
		corsConfig.AllowOrigins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	corsConfig.AllowCredentials = true
	router.Use(cors.New(corsConfig))

	// 安全头中间件:设置 X-Content-Type-Options、X-Frame-Options 等安全响应头
	router.Use(middleware.NewValidationMiddleware().SecurityHeaders())

	// 统一错误处理中间件:捕获 panic 并转换为标准错误响应
	router.Use(middleware.ErrorHandler())

	// 获取当前工作目录作为项目根目录
	projectRoot, err := os.Getwd()
	if err != nil {
		logger.Fatal("Failed to get working directory", zap.Error(err))
	}

	// 设置API路由
	api.SetupRoutesWithConfig(router, db, supervisorService, hub, &appConfig.DeveloperTools)

	// 设置 Prometheus metrics 端点
	if appConfig.Metrics.Enabled {
		promMetrics := metrics.NewPrometheusMetrics(supervisorService)
		metricsPath := appConfig.Metrics.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}

		handler := promMetrics.Handler()
		if appConfig.Metrics.Username != "" || appConfig.Metrics.Password != "" {
			authMiddleware := metrics.NewBasicAuthMiddleware(appConfig.Metrics.Username, appConfig.Metrics.Password)
			handler = authMiddleware.Wrap(handler)
		}

		router.GET(metricsPath, gin.WrapF(handler))
		logger.Info("Prometheus metrics endpoint enabled",
			zap.String("path", metricsPath),
			zap.Bool("auth_enabled", appConfig.Metrics.Username != ""))
	}

	// extractToken extracts JWT token from query parameter or Authorization header
	extractToken := func(c *gin.Context) string {
		// Query parameter takes precedence
		if token := c.Query("token"); token != "" {
			return token
		}
		// Fallback to Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			return ""
		}
		// Strip "Bearer " prefix if present
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	// 设置WebSocket路由
	router.GET("/ws", func(c *gin.Context) {
		// Extract token from query parameter or header
		token := extractToken(c)
		if token == "" {
			logger.Warn("WebSocket authentication failed: missing token",
				zap.String("remote_addr", c.ClientIP()))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication failed: missing token"})
			return
		}

		// Validate the token against the current database-backed user session.
		user, claims, err := auth.AuthenticateToken(db, token)
		if err != nil {
			logger.Warn("WebSocket authentication failed: invalid or revoked session",
				zap.String("remote_addr", c.ClientIP()),
				zap.Error(err))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication failed: invalid or revoked session"})
			return
		}

		// Set user context
		c.Set("user_id", user.ID)
		c.Set("userID", user.ID)
		c.Set("user", user)
		c.Set("token_version", claims.TokenVersion)
		logger.Debug("WebSocket authentication successful",
			zap.String("user_id", user.ID),
			zap.String("remote_addr", c.ClientIP()))

		// Upgrade to WebSocket
		hub.HandleWebSocket(c)
	})

	// 服务 React 构建产物
	// 静态资源（JS/CSS/图片等）
	staticPath := filepath.Join(projectRoot, "web", "react-app", "dist", "assets")
	router.Static("/assets", staticPath)

	// 前端应用路由 - 服务于所有前端路由
	router.NoRoute(func(c *gin.Context) {
		// 如果是API请求，返回404
		if strings.HasPrefix(c.Request.URL.Path, "/api") || strings.HasPrefix(c.Request.URL.Path, "/ws") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
			return
		}
		// 否则服务 React 应用的 index.html
		// 设置缓存控制头：不缓存 HTML，让浏览器每次都获取最新版本
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		indexPath := filepath.Join(projectRoot, "web", "react-app", "dist", "index.html")
		c.File(indexPath)
	})

	// 设置优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)

	// 启动配置热重载协程
	go func() {
		for range sigChan {
			logger.Info("Received SIGHUP, reloading configuration")
			newConfig, err := loadConfig()
			if err != nil {
				logger.Error("Failed to reload config", zap.Error(err))
				continue
			}

			// 更新节点配置
			for _, node := range newConfig.Nodes {
				if _, err := supervisorService.GetNode(node.Name); err != nil {
					// 新增节点
					if err := supervisorService.AddNode(
						node.Name,
						node.Environment,
						node.Host,
						node.Port,
						node.Username,
						node.Password,
					); err != nil {
						logger.Error("Failed to add new node",
							zap.String("node_name", node.Name),
							zap.Error(err))
					}
				}
			}

			// 更新admin配置已移除：数据库为管理员信息的唯一真相源
			// 管理员密码/email 应通过 Web UI 或 CLI 修改，不通过热重载覆盖

			nodeConfig = newConfig
			logger.Info("Configuration reloaded successfully")
		}
	}()

	// 启动HTTP服务器
	serverAddr := fmt.Sprintf("0.0.0.0:%d", nodeConfig.Server.Port)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: router,
	}

	// 在goroutine中启动服务器
	go func() {
		logger.Info("Server starting", zap.String("address", serverAddr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// 等待中断信号
	<-quit
	logger.Info("Shutting down server...")

	// 优雅关闭资源
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 关闭HTTP服务器
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	// 关闭WebSocket Hub
	hub.Close()

	// 停止Alert Monitor
	alertMonitor.Stop()
	logger.Info("Alert Monitor stopped")

	// 关闭遗留日志子系统(L-05: 释放文件句柄)
	if loggers.GetActivityLogService() != nil {
		if err := loggers.GetActivityLogService().Close(); err != nil {
			logger.Warn("Failed to close legacy activity log service", zap.Error(err))
		}
		logger.Info("Legacy activity log service closed")
	}

	// 停止配置监听器
	watchCancel()
	<-watchDone
	logger.Info("Refresh interval watcher stopped")

	// 停止自动刷新和监控
	supervisorService.StopAutoRefresh(stopRefresh)
	supervisorService.StopMonitoring(stopMonitoring)

	// 停止性能监控
	if appConfig.Performance.MemoryMonitoringEnabled {
		middleware.StopMemoryMonitoring()
		logger.Info("Memory monitoring stopped")
	}

	if appConfig.Performance.MetricsCleanupEnabled {
		middleware.StopPerformanceCleanup()
		logger.Info("Performance metrics cleanup stopped")
	}

	// 关闭数据库连接
	database.Close()
	logger.Info("Database connection closed")

	logger.Info("Server exited")
}

func loadConfig() (*Config, error) {
	mainConfigPath, nodeListPath := determineConfigPaths()

	// 检查是否有节点配置，如果有则提供迁移建议
	if nodeListPath == "" {
		viper.SetConfigFile(mainConfigPath)
		if err := viper.ReadInConfig(); err == nil {
			if viper.IsSet("nodes") && len(viper.Get("nodes").([]interface{})) > 0 {
				logger.Info("=== Configuration Migration Suggestion ===")
				logger.Info("Your config.toml contains node configurations.")
				logger.Info("Consider migrating to the new config/ directory structure for better maintainability:")
				logger.Info("  1. Create a 'config/' directory")
				logger.Info("  2. Move system configuration to 'config/config.toml'")
				logger.Info("  3. Move node configurations to 'config/nodelist.toml'")
				logger.Info("  4. See config/config.example.toml and config/nodelist.example.toml for reference")
				logger.Info("==========================================")
			}
		}
	}

	// 使用 ConfigLoader 加载配置
	loader := config.NewConfigLoader(mainConfigPath, nodeListPath)
	appCfg, err := loader.LoadWithDefaults()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	// 转换为 main.go 的 Config 结构
	var cfg Config
	cfg.Server.Port = 8081 // 默认端口

	// 从 viper 读取服务器端口（如果存在）
	viper.SetConfigFile(mainConfigPath)
	if err := viper.ReadInConfig(); err == nil {
		cfg.Server.Port = viper.GetInt("server.port")
		if cfg.Server.Port == 0 {
			cfg.Server.Port = 8081
		}
	}

	cfg.Admin.Username = appCfg.AdminUsername
	cfg.Admin.Password = appCfg.AdminPassword
	cfg.Admin.Email = appCfg.Admin.Email
	if cfg.Admin.Email == "" {
		cfg.Admin.Email = "admin@example.com"
	}

	// 转换节点配置
	cfg.Nodes = make([]struct {
		Name        string `mapstructure:"name"`
		Environment string `mapstructure:"environment"`
		Host        string `mapstructure:"host"`
		Port        int    `mapstructure:"port"`
		Username    string `mapstructure:"username"`
		Password    string `mapstructure:"password"`
	}, len(appCfg.Nodes))

	for i, node := range appCfg.Nodes {
		cfg.Nodes[i].Name = node.Name
		cfg.Nodes[i].Environment = node.Environment
		cfg.Nodes[i].Host = node.Host
		cfg.Nodes[i].Port = node.Port
		cfg.Nodes[i].Username = node.Username
		cfg.Nodes[i].Password = node.Password
	}

	logger.Info("Config loaded",
		zap.String("config_file", mainConfigPath),
		zap.Int("nodes_count", len(cfg.Nodes)))

	return &cfg, nil
}

// getRefreshIntervalFromSettings 从系统设置中读取刷新间隔
func getRefreshIntervalFromSettings(db *gorm.DB) time.Duration {
	var setting models.SystemSettings
	result := db.Where("key = ?", "refresh_interval").First(&setting)

	if result.Error != nil || setting.Value == "" {
		// 默认 30 秒
		return 30 * time.Second
	}

	// 解析间隔值（秒）
	interval, err := strconv.Atoi(setting.Value)
	if err != nil || interval <= 0 {
		return 30 * time.Second
	}

	return time.Duration(interval) * time.Second
}

// getProcessRefreshIntervalFromSettings 从系统设置中读取进程页面刷新间隔
func getProcessRefreshIntervalFromSettings(db *gorm.DB) time.Duration {
	var setting models.SystemSettings
	result := db.Where("key = ?", "process_refresh_interval").First(&setting)

	if result.Error != nil || setting.Value == "" {
		// 默认 5 秒
		return 5 * time.Second
	}

	// 解析间隔值（秒）
	interval, err := strconv.Atoi(setting.Value)
	if err != nil || interval <= 0 {
		return 5 * time.Second
	}

	return time.Duration(interval) * time.Second
}

// watchRefreshIntervalChanges 监听刷新间隔配置的变化
func watchRefreshIntervalChanges(ctx context.Context, db *gorm.DB, service *supervisor.SupervisorService, hub *websocket.Hub, stopRefresh *chan struct{}, stopMonitoring *chan struct{}) {
	ticker := time.NewTicker(10 * time.Second) // 每 10 秒检查一次配置
	defer ticker.Stop()

	lastInterval := getRefreshIntervalFromSettings(db)
	lastProcessInterval := getProcessRefreshIntervalFromSettings(db)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping refresh interval watcher")
			return
		case <-ticker.C:
		}

		currentInterval := getRefreshIntervalFromSettings(db)
		currentProcessInterval := getProcessRefreshIntervalFromSettings(db)

		// 检查节点刷新间隔是否变化
		if currentInterval != lastInterval {
			logger.Info("Refresh interval changed, restarting auto-refresh and monitoring",
				zap.Duration("old_interval", lastInterval),
				zap.Duration("new_interval", currentInterval))

			// 停止旧的自动刷新和监控(设置为 nil,避免重复关闭)
			if *stopRefresh != nil {
				close(*stopRefresh)
			}
			if *stopMonitoring != nil {
				close(*stopMonitoring)
			}

			// 启动新的自动刷新和监控
			*stopRefresh = service.StartAutoRefresh(currentInterval)
			*stopMonitoring = service.StartMonitoring(currentInterval)
			lastInterval = currentInterval
		}

		// 检查进程刷新间隔是否变化
		if currentProcessInterval != lastProcessInterval {
			logger.Info("Process refresh interval changed, updating WebSocket hub",
				zap.Duration("old_interval", lastProcessInterval),
				zap.Duration("new_interval", currentProcessInterval))

			hub.SetRefreshInterval(currentProcessInterval)
			lastProcessInterval = currentProcessInterval
		}
	}
}
