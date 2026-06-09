package api

import (
	"fmt"
	"net/http"
	"strconv"

	"superview/internal/errors"
	"superview/internal/models"
	"superview/internal/services"
	"superview/internal/utils"

	"github.com/gin-gonic/gin"
)

// DiscoveryAPI handles HTTP endpoints for node discovery operations.
// Requirements: 2.1, 2.4, 6.4, 7.3, 8.1, 8.2, 8.3, 8.4
type DiscoveryAPI struct {
	service            *services.DiscoveryService
	activityLogService *services.ActivityLogService
}

// NewDiscoveryAPI creates a new DiscoveryAPI instance.
func NewDiscoveryAPI(service *services.DiscoveryService, activityLogService *services.ActivityLogService) *DiscoveryAPI {
	return &DiscoveryAPI{
		service:            service,
		activityLogService: activityLogService,
	}
}

func (api *DiscoveryAPI) authorizeDiscoveryAccess(c *gin.Context) (*models.User, bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		return nil, false
	}

	if user.IsSuperAdmin() || user.HasPermission(models.PermissionSystemManage) {
		return user, true
	}

	handleForbidden(c, "Discovery access is forbidden")
	return nil, false
}

// StartDiscoveryRequest represents the request body for starting a discovery task.
type StartDiscoveryRequest struct {
	CIDR           string `json:"cidr" binding:"required"`
	Port           int    `json:"port" binding:"required,min=1,max=65535"`
	Username       string `json:"username" binding:"required"`
	Password       string `json:"password" binding:"required"`
	TimeoutSeconds int    `json:"timeout_seconds"` // optional, default 3
	MaxWorkers     int    `json:"max_workers"`     // optional, default 50
}

// StartDiscovery handles POST /api/discovery/tasks
// Creates a new discovery task and starts scanning.
// Requirements: 2.1, 8.1, 8.4
func (api *DiscoveryAPI) StartDiscovery(c *gin.Context) {
	if _, ok := api.authorizeDiscoveryAccess(c); !ok {
		return
	}

	var req StartDiscoveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBadRequest(c, err)
		return
	}

	// Get user from JWT context
	userID, ok := validateUserAuthString(c)
	if !ok {
		return
	}

	// Build service request
	serviceReq := &services.DiscoveryRequest{
		CIDR:           req.CIDR,
		Port:           req.Port,
		Username:       req.Username,
		Password:       req.Password,
		TimeoutSeconds: req.TimeoutSeconds,
		MaxWorkers:     req.MaxWorkers,
		CreatedBy:      userID,
	}

	task, err := api.service.StartDiscovery(serviceReq)
	if err != nil {
		// Log discovery failure - Requirements: 8.4
		if api.activityLogService != nil {
			message := fmt.Sprintf("Discovery task failed to start for CIDR %s: %s", req.CIDR, err.Error())
			api.activityLogService.LogWithContext(c, "ERROR", "discovery_failed", "discovery", req.CIDR, message, nil)
		}
		handleAppError(c, err)
		return
	}

	// Log discovery started - Requirements: 8.1
	if api.activityLogService != nil {
		message := fmt.Sprintf("Discovery task started for CIDR %s on port %d (%d IPs to scan)",
			req.CIDR, req.Port, task.TotalIPs)
		api.activityLogService.LogWithContext(c, "INFO", "discovery_started", "discovery", fmt.Sprintf("task-%d", task.ID), message, nil)
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"task":   task,
	})
}

// ListTasks handles GET /api/discovery/tasks
// Returns paginated list of discovery tasks.
// Requirements: 7.3
func (api *DiscoveryAPI) ListTasks(c *gin.Context) {
	if _, ok := api.authorizeDiscoveryAccess(c); !ok {
		return
	}

	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")

	// Validate pagination
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	tasks, total, err := api.service.ListTasks(offset, limit, status)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"tasks":  tasks,
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}

// GetTask handles GET /api/discovery/tasks/:id
// Returns task details with results.
// Requirements: 7.1, 7.2
func (api *DiscoveryAPI) GetTask(c *gin.Context) {
	if _, ok := api.authorizeDiscoveryAccess(c); !ok {
		return
	}

	taskID, ok := parseAndValidateID(c, "id", "task")
	if !ok {
		return
	}

	task, err := api.service.GetTask(taskID)
	if err != nil {
		handleAppError(c, err)
		return
	}

	// Get results for this task
	results, err := api.service.GetTaskResults(taskID)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"task":    task,
		"results": results,
	})
}

