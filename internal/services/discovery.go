package services

import (
	"context"
	"sync"

	"superview/internal/errors"
	"superview/internal/logger"
	"superview/internal/models"
	"superview/internal/repository"
	"superview/internal/utils"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DiscoveryRequest represents a request to start a discovery scan.
type DiscoveryRequest struct {
	CIDR           string `json:"cidr"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	TimeoutSeconds int    `json:"timeout_seconds"` // optional, default 3
	MaxWorkers     int    `json:"max_workers"`     // optional, default 50
	CreatedBy      string `json:"created_by"`
}

// Default values for discovery requests
const (
	DefaultTimeoutSeconds = 3
	DefaultMaxWorkers     = 50
	MinPort               = 1
	MaxPort               = 65535
)

// ScanContext holds the context for an active scan.
type ScanContext struct {
	TaskID uint
	Cancel context.CancelFunc
	Pool   *utils.WorkerPool
}

// SupervisorService interface for adding discovered nodes to memory
type SupervisorService interface {
	AddNode(name, environment, host string, port int, username, password string) error
}

// DiscoveryService handles node discovery operations.
type DiscoveryService struct {
	db                 *gorm.DB
	repo               repository.DiscoveryRepository
	nodeRepo           repository.NodeRepository
	hub                WebSocketHub
	supervisorService  SupervisorService
	activityLogService *ActivityLogService
	activeScans        map[uint]*ScanContext
	mu                 sync.RWMutex
}

// NewDiscoveryService creates a new DiscoveryService instance.
func NewDiscoveryService(db *gorm.DB, repo repository.DiscoveryRepository, nodeRepo repository.NodeRepository, hub WebSocketHub, supervisorService SupervisorService) *DiscoveryService {
	return &DiscoveryService{
		db:                 db,
		repo:               repo,
		nodeRepo:           nodeRepo,
		hub:                hub,
		supervisorService:  supervisorService,
		activityLogService: NewActivityLogService(db),
		activeScans:        make(map[uint]*ScanContext),
	}
}

// StartDiscovery validates input, creates a task, and starts the scan.
// Requirements: 2.1, 2.2
func (s *DiscoveryService) StartDiscovery(req *DiscoveryRequest) (*models.DiscoveryTask, error) {
	// Validate CIDR
	cidrRange, err := utils.ParseCIDR(req.CIDR)
	if err != nil {
		logger.Warn("Invalid CIDR provided",
			zap.String("cidr", req.CIDR),
			zap.Error(err))
		return nil, errors.NewValidationError("cidr", err.Error())
	}

	// Validate port
	if req.Port < MinPort || req.Port > MaxPort {
		logger.Warn("Invalid port provided",
			zap.Int("port", req.Port))
		return nil, errors.NewValidationError("port", "port must be between 1 and 65535")
	}

	// Validate username (required for Supervisor auth)
	if req.Username == "" {
		return nil, errors.NewValidationError("username", "username is required")
	}

	// Validate password (required for Supervisor auth)
	if req.Password == "" {
		return nil, errors.NewValidationError("password", "password is required")
	}

	// Validate created_by
	if req.CreatedBy == "" {
		return nil, errors.NewValidationError("created_by", "created_by is required")
	}

	// Apply defaults
	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = DefaultTimeoutSeconds
	}

	maxWorkers := req.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = DefaultMaxWorkers
	}

	// Create task with status "pending"
	task := &models.DiscoveryTask{
		CIDR:      req.CIDR,
		Port:      req.Port,
		Username:  req.Username,
		Status:    models.DiscoveryStatusPending,
		TotalIPs:  cidrRange.Count(),
		CreatedBy: req.CreatedBy,
	}

	// Persist to database
	if err := s.repo.CreateTask(task); err != nil {
		logger.Error("Failed to create discovery task",
			zap.String("cidr", req.CIDR),
			zap.Error(err))
		return nil, err
	}

	logger.Info("Discovery task created",
		zap.Uint("task_id", task.ID),
		zap.String("cidr", req.CIDR),
		zap.Int("port", req.Port),
		zap.Int("total_ips", task.TotalIPs),
		zap.String("created_by", req.CreatedBy))

	// Start the scan asynchronously
	scanner := NewScanner(s)
	scanConfig := &ScanConfig{
		TaskID:         task.ID,
		CIDR:           req.CIDR,
		Port:           req.Port,
		Username:       req.Username,
		Password:       req.Password,
		TimeoutSeconds: timeoutSeconds,
		MaxWorkers:     maxWorkers,
	}

	if err := scanner.StartScan(scanConfig); err != nil {
		// Mark task as failed if scan couldn't start
		task.Status = models.DiscoveryStatusFailed
		task.ErrorMsg = "failed to start scan: " + err.Error()
		s.repo.UpdateTask(task)
		return nil, err
	}

	return task, nil
}

// CancelDiscovery stops a running scan and updates its status.
// Requirements: 2.4
func (s *DiscoveryService) CancelDiscovery(taskID uint) error {
	// Get the task first
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return err
	}

	// Check if task is in a cancellable state
	if task.IsTerminal() {
		return errors.NewConflictError("discovery_task",
			"task is already in terminal state: "+task.Status)
	}

	// Stop the worker pool if scan is active
	s.mu.Lock()
	scanCtx, exists := s.activeScans[taskID]
	if exists {
		// Cancel the context to signal workers to stop
		scanCtx.Cancel()
		// Stop the worker pool
		if scanCtx.Pool != nil {
			scanCtx.Pool.Stop()
		}
		delete(s.activeScans, taskID)
	}
	s.mu.Unlock()

	// Update task status to cancelled
	task.Status = models.DiscoveryStatusCancelled
	if err := s.repo.UpdateTask(task); err != nil {
		logger.Error("Failed to update task status to cancelled",
			zap.Uint("task_id", taskID),
			zap.Error(err))
		return err
	}

	logger.Info("Discovery task cancelled",
		zap.Uint("task_id", taskID),
		zap.Int("scanned_ips", task.ScannedIPs),
		zap.Int("total_ips", task.TotalIPs))

	return nil
}

// GetTask retrieves a discovery task by ID.
// Requirements: 7.1, 7.2
func (s *DiscoveryService) GetTask(taskID uint) (*models.DiscoveryTask, error) {
	return s.repo.GetTask(taskID)
}

// ListTasks retrieves discovery tasks with pagination.
// Requirements: 7.3
func (s *DiscoveryService) ListTasks(offset, limit int, status string) ([]*models.DiscoveryTask, int64, error) {
	return s.repo.ListTasks(offset, limit, status)
}

// GetTaskResults retrieves all results for a given task.
// Requirements: 7.2
func (s *DiscoveryService) GetTaskResults(taskID uint) ([]*models.DiscoveryResult, error) {
	// Verify task exists
	_, err := s.repo.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	return s.repo.GetResultsByTaskID(taskID)
}

// DeleteTask deletes a discovery task and its results.
// Only terminal tasks can be deleted.
func (s *DiscoveryService) DeleteTask(taskID uint) error {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return err
	}

	// Only allow deletion of terminal tasks
	if !task.IsTerminal() {
		return errors.NewConflictError("discovery_task",
			"cannot delete task in non-terminal state: "+task.Status)
	}

	return s.repo.DeleteTask(taskID)
}

// IsTaskRunning checks if a task is currently being scanned.
func (s *DiscoveryService) IsTaskRunning(taskID uint) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.activeScans[taskID]
	return exists
}

// RegisterScan registers an active scan context.
// This is called by the scanner when starting a scan.
func (s *DiscoveryService) RegisterScan(taskID uint, ctx *ScanContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeScans[taskID] = ctx
}

// UnregisterScan removes a scan context when scanning completes.
// This is called by the scanner when a scan finishes.
func (s *DiscoveryService) UnregisterScan(taskID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.activeScans, taskID)
}

// UpdateTaskProgress updates the progress counters for a task.
// This is called by the scanner during scanning.
// Requirements: 2.3
func (s *DiscoveryService) UpdateTaskProgress(taskID uint, scannedIPs, foundNodes, failedIPs int) error {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return err
	}

	task.ScannedIPs = scannedIPs
	task.FoundNodes = foundNodes
	task.FailedIPs = failedIPs

	return s.repo.UpdateTask(task)
}

// UpdateTaskStatus updates the status of a task.
// This is called by the scanner to transition task states.
// Requirements: 2.5, 2.6
func (s *DiscoveryService) UpdateTaskStatus(taskID uint, status string, errorMsg string) error {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return err
	}

	task.Status = status
	if errorMsg != "" {
		task.ErrorMsg = errorMsg
	}

	return s.repo.UpdateTask(task)
}

// CreateResult creates a discovery result record.
// This is called by the scanner for each probed IP.
func (s *DiscoveryService) CreateResult(result *models.DiscoveryResult) error {
	return s.repo.CreateResult(result)
}

// CreateResults batch-creates discovery result records.
func (s *DiscoveryService) CreateResults(results []*models.DiscoveryResult) error {
	return s.repo.CreateResults(results)
}

// GetRepository returns the discovery repository.
// This allows the scanner to access the repository directly if needed.
func (s *DiscoveryService) GetRepository() repository.DiscoveryRepository {
	return s.repo
}

// GetNodeRepository returns the node repository.
// This allows the scanner to check for existing nodes.
func (s *DiscoveryService) GetNodeRepository() repository.NodeRepository {
	return s.nodeRepo
}

// GetHub returns the WebSocket hub for broadcasting events.
func (s *DiscoveryService) GetHub() WebSocketHub {
	return s.hub
}

// GetActivityLogService returns the activity log service for logging events.
// Requirements: 8.1, 8.2, 8.3, 8.4
func (s *DiscoveryService) GetActivityLogService() *ActivityLogService {
	return s.activityLogService
}

// GetSupervisorService returns the supervisor service for adding discovered nodes.
func (s *DiscoveryService) GetSupervisorService() SupervisorService {
	return s.supervisorService
}
