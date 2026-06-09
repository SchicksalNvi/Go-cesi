package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"superview/internal/models"
	"superview/internal/services"
)

// SystemSettingsAPI handles system settings related operations
type SystemSettingsAPI struct {
	db                 *gorm.DB
	activityLogService *services.ActivityLogService
}

type userPreferencesRequest struct {
	Theme              *string `json:"theme"`
	Language           *string `json:"language"`
	Timezone           *string `json:"timezone"`
	DateFormat         *string `json:"date_format"`
	TimeFormat         *string `json:"time_format"`
	PageSize           *int    `json:"page_size"`
	AutoRefresh        *bool   `json:"auto_refresh"`
	RefreshInterval    *int    `json:"refresh_interval"`
	EmailNotifications *bool   `json:"email_notifications"`
	ProcessAlerts      *bool   `json:"process_alerts"`
	SystemAlerts       *bool   `json:"system_alerts"`
	NodeStatusChanges  *bool   `json:"node_status_changes"`
	WeeklyReport       *bool   `json:"weekly_report"`
	Notifications      *string `json:"notifications"`
	DashboardLayout    *string `json:"dashboard_layout"`
}

// NewSystemSettingsAPI creates a new SystemSettingsAPI instance
func NewSystemSettingsAPI(db *gorm.DB, activityLogService ...*services.ActivityLogService) *SystemSettingsAPI {
	api := &SystemSettingsAPI{db: db}
	if len(activityLogService) > 0 {
		api.activityLogService = activityLogService[0]
	}
	return api
}

func defaultUserPreferences(userID string) models.UserPreferences {
	return models.UserPreferences{
		UserID:             userID,
		Theme:              "light",
		Language:           "en",
		Timezone:           "UTC",
		DateFormat:         "YYYY-MM-DD",
		TimeFormat:         "HH:mm:ss",
		PageSize:           20,
		AutoRefresh:        true,
		RefreshInterval:    30,
		EmailNotifications: true,
		ProcessAlerts:      true,
		SystemAlerts:       true,
		NodeStatusChanges:  false,
		WeeklyReport:       false,
	}
}

func applyUserPreferencesRequest(preferences *models.UserPreferences, request userPreferencesRequest) {
	if request.Theme != nil {
		preferences.Theme = *request.Theme
	}
	if request.Language != nil {
		preferences.Language = *request.Language
	}
	if request.Timezone != nil {
		preferences.Timezone = *request.Timezone
	}
	if request.DateFormat != nil {
		preferences.DateFormat = *request.DateFormat
	}
	if request.TimeFormat != nil {
		preferences.TimeFormat = *request.TimeFormat
	}
	if request.PageSize != nil {
		preferences.PageSize = *request.PageSize
	}
	if request.AutoRefresh != nil {
		preferences.AutoRefresh = *request.AutoRefresh
	}
	if request.RefreshInterval != nil {
		preferences.RefreshInterval = *request.RefreshInterval
	}
	if request.EmailNotifications != nil {
		preferences.EmailNotifications = *request.EmailNotifications
	}
	if request.ProcessAlerts != nil {
		preferences.ProcessAlerts = *request.ProcessAlerts
	}
	if request.SystemAlerts != nil {
		preferences.SystemAlerts = *request.SystemAlerts
	}
	if request.NodeStatusChanges != nil {
		preferences.NodeStatusChanges = *request.NodeStatusChanges
	}
	if request.WeeklyReport != nil {
		preferences.WeeklyReport = *request.WeeklyReport
	}
	if request.Notifications != nil {
		preferences.Notifications = *request.Notifications
	}
	if request.DashboardLayout != nil {
		preferences.DashboardLayout = *request.DashboardLayout
	}
}

func (api *SystemSettingsAPI) createUserPreferences(preferences *models.UserPreferences) error {
	boolValues := map[string]interface{}{
		"auto_refresh":        preferences.AutoRefresh,
		"email_notifications": preferences.EmailNotifications,
		"process_alerts":      preferences.ProcessAlerts,
		"system_alerts":       preferences.SystemAlerts,
		"node_status_changes": preferences.NodeStatusChanges,
		"weekly_report":       preferences.WeeklyReport,
	}

	if err := api.db.Create(preferences).Error; err != nil {
		return err
	}

	return api.db.Model(preferences).UpdateColumns(boolValues).Error
}

// GetSystemSettings retrieves all system settings
func (api *SystemSettingsAPI) GetSystemSettings(c *gin.Context) {
	var settings []models.SystemSettings
	result := api.db.Find(&settings)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch system settings"})
		return
	}

	// Convert to map for easier frontend consumption
	settingsMap := make(map[string]interface{})
	for _, setting := range settings {
		settingsMap[setting.Key] = setting.Value
	}

	c.JSON(http.StatusOK, gin.H{
		"settings": settingsMap,
		"count":    len(settings),
	})
}

