package api

import (
	"superview/internal/auth"
	"superview/internal/config"
	"superview/internal/middleware"
	"superview/internal/models"
	"superview/internal/repository"
	"superview/internal/services"
	"superview/internal/supervisor"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WebSocketHub interface for broadcasting
type WebSocketHub interface {
	Broadcast(message []byte)
	GetConnectionCount() int64
}

func SetupRoutes(r *gin.Engine, db *gorm.DB, service *supervisor.SupervisorService, hub WebSocketHub) {
	SetupRoutesWithConfig(r, db, service, hub, nil)
}

// SetupRoutesWithConfig 设置路由,并允许传入开发者工具配置。
// developerToolsConfig 为 nil 时开发者工具默认启用(向后兼容)。
func SetupRoutesWithConfig(r *gin.Engine, db *gorm.DB, service *supervisor.SupervisorService, hub WebSocketHub, developerToolsConfig *config.DeveloperToolsConfig) {
	// 添加性能监控中间件
	r.Use(middleware.PerformanceMiddleware())

	activityLogService := services.NewActivityLogService(db)
	authService := auth.NewAuthService(db, activityLogService)
	permissionChecker := auth.NewPermissionChecker(db)
	nodesAPI := NewNodesAPI(service, db, activityLogService)
	userAPI := NewUserAPI(db, activityLogService)
	environmentsAPI := NewEnvironmentsAPI(service, db)
	groupsAPI := NewGroupsAPI(service, db, activityLogService)
	processesAPI := NewProcessesAPI(service, db, activityLogService)
	activityLogsAPI := NewActivityLogsAPI(activityLogService)
	healthAPI := NewHealthAPI(db, service)
	logManagementAPI := NewLogManagementAPI()

	roleHandler := NewRoleHandler(db, activityLogService)
	processEnhancedHandler := NewProcessEnhancedHandler(db, activityLogService)
	configurationHandler := NewConfigurationHandler(db, activityLogService)
	logAnalysisHandler := NewLogAnalysisHandler(db, activityLogService)

	// Discovery service and API
	// Requirements: 9.3, 9.4 - Authentication required for all discovery endpoints
	discoveryRepo := repository.NewDiscoveryRepository(db)
	nodeRepo := repository.NewNodeRepository(db)
	discoveryService := services.NewDiscoveryService(db, discoveryRepo, nodeRepo, hub, service)
	discoveryAPI := NewDiscoveryAPI(discoveryService, activityLogService)

	// Auth routes
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/login", authService.Login)
		authGroup.POST("/logout", authService.Logout)
		authGroup.GET("/user", authService.AuthMiddleware(), authService.GetCurrentUser)
	}

	// Protected API routes
	apiGroup := r.Group("/api", authService.AuthMiddleware())
	{
		// Health check endpoints
		healthGroup := apiGroup.Group("/health")
		{
			healthGroup.GET("", permissionChecker.RequirePermission(models.PermissionSystemManage), healthAPI.GetHealth)
			healthGroup.GET("/live", healthAPI.GetHealthLive)
			healthGroup.GET("/ready", healthAPI.GetHealthReady)
		}

		// Nodes routes
		nodesGroup := apiGroup.Group("/nodes")
		{
			nodesGroup.GET("", permissionChecker.RequirePermission(models.PermissionNodeRead), nodesAPI.GetNodes)
			nodesGroup.GET("/:node_name", permissionChecker.RequirePermission(models.PermissionNodeRead), nodesAPI.GetNode)
			nodesGroup.PUT("/:node_name", permissionChecker.RequirePermission(models.PermissionNodeWrite), nodesAPI.UpdateNode)
			nodesGroup.GET("/:node_name/processes", permissionChecker.RequirePermission(models.PermissionProcessRead), nodesAPI.GetNodeProcesses)
			nodesGroup.POST("/:node_name/processes/:process_name/start", permissionChecker.RequirePermission(models.PermissionProcessExecute), nodesAPI.StartProcess)
			nodesGroup.POST("/:node_name/processes/:process_name/stop", permissionChecker.RequirePermission(models.PermissionProcessExecute), nodesAPI.StopProcess)
			nodesGroup.POST("/:node_name/processes/:process_name/restart", permissionChecker.RequirePermission(models.PermissionProcessExecute), nodesAPI.RestartProcess)
			nodesGroup.GET("/:node_name/processes/:process_name/logs", permissionChecker.RequirePermission(models.PermissionLogRead), nodesAPI.GetProcessLogs)
			nodesGroup.GET("/:node_name/processes/:process_name/logs/page", permissionChecker.RequirePermission(models.PermissionLogRead), nodesAPI.ReadProcessLogs)
			nodesGroup.POST("/:node_name/processes/:process_name/logs/clear", permissionChecker.RequirePermission(models.PermissionLogWrite), nodesAPI.ClearProcessLogs)
			nodesGroup.GET("/:node_name/processes/:process_name/logs/stream", permissionChecker.RequirePermission(models.PermissionLogRead), nodesAPI.GetProcessLogStream)
			// Batch operations
			nodesGroup.POST("/:node_name/processes/start-all", permissionChecker.RequirePermission(models.PermissionProcessExecute), nodesAPI.StartAllProcesses)
			nodesGroup.POST("/:node_name/processes/stop-all", permissionChecker.RequirePermission(models.PermissionProcessExecute), nodesAPI.StopAllProcesses)
			nodesGroup.POST("/:node_name/processes/restart-all", permissionChecker.RequirePermission(models.PermissionProcessExecute), nodesAPI.RestartAllProcesses)
			// Config management (getAllConfigInfo / reloadConfig)
			nodesGroup.GET("/:node_name/processes/configs", permissionChecker.RequirePermission(models.PermissionProcessRead), nodesAPI.GetAllConfigInfo)
			nodesGroup.POST("/:node_name/processes/reload-config", permissionChecker.RequirePermission(models.PermissionProcessWrite), nodesAPI.ReloadConfig)
		}

		// User management API
		userHandler := NewUserHandler(db, activityLogService)
		userGroup := apiGroup.Group("/users")
		{
			userGroup.GET("", permissionChecker.RequirePermission(models.PermissionUserRead), userHandler.GetUsers)
			userGroup.POST("", permissionChecker.RequirePermission(models.PermissionUserWrite), userHandler.CreateUser)
			userGroup.GET("/:id", permissionChecker.RequirePermission(models.PermissionUserRead), userHandler.GetUserByID)
			userGroup.PUT("/:id", permissionChecker.RequirePermission(models.PermissionUserWrite), userHandler.UpdateUser)
			userGroup.DELETE("/:id", permissionChecker.RequirePermission(models.PermissionUserDelete), userHandler.DeleteUser)
			userGroup.PUT("/:id/password", permissionChecker.RequirePermission(models.PermissionUserWrite), userHandler.ResetPassword)
			userGroup.PATCH("/:id/toggle", permissionChecker.RequirePermission(models.PermissionUserWrite), userHandler.ToggleUserStatus)
			userGroup.GET("/:id/node-access", permissionChecker.RequirePermission(models.PermissionUserRead), userHandler.GetUserNodeAccess)
			userGroup.PUT("/:id/node-access", permissionChecker.RequirePermission(models.PermissionUserWrite), userHandler.UpdateUserNodeAccess)
		}

		// Profile management API
		profileGroup := apiGroup.Group("/profile")
		{
			profileGroup.GET("", userAPI.GetProfile)
			profileGroup.PUT("", userAPI.UpdateProfile)
			profileGroup.PUT("/password", userAPI.ChangeOwnPassword)
		}

		// Environments API
		environmentsGroup := apiGroup.Group("/environments")
		{
			environmentsGroup.GET("", permissionChecker.RequirePermission(models.PermissionNodeRead), environmentsAPI.GetEnvironments)
			environmentsGroup.GET("/:environment_name", permissionChecker.RequirePermission(models.PermissionNodeRead), environmentsAPI.GetEnvironmentDetails)
		}

		// Groups API
		groupsGroup := apiGroup.Group("/groups")
		{
			groupsGroup.GET("", permissionChecker.RequirePermission(models.PermissionProcessRead), groupsAPI.GetGroups)
			groupsGroup.GET("/:group_name", permissionChecker.RequirePermission(models.PermissionProcessRead), groupsAPI.GetGroupDetails)
			groupsGroup.POST("/:group_name/start", permissionChecker.RequirePermission(models.PermissionProcessExecute), groupsAPI.StartGroupProcesses)
			groupsGroup.POST("/:group_name/stop", permissionChecker.RequirePermission(models.PermissionProcessExecute), groupsAPI.StopGroupProcesses)
			groupsGroup.POST("/:group_name/restart", permissionChecker.RequirePermission(models.PermissionProcessExecute), groupsAPI.RestartGroupProcesses)
		}

		// Processes Aggregation API
		processesGroup := apiGroup.Group("/processes")
		{
			processesGroup.GET("/aggregated", permissionChecker.RequirePermission(models.PermissionProcessRead), processesAPI.GetAggregatedProcesses)
			processesGroup.POST("/:process_name/start", permissionChecker.RequirePermission(models.PermissionProcessExecute), processesAPI.BatchStartProcess)
			processesGroup.POST("/:process_name/stop", permissionChecker.RequirePermission(models.PermissionProcessExecute), processesAPI.BatchStopProcess)
			processesGroup.POST("/:process_name/restart", permissionChecker.RequirePermission(models.PermissionProcessExecute), processesAPI.BatchRestartProcess)
		}

		// Activity Logs API
		activityLogsGroup := apiGroup.Group("/activity-logs")
		{
			activityLogsGroup.GET("", permissionChecker.RequirePermission(models.PermissionLogRead), activityLogsAPI.GetActivityLogs)
			activityLogsGroup.GET("/recent", permissionChecker.RequirePermission(models.PermissionLogRead), activityLogsAPI.GetRecentLogs)
			activityLogsGroup.GET("/statistics", permissionChecker.RequirePermission(models.PermissionLogRead), activityLogsAPI.GetLogStatistics)
			activityLogsGroup.GET("/export", permissionChecker.RequirePermission(models.PermissionLogRead), activityLogsAPI.ExportLogs)
			activityLogsGroup.DELETE("/clean", permissionChecker.RequirePermission(models.PermissionLogDelete), activityLogsAPI.CleanOldLogs)
			activityLogsGroup.DELETE("", permissionChecker.RequirePermission(models.PermissionLogDelete), activityLogsAPI.DeleteLogs)
		}

		// Roles and Permissions API
		rolesGroup := apiGroup.Group("/roles")
		{
			rolesGroup.GET("", permissionChecker.RequirePermission(models.PermissionUserRead), roleHandler.GetRoles)
			rolesGroup.POST("", permissionChecker.RequirePermission(models.PermissionUserWrite), roleHandler.CreateRole)
			rolesGroup.GET("/:id", permissionChecker.RequirePermission(models.PermissionUserRead), roleHandler.GetRole)
			rolesGroup.PUT("/:id", permissionChecker.RequirePermission(models.PermissionUserWrite), roleHandler.UpdateRole)
			rolesGroup.DELETE("/:id", permissionChecker.RequirePermission(models.PermissionUserDelete), roleHandler.DeleteRole)
			rolesGroup.POST("/:id/permissions", permissionChecker.RequirePermission(models.PermissionUserWrite), roleHandler.AssignPermissions)
		}

		// Role-User assignment API (separate group to avoid conflicts)
		roleUsersGroup := apiGroup.Group("/role-users")
		{
			roleUsersGroup.POST("/:roleId/users/:userId", permissionChecker.RequirePermission(models.PermissionUserWrite), roleHandler.AssignRoleToUser)
			roleUsersGroup.DELETE("/:roleId/users/:userId", permissionChecker.RequirePermission(models.PermissionUserWrite), roleHandler.RemoveRoleFromUser)
		}

		// Permissions API
		permissionsGroup := apiGroup.Group("/permissions")
		{
			permissionsGroup.GET("", permissionChecker.RequirePermission(models.PermissionUserRead), roleHandler.GetPermissions)
		}

		// Alerts API
		alertHandler := NewAlertHandler(db, hub, activityLogService)
		alertsGroup := apiGroup.Group("/alerts", permissionChecker.RequirePermission(models.PermissionSystemManage))
		{
			// Alert rules management
			alertsGroup.POST("/rules", alertHandler.CreateAlertRule)
			alertsGroup.GET("/rules", alertHandler.GetAlertRules)
			alertsGroup.GET("/rules/:id", alertHandler.GetAlertRule)
			alertsGroup.PUT("/rules/:id", alertHandler.UpdateAlertRule)
			alertsGroup.DELETE("/rules/:id", alertHandler.DeleteAlertRule)

			// Alert records management
			alertsGroup.GET("", alertHandler.GetAlerts)
			alertsGroup.GET("/:id", alertHandler.GetAlert)
			alertsGroup.POST("/:id/acknowledge", alertHandler.AcknowledgeAlert)
			alertsGroup.POST("/:id/resolve", alertHandler.ResolveAlert)

			// Notification channels management
			alertsGroup.POST("/channels", alertHandler.CreateNotificationChannel)
			alertsGroup.GET("/channels", alertHandler.GetNotificationChannels)
			alertsGroup.GET("/channels/:id", alertHandler.GetNotificationChannel)
			alertsGroup.PUT("/channels/:id", alertHandler.UpdateNotificationChannel)
			alertsGroup.DELETE("/channels/:id", alertHandler.DeleteNotificationChannel)
			alertsGroup.POST("/channels/:id/test", alertHandler.TestNotificationChannel)

			// System metrics and statistics
			alertsGroup.POST("/metrics", alertHandler.RecordSystemMetric)
			alertsGroup.GET("/metrics", alertHandler.GetSystemMetrics)
			alertsGroup.GET("/statistics", alertHandler.GetAlertStatistics)
		}

		// Process Enhanced API
		processEnhancedGroup := apiGroup.Group("/process-enhanced")
		{
			// Task scheduler management
			processEnhancedGroup.POST("/scheduler/start", permissionChecker.RequirePermission(models.PermissionProcessExecute), processEnhancedHandler.StartScheduler)
			processEnhancedGroup.POST("/scheduler/stop", permissionChecker.RequirePermission(models.PermissionProcessExecute), processEnhancedHandler.StopScheduler)

			// Process group management
			processEnhancedGroup.POST("/groups", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.CreateProcessGroup)
			processEnhancedGroup.GET("/groups", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetProcessGroups)
			processEnhancedGroup.GET("/groups/:id", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetProcessGroup)
			processEnhancedGroup.PUT("/groups/:id", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.UpdateProcessGroup)
			processEnhancedGroup.DELETE("/groups/:id", permissionChecker.RequirePermission(models.PermissionProcessDelete), processEnhancedHandler.DeleteProcessGroup)
			processEnhancedGroup.POST("/groups/:id/processes", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.AddProcessToGroup)
			processEnhancedGroup.DELETE("/groups/:id/processes", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.RemoveProcessFromGroup)
			processEnhancedGroup.PUT("/groups/:id/reorder", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.ReorderProcessesInGroup)

			// Process dependency management
			processEnhancedGroup.POST("/dependencies", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.CreateProcessDependency)
			processEnhancedGroup.GET("/dependencies", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetProcessDependencies)
			processEnhancedGroup.GET("/dependent-processes", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetDependentProcesses)
			processEnhancedGroup.DELETE("/dependencies/:id", permissionChecker.RequirePermission(models.PermissionProcessDelete), processEnhancedHandler.DeleteProcessDependency)
			processEnhancedGroup.POST("/startup-order", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetStartupOrder)

			// Scheduled task management
			processEnhancedGroup.POST("/scheduled-tasks", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.CreateScheduledTask)
			processEnhancedGroup.GET("/scheduled-tasks", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetScheduledTasks)
			processEnhancedGroup.GET("/scheduled-tasks/:id", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetScheduledTask)
			processEnhancedGroup.PUT("/scheduled-tasks/:id", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.UpdateScheduledTask)
			processEnhancedGroup.DELETE("/scheduled-tasks/:id", permissionChecker.RequirePermission(models.PermissionProcessDelete), processEnhancedHandler.DeleteScheduledTask)
			processEnhancedGroup.GET("/scheduled-tasks/:id/executions", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetTaskExecutions)

			// Process template management
			processEnhancedGroup.POST("/templates", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.CreateProcessTemplate)
			processEnhancedGroup.GET("/templates", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetProcessTemplates)
			processEnhancedGroup.GET("/templates/:id", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetProcessTemplate)
			processEnhancedGroup.PUT("/templates/:id", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.UpdateProcessTemplate)
			processEnhancedGroup.DELETE("/templates/:id", permissionChecker.RequirePermission(models.PermissionProcessDelete), processEnhancedHandler.DeleteProcessTemplate)
			processEnhancedGroup.POST("/templates/:id/use", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.UseTemplate)

			// Process performance metrics
			processEnhancedGroup.POST("/metrics", permissionChecker.RequirePermission(models.PermissionProcessWrite), processEnhancedHandler.RecordProcessMetrics)
			processEnhancedGroup.GET("/metrics", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetProcessMetrics)
			processEnhancedGroup.GET("/metrics/statistics", permissionChecker.RequirePermission(models.PermissionProcessRead), processEnhancedHandler.GetProcessMetricsStatistics)

			// Data cleanup
			processEnhancedGroup.POST("/cleanup", permissionChecker.RequirePermission(models.PermissionProcessDelete), processEnhancedHandler.CleanupOldData)
		}

		// Configuration API
		configurationGroup := apiGroup.Group("/configuration")
		{
			// 配置项管理
			configurationGroup.GET("", permissionChecker.RequirePermission(models.PermissionConfigRead), configurationHandler.GetConfigurations)
			configurationGroup.POST("", permissionChecker.RequirePermission(models.PermissionConfigWrite), configurationHandler.CreateConfiguration)
			configurationGroup.GET("/:id", permissionChecker.RequirePermission(models.PermissionConfigRead), configurationHandler.GetConfiguration)
			configurationGroup.PUT("/:id", permissionChecker.RequirePermission(models.PermissionConfigWrite), configurationHandler.UpdateConfiguration)
			configurationGroup.DELETE("/:id", permissionChecker.RequirePermission(models.PermissionConfigDelete), configurationHandler.DeleteConfiguration)

			// 环境变量管理
			configurationGroup.GET("/env-vars", permissionChecker.RequirePermission(models.PermissionEnvVarRead), configurationHandler.GetEnvironmentVariables)
			configurationGroup.POST("/env-vars", permissionChecker.RequirePermission(models.PermissionEnvVarWrite), configurationHandler.CreateEnvironmentVariable)
			configurationGroup.GET("/env-vars/:id", permissionChecker.RequirePermission(models.PermissionEnvVarRead), configurationHandler.GetEnvironmentVariable)
			configurationGroup.PUT("/env-vars/:id", permissionChecker.RequirePermission(models.PermissionEnvVarWrite), configurationHandler.UpdateEnvironmentVariable)
			configurationGroup.DELETE("/env-vars/:id", permissionChecker.RequirePermission(models.PermissionEnvVarDelete), configurationHandler.DeleteEnvironmentVariable)

			// 配置导入导出
			configurationGroup.GET("/export", permissionChecker.RequirePermission(models.PermissionConfigRead), configurationHandler.ExportConfigurations)
			configurationGroup.POST("/import", permissionChecker.RequirePermission(models.PermissionConfigWrite), configurationHandler.ImportConfigurations)

			// 配置变更历史
			configurationGroup.GET("/history", permissionChecker.RequirePermission(models.PermissionConfigRead), configurationHandler.GetConfigurationHistory)

			// 审计日志
			configurationGroup.GET("/audit-logs", permissionChecker.RequirePermission(models.PermissionConfigRead), configurationHandler.GetAuditLogs)

			// 数据清理
			configurationGroup.POST("/cleanup", permissionChecker.RequirePermission(models.PermissionConfigDelete), configurationHandler.CleanupOldData)
		}

		// Log Analysis API
		logAnalysisGroup := apiGroup.Group("/logs")
		{
			// 日志条目管理
			logAnalysisGroup.GET("", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetLogEntries)
			logAnalysisGroup.POST("", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.CreateLogEntry)
			logAnalysisGroup.GET("/:id", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetLogEntry)
			logAnalysisGroup.DELETE("/:id", permissionChecker.RequirePermission(models.PermissionLogDelete), logAnalysisHandler.DeleteLogEntry)

			// 分析规则管理
			logAnalysisGroup.GET("/rules", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetAnalysisRules)
			logAnalysisGroup.POST("/rules", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.CreateAnalysisRule)
			logAnalysisGroup.GET("/rules/:id", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetAnalysisRule)
			logAnalysisGroup.PUT("/rules/:id", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.UpdateAnalysisRule)
			logAnalysisGroup.DELETE("/rules/:id", permissionChecker.RequirePermission(models.PermissionLogDelete), logAnalysisHandler.DeleteAnalysisRule)

			// 日志统计
			logAnalysisGroup.GET("/statistics", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetLogStatistics)

			// 日志告警
			logAnalysisGroup.GET("/alerts", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetLogAlerts)
			logAnalysisGroup.POST("/alerts/:id/acknowledge", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.AcknowledgeAlert)
			logAnalysisGroup.POST("/alerts/:id/resolve", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.ResolveAlert)

			// 日志过滤器
			logAnalysisGroup.GET("/filters", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetLogFilters)
			logAnalysisGroup.POST("/filters", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.CreateLogFilter)
			logAnalysisGroup.PUT("/filters/:id", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.UpdateLogFilter)
			logAnalysisGroup.DELETE("/filters/:id", permissionChecker.RequirePermission(models.PermissionLogDelete), logAnalysisHandler.DeleteLogFilter)

			// 日志导出
			logAnalysisGroup.GET("/exports", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetLogExports)
			logAnalysisGroup.POST("/exports", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.CreateLogExport)
			logAnalysisGroup.GET("/exports/:id", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetLogExport)
			logAnalysisGroup.DELETE("/exports/:id", permissionChecker.RequirePermission(models.PermissionLogDelete), logAnalysisHandler.DeleteLogExport)

			// 保留策略
			logAnalysisGroup.GET("/retention-policies", permissionChecker.RequirePermission(models.PermissionLogRead), logAnalysisHandler.GetRetentionPolicies)
			logAnalysisGroup.POST("/retention-policies", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.CreateRetentionPolicy)
			logAnalysisGroup.PUT("/retention-policies/:id", permissionChecker.RequirePermission(models.PermissionLogWrite), logAnalysisHandler.UpdateRetentionPolicy)
			logAnalysisGroup.DELETE("/retention-policies/:id", permissionChecker.RequirePermission(models.PermissionLogDelete), logAnalysisHandler.DeleteRetentionPolicy)
			logAnalysisGroup.POST("/retention-policies/execute", permissionChecker.RequirePermission(models.PermissionLogDelete), logAnalysisHandler.ExecuteRetentionPolicies)

			// 数据清理
			logAnalysisGroup.POST("/cleanup", permissionChecker.RequirePermission(models.PermissionLogDelete), logAnalysisHandler.CleanupOldLogs)
		}

		// Data Management API
		dataManagementHandler := NewDataManagementAPI(activityLogService)
		dataManagementGroup := apiGroup.Group("/data-management", permissionChecker.RequirePermission(models.PermissionSystemManage))
		{
			// 数据导出
			dataManagementGroup.POST("/export", dataManagementHandler.ExportData)
			dataManagementGroup.POST("/exports", dataManagementHandler.ExportData)
			dataManagementGroup.GET("/exports", dataManagementHandler.GetExportRecords)
			dataManagementGroup.GET("/exports/:id/download", dataManagementHandler.DownloadExportFile)
			dataManagementGroup.DELETE("/exports/:id", dataManagementHandler.DeleteExportRecord)

			// 数据导入
			dataManagementGroup.POST("/import", dataManagementHandler.ImportData)
			dataManagementGroup.POST("/imports", dataManagementHandler.ImportData)
			dataManagementGroup.GET("/imports", dataManagementHandler.GetImportRecords)
			dataManagementGroup.DELETE("/imports/:id", dataManagementHandler.DeleteImportRecord)
		}

		// System Settings API
		systemSettingsHandler := NewSystemSettingsAPI(db, activityLogService)
		systemSettingsGroup := apiGroup.Group("/system-settings")
		{
			// 系统设置管理
			systemSettingsGroup.GET("", permissionChecker.RequirePermission(models.PermissionSystemConfig), systemSettingsHandler.GetSystemSettings)
			systemSettingsGroup.GET("/:key", permissionChecker.RequirePermission(models.PermissionSystemConfig), systemSettingsHandler.GetSystemSetting)
			systemSettingsGroup.PUT("/:key", permissionChecker.RequirePermission(models.PermissionSystemConfig), systemSettingsHandler.UpdateSystemSetting)
			systemSettingsGroup.PUT("/batch", permissionChecker.RequirePermission(models.PermissionSystemConfig), systemSettingsHandler.UpdateMultipleSettings)
			systemSettingsGroup.DELETE("/:key", permissionChecker.RequirePermission(models.PermissionSystemConfig), systemSettingsHandler.DeleteSystemSetting)
			systemSettingsGroup.POST("/reset", permissionChecker.RequirePermission(models.PermissionSystemManage), systemSettingsHandler.ResetToDefaults)

			// 用户偏好设置（当前用户）
			systemSettingsGroup.GET("/user-preferences", systemSettingsHandler.GetUserPreferences)
			systemSettingsGroup.PUT("/user-preferences", systemSettingsHandler.UpdateUserPreferences)

			// 管理员管理其他用户偏好
			systemSettingsGroup.GET("/users/:userId/preferences", permissionChecker.RequirePermission(models.PermissionUserRead), systemSettingsHandler.GetUserPreferencesByAdmin)
			systemSettingsGroup.PUT("/users/:userId/preferences", permissionChecker.RequirePermission(models.PermissionUserWrite), systemSettingsHandler.UpdateUserPreferencesByAdmin)

			// 邮件配置测试
			systemSettingsGroup.POST("/test-email", permissionChecker.RequirePermission(models.PermissionSystemConfig), systemSettingsHandler.TestEmailConfiguration)
		}

		// Developer Tools API
		developerToolsHandler := NewDeveloperToolsAPI(db, service, developerToolsConfig, hub)
		developerGroup := apiGroup.Group("/developer", permissionChecker.RequirePermission(models.PermissionSystemManage))
		developerGroup.Use(developerToolsHandler.RequireEnabled())
		{
			// API 文档
			developerGroup.GET("/api-docs", developerToolsHandler.GetApiEndpoints)
			developerGroup.POST("/test-api", developerToolsHandler.TestApiEndpoint)

			// 调试工具
			developerGroup.GET("/debug-logs", developerToolsHandler.GetDebugLogs)
			developerGroup.DELETE("/debug-logs", developerToolsHandler.ClearDebugLogs)
			developerGroup.PUT("/log-level", developerToolsHandler.SetLogLevel)

			// 性能监控
			developerGroup.GET("/performance", developerToolsHandler.GetPerformanceMetrics)
			developerGroup.POST("/performance/reset", developerToolsHandler.ResetPerformanceMetrics)
			developerGroup.GET("/performance/slow-endpoints", developerToolsHandler.GetTopSlowEndpoints)
			developerGroup.GET("/performance/error-rates", developerToolsHandler.GetErrorRateByEndpoint)
			developerGroup.GET("/system-metrics", developerToolsHandler.GetSystemMetrics)
			developerGroup.GET("/api-metrics", developerToolsHandler.GetApiMetrics)
			developerGroup.GET("/database-metrics", developerToolsHandler.GetDatabaseMetrics)
			developerGroup.GET("/websocket-metrics", developerToolsHandler.GetWebSocketMetrics)

			// 日志级别管理
			logGroup := developerGroup.Group("/logs")
			{
				logGroup.GET("/level", logManagementAPI.GetLogLevel)
				logGroup.PUT("/level", logManagementAPI.SetLogLevel)
				logGroup.POST("/level/temporary", logManagementAPI.SetTemporaryLogLevel)
				logGroup.POST("/level/reset", logManagementAPI.ResetLogLevel)
				logGroup.GET("/levels", logManagementAPI.GetAvailableLogLevels)
				logGroup.DELETE("/level/history", logManagementAPI.ClearLogLevelHistory)
			}
		}

		// Discovery API - Node discovery and network scanning
		// Requirements: 9.3, 9.4 - All discovery endpoints require authentication
		discoveryGroup := apiGroup.Group("/discovery")
		{
			discoveryGroup.POST("/tasks", discoveryAPI.StartDiscovery)
			discoveryGroup.GET("/tasks", discoveryAPI.ListTasks)
			discoveryGroup.GET("/tasks/:id", discoveryAPI.GetTask)
			discoveryGroup.POST("/tasks/:id/cancel", discoveryAPI.CancelTask)
			discoveryGroup.DELETE("/tasks/:id", discoveryAPI.DeleteTask)
			discoveryGroup.GET("/tasks/:id/progress", discoveryAPI.GetTaskProgress)
			discoveryGroup.POST("/validate-cidr", discoveryAPI.ValidateCIDR)
		}
	}
}