// CancelTask handles POST /api/discovery/tasks/:id/cancel
// Cancels a running discovery task.
// Requirements: 2.4, 8.3
func (api *DiscoveryAPI) CancelTask(c *gin.Context) {
	if _, ok := api.authorizeDiscoveryAccess(c); !ok {
		return
	}

	taskID, ok := parseAndValidateID(c, "id", "task")
	if !ok {
		return
	}

	// Get task details before cancellation for logging
	task, err := api.service.GetTask(taskID)
	if err != nil {
		handleAppError(c, err)
		return
	}

	err = api.service.CancelDiscovery(taskID)
	if err != nil {
		handleAppError(c, err)
		return
	}

	// Log task cancellation - Requirements: 8.3
	if api.activityLogService != nil {
		message := fmt.Sprintf("Discovery task %d cancelled for CIDR %s (scanned %d/%d IPs, found %d nodes)",
			taskID, task.CIDR, task.ScannedIPs, task.TotalIPs, task.FoundNodes)
		api.activityLogService.LogWithContext(c, "INFO", "discovery_cancelled", "discovery", fmt.Sprintf("task-%d", taskID), message, nil)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Task cancelled",
	})
}

// DeleteTask handles DELETE /api/discovery/tasks/:id
// Deletes a discovery task and its results.
// Only terminal tasks (completed, cancelled, failed) can be deleted.
func (api *DiscoveryAPI) DeleteTask(c *gin.Context) {
	if _, ok := api.authorizeDiscoveryAccess(c); !ok {
		return
	}

	taskID, ok := parseAndValidateID(c, "id", "task")
	if !ok {
		return
	}

	// Check if task exists first
	task, err := api.service.GetTask(taskID)
	if err != nil {
		handleAppError(c, err)
		return
	}

	// Verify task is in terminal state
	if !task.IsTerminal() {
		handleConflict(c, "discovery_task", "cannot delete task in non-terminal state: "+task.Status)
		return
	}

	err = api.service.DeleteTask(taskID)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Task deleted",
	})
}

// GetTaskProgress handles GET /api/discovery/tasks/:id/progress
// Returns current progress of a discovery task.
// This is a polling endpoint for clients that cannot use WebSocket.
// Requirements: 6.4
func (api *DiscoveryAPI) GetTaskProgress(c *gin.Context) {
	if _, ok := api.authorizeDiscoveryAccess(c); !ok {
		return
	}

	taskID, ok := parseAndValidateID(c, "id", "task")
	if !ok {
		return
	}

	task, err := api.service.GetTask(taskID)
	if err != nil {
		handleAppError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"progress": gin.H{
			"task_id":     task.ID,
			"status":      task.Status,
			"total_ips":   task.TotalIPs,
			"scanned_ips": task.ScannedIPs,
			"found_nodes": task.FoundNodes,
			"failed_ips":  task.FailedIPs,
			"percent":     task.Progress(),
		},
	})
}

// ValidateCIDR handles POST /api/discovery/validate-cidr
// Validates a CIDR string and returns the IP count.
// This is a helper endpoint for frontend validation.
// Requirements: 1.1, 1.2, 1.3
func (api *DiscoveryAPI) ValidateCIDR(c *gin.Context) {
	if _, ok := api.authorizeDiscoveryAccess(c); !ok {
		return
	}

	var req struct {
		CIDR string `json:"cidr" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		handleBadRequest(c, err)
		return
	}

	// Use the CIDR parser from utils
	cidrRange, err := parseCIDRForValidation(req.CIDR)
	if err != nil {
		appErr := errors.NewValidationError("cidr", err.Error())
		handleAppError(c, appErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"valid":  true,
		"cidr":   req.CIDR,
		"count":  cidrRange.count,
	})
}

// cidrValidationResult holds the result of CIDR validation.
type cidrValidationResult struct {
	count int
}

// parseCIDRDirect validates a CIDR string using the utils package.
func parseCIDRDirect(cidr string) (*cidrValidationResult, error) {
	cidrRange, err := utils.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	return &cidrValidationResult{count: cidrRange.Count()}, nil
}

// parseCIDRForValidation validates a CIDR string.
func parseCIDRForValidation(cidr string) (*cidrValidationResult, error) {
	return parseCIDRDirect(cidr)
}