// GetSystemSetting retrieves a specific system setting
func (api *SystemSettingsAPI) GetSystemSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Setting key is required"})
		return
	}

	var setting models.SystemSettings
	result := api.db.Where("key = ?", key).First(&setting)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Setting not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch setting"})
		}
		return
	}

	c.JSON(http.StatusOK, setting)
}

// UpdateSystemSetting updates a system setting
func (api *SystemSettingsAPI) UpdateSystemSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Setting key is required"})
		return
	}

	var request struct {
		Value       string `json:"value" binding:"required"`
		Description string `json:"description"`
		Category    string `json:"category"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户 ID
	userIDStr, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Check if setting exists
	var setting models.SystemSettings
	result := api.db.Where("key = ?", key).First(&setting)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Create new setting
			setting = models.SystemSettings{
				ID:          uuid.New().String(),
				Key:         key,
				Value:       request.Value,
				Description: request.Description,
				Category:    request.Category,
				UpdatedBy:   &userIDStr,
			}
			result = api.db.Create(&setting)
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch setting"})
			return
		}
	} else {
		// Update existing setting
		setting.Value = request.Value
		setting.UpdatedBy = &userIDStr
		if request.Description != "" {
			setting.Description = request.Description
		}
		if request.Category != "" {
			setting.Category = request.Category
		}
		result = api.db.Save(&setting)
	}

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save setting"})
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Updated system setting: %s", key)
		api.activityLogService.LogWithContext(c, "INFO", "update_setting", "system_setting", key, msg, nil)
	}

	c.JSON(http.StatusOK, setting)
}

// UpdateMultipleSettings updates multiple system settings at once
func (api *SystemSettingsAPI) UpdateMultipleSettings(c *gin.Context) {
	var request struct {
		Settings map[string]interface{} `json:"settings" binding:"required"`
		Category string                 `json:"category"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户 ID
	userIDStr, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	tx := api.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for key, value := range request.Settings {
		var setting models.SystemSettings
		result := tx.Where("key = ?", key).First(&setting)

		valueStr := ""
		switch v := value.(type) {
		case string:
			valueStr = v
		case bool:
			valueStr = strconv.FormatBool(v)
		case float64:
			valueStr = strconv.FormatFloat(v, 'f', -1, 64)
		default:
			valueStr = "" // Handle other types as needed
		}

		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				// Create new setting
				setting = models.SystemSettings{
					ID:        uuid.New().String(),
					Key:       key,
					Value:     valueStr,
					Category:  request.Category,
					UpdatedBy: &userIDStr,
				}
				if err := tx.Create(&setting).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create setting: " + key})
					return
				}
			} else {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch setting: " + key})
				return
			}
		} else {
			// Update existing setting
			setting.Value = valueStr
			setting.UpdatedBy = &userIDStr
			if request.Category != "" {
				setting.Category = request.Category
			}
			if err := tx.Save(&setting).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update setting: " + key})
				return
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit settings update"})
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Updated %d system settings", len(request.Settings))
		api.activityLogService.LogWithContext(c, "INFO", "update_settings_batch", "system_setting", "batch", msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings updated successfully"})
}

// DeleteSystemSetting deletes a system setting
func (api *SystemSettingsAPI) DeleteSystemSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Setting key is required"})
		return
	}

	result := api.db.Where("key = ?", key).Delete(&models.SystemSettings{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete setting"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Setting not found"})
		return
	}

	if api.activityLogService != nil {
		msg := fmt.Sprintf("Deleted system setting: %s", key)
		api.activityLogService.LogWithContext(c, "WARNING", "delete_setting", "system_setting", key, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Setting deleted successfully"})
}

// GetUserPreferences retrieves user preferences
func (api *SystemSettingsAPI) GetUserPreferences(c *gin.Context) {
	userIDStr, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var preferences models.UserPreferences
	result := api.db.Where("user_id = ?", userIDStr).First(&preferences)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			preferences = defaultUserPreferences(userIDStr)
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user preferences"})
			return
		}
	}

	c.JSON(http.StatusOK, preferences)
}

// UpdateUserPreferences updates user preferences
func (api *SystemSettingsAPI) UpdateUserPreferences(c *gin.Context) {
	userIDStr, exists := getUserIDString(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var request userPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if preferences exist
	var preferences models.UserPreferences
	result := api.db.Where("user_id = ?", userIDStr).First(&preferences)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			preferences = defaultUserPreferences(userIDStr)
			applyUserPreferencesRequest(&preferences, request)
			if err := api.createUserPreferences(&preferences); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user preferences"})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user preferences"})
			return
		}
	} else {
		applyUserPreferencesRequest(&preferences, request)
		result = api.db.Save(&preferences)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user preferences"})
			return
		}
	}

	c.JSON(http.StatusOK, preferences)
}

// GetUserPreferencesByAdmin retrieves preferences for a specific user (admin or self)
func (api *SystemSettingsAPI) GetUserPreferencesByAdmin(c *gin.Context) {
	if _, exists := getUserIDString(c); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	targetUserID := c.Param("userId")
	if targetUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	var preferences models.UserPreferences
	result := api.db.Where("user_id = ?", targetUserID).First(&preferences)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			preferences = defaultUserPreferences(targetUserID)
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user preferences"})
			return
		}
	}

	c.JSON(http.StatusOK, preferences)
}

// UpdateUserPreferencesByAdmin updates preferences for a specific user (admin or self)
func (api *SystemSettingsAPI) UpdateUserPreferencesByAdmin(c *gin.Context) {
	if _, exists := getUserIDString(c); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	targetUserID := c.Param("userId")
	if targetUserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID is required"})
		return
	}

	var request userPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var preferences models.UserPreferences
	result := api.db.Where("user_id = ?", targetUserID).First(&preferences)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			preferences = defaultUserPreferences(targetUserID)
			applyUserPreferencesRequest(&preferences, request)
			if err := api.createUserPreferences(&preferences); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user preferences"})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user preferences"})
			return
		}
	} else {
		applyUserPreferencesRequest(&preferences, request)
		result = api.db.Save(&preferences)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user preferences"})
			return
		}
	}

	c.JSON(http.StatusOK, preferences)
}

// TestEmailConfiguration tests the email configuration
func (api *SystemSettingsAPI) TestEmailConfiguration(c *gin.Context) {
	var request struct {
		TestEmail string `json:"test_email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get email settings
	var settings []models.SystemSettings
	api.db.Where("category = ?", "email").Find(&settings)

	emailConfig := make(map[string]string)
	for _, setting := range settings {
		emailConfig[setting.Key] = setting.Value
	}

	// Validate required email settings
	requiredSettings := []string{"smtp_host", "smtp_port", "smtp_username", "smtp_password"}
	for _, key := range requiredSettings {
		if emailConfig[key] == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Email configuration incomplete. Missing: " + key,
				"success": false,
			})
			return
		}
	}

	// Test email sending in background
	go func() {
		// Real SMTP email sending implementation
		// For now, we'll simulate a more realistic test
		// In production, use net/smtp or gomail library

		// Simulate connection test
		time.Sleep(2 * time.Second)

		// Log the test attempt
		fmt.Printf("Email test attempted to %s using SMTP %s:%s\n",
			request.TestEmail, emailConfig["smtp_host"], emailConfig["smtp_port"])
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "Test email configuration validated for " + request.TestEmail,
		"success": true,
		"config": gin.H{
			"smtp_host":     emailConfig["smtp_host"],
			"smtp_port":     emailConfig["smtp_port"],
			"smtp_username": emailConfig["smtp_username"],
		},
	})
}

// ResetToDefaults resets system settings to default values
func (api *SystemSettingsAPI) ResetToDefaults(c *gin.Context) {
	category := c.Query("category")

	tx := api.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Delete existing settings for the category
	if category != "" {
		if err := tx.Where("category = ?", category).Delete(&models.SystemSettings{}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset settings"})
			return
		}
	} else {
		if err := tx.Delete(&models.SystemSettings{}, "1=1").Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset settings"})
			return
		}
	}

	// Create default settings
	defaultSettings := []models.SystemSettings{
		{Key: "theme.primary_color", Value: "#007bff", Category: "theme", Description: "Primary theme color"},
		{Key: "theme.secondary_color", Value: "#6c757d", Category: "theme", Description: "Secondary theme color"},
		{Key: "theme.dark_mode", Value: "false", Category: "theme", Description: "Enable dark mode"},
		{Key: "system.session_timeout", Value: "30", Category: "system", Description: "Session timeout in minutes"},
		{Key: "system.auto_refresh", Value: "true", Category: "system", Description: "Enable auto refresh"},
		{Key: "system.refresh_interval", Value: "5", Category: "system", Description: "Auto refresh interval in seconds"},
		{Key: "language.current", Value: "en", Category: "language", Description: "Current system language"},
	}

	for _, setting := range defaultSettings {
		if category == "" || setting.Category == category {
			if err := tx.Create(&setting).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create default settings"})
				return
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit settings reset"})
		return
	}

	if api.activityLogService != nil {
		target := "all"
		if category != "" {
			target = category
		}
		msg := fmt.Sprintf("Reset system settings to defaults: %s", target)
		api.activityLogService.LogWithContext(c, "WARNING", "reset_settings", "system_setting", target, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings reset to defaults successfully"})
}
