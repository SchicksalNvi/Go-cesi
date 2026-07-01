package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"superview/internal/logger"
	"superview/internal/models"
	"superview/internal/supervisor/xmlrpc"
	"superview/internal/utils"

	"go.uber.org/zap"
)

// ProbeTask represents a single IP probe task.
// Implements utils.Task interface.
// Requirements: 3.1, 3.2, 3.3, 3.4, 4.1, 4.2, 4.3, 4.4, 4.5, 4.6
type ProbeTask struct {
	TaskID   uint
	IP       string
	Port     int
	Username string
	Password string
	Timeout  time.Duration

	// Result fields populated after execution
	Status   string
	Version  string
	ErrorMsg string
	Duration time.Duration
}

// ID returns a unique identifier for this probe task.
func (t *ProbeTask) ID() string {
	return fmt.Sprintf("probe-%d-%s:%d", t.TaskID, t.IP, t.Port)
}

// Execute performs the XML-RPC probe to check for a Supervisor instance.
// Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6
func (t *ProbeTask) Execute(ctx context.Context) error {
	startTime := time.Now()
	defer func() {
		t.Duration = time.Since(startTime)
	}()

	// Create XML-RPC client with timeout
	client, err := xmlrpc.NewClient(t.IP, t.Port, t.Username, t.Password)
	if err != nil {
		t.Status = models.ResultStatusError
		t.ErrorMsg = fmt.Sprintf("failed to create client: %v", err)
		return err
	}

	// Create a context with timeout for the probe
	probeCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()

	// Channel to receive result
	resultCh := make(chan error, 1)

	go func() {
		// Call supervisor.getState() to verify Supervisor is running
		result, err := client.Call("supervisor.getState", nil)
		if err != nil {
			resultCh <- err
			return
		}

		// Parse the response to extract version info
		t.Version = extractVersionFromResponse(result)
		resultCh <- nil
	}()

	// Wait for result or timeout
	select {
	case <-probeCtx.Done():
		// Context cancelled or timed out
		t.Status = models.ResultStatusTimeout
		t.ErrorMsg = "connection timeout"
		return probeCtx.Err()

	case err := <-resultCh:
		if err != nil {
			t.categorizeError(err)
			return err
		}
		// Success
		t.Status = models.ResultStatusSuccess
		return nil
	}
}

// categorizeError determines the error type and sets appropriate status.
// Requirements: 4.4, 4.5, 4.6
func (t *ProbeTask) categorizeError(err error) {
	errStr := err.Error()

	// Check for connection refused
	if isConnectionRefused(err) {
		t.Status = models.ResultStatusConnectionRefused
		t.ErrorMsg = "connection refused"
		return
	}

	// Check for timeout
	if isTimeout(err) {
		t.Status = models.ResultStatusTimeout
		t.ErrorMsg = "connection timeout"
		return
	}

	// Check for authentication failure (HTTP 401)
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "Unauthorized") {
		t.Status = models.ResultStatusAuthFailed
		t.ErrorMsg = "authentication failed"
		return
	}

	// Generic error
	t.Status = models.ResultStatusError
	t.ErrorMsg = errStr
}

// isConnectionRefused checks if the error indicates a connection refused.
func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	if strings.Contains(errStr, "connection refused") {
		return true
	}

	// Check for net.OpError with connection refused
	var opErr *net.OpError
	if ok := isNetOpError(err, &opErr); ok {
		if opErr.Op == "dial" {
			return true
		}
	}

	return false
}

// isTimeout checks if the error indicates a timeout.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return true
	}

	// Check for net.Error timeout
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}

	return false
}

// isNetOpError checks if err is a net.OpError and assigns it to target.
func isNetOpError(err error, target **net.OpError) bool {
	if opErr, ok := err.(*net.OpError); ok {
		*target = opErr
		return true
	}
	return false
}

// extractVersionFromResponse parses the XML-RPC response to extract version.
func extractVersionFromResponse(result interface{}) string {
	if result == nil {
		return ""
	}

	// The response is an XML string, try to extract version
	xmlStr, ok := result.(string)
	if !ok {
		return ""
	}

	// Look for statename in the response (indicates successful connection)
	// The actual version might be in a different call, but getState confirms connectivity
	if strings.Contains(xmlStr, "statename") {
		// Extract version if present, otherwise return "unknown"
		// Supervisor getState returns state info, not version directly
		// Version can be obtained from supervisor.getSupervisorVersion()
		return "detected"
	}

	return ""
}

// Scanner handles the network scanning process.
type Scanner struct {
	service *DiscoveryService
	mu      sync.Mutex
}

// NewScanner creates a new Scanner instance.
func NewScanner(service *DiscoveryService) *Scanner {
	return &Scanner{
		service: service,
	}
}

// ScanConfig holds configuration for a scan operation.
type ScanConfig struct {
	TaskID         uint
	CIDR           string
	Port           int
	Username       string
	Password       string
	TimeoutSeconds int
	MaxWorkers     int
}

// StartScan initiates an asynchronous network scan.
// This runs in a goroutine and updates the task status as it progresses.
// Requirements: 3.1, 3.2, 3.3, 3.4, 3.5
func (s *Scanner) StartScan(config *ScanConfig) error {
	// Parse CIDR to get IP list
	cidrRange, err := utils.ParseCIDR(config.CIDR)
	if err != nil {
		return fmt.Errorf("invalid CIDR: %w", err)
	}

	ips := cidrRange.IPs()
	if len(ips) == 0 {
		return fmt.Errorf("no IPs in CIDR range")
	}

	// Apply defaults
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = DefaultTimeoutSeconds * time.Second
	}

	maxWorkers := config.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = DefaultMaxWorkers
	}

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Create worker pool
	poolConfig := &utils.WorkerPoolConfig{
		Workers:      maxWorkers,
		QueueSize:    len(ips),
		ResultBuffer: len(ips),
		TaskTimeout:  timeout + time.Second, // Add buffer for task timeout
	}
	pool := utils.NewWorkerPool(poolConfig)

	// Register scan context for cancellation support
	scanCtx := &ScanContext{
		TaskID: config.TaskID,
		Cancel: cancel,
		Pool:   pool,
	}
	s.service.RegisterScan(config.TaskID, scanCtx)

	// Start the scan in a goroutine
	go s.runScan(ctx, config, ips, timeout, pool)

	return nil
}

// runScan executes the actual scanning process.
func (s *Scanner) runScan(ctx context.Context, config *ScanConfig, ips []string, timeout time.Duration, pool *utils.WorkerPool) {
	taskID := config.TaskID

	// Ensure cleanup on exit
	defer func() {
		pool.Stop()
		s.service.UnregisterScan(taskID)
	}()

	// Update task status to running
	now := time.Now()
	if err := s.updateTaskStarted(taskID, &now); err != nil {
		logger.Error("Failed to update task status to running",
			zap.Uint("task_id", taskID),
			zap.Error(err))
		s.markTaskFailed(taskID, "failed to start scan: "+err.Error())
		return
	}

	logger.Info("Starting network scan",
		zap.Uint("task_id", taskID),
		zap.String("cidr", config.CIDR),
		zap.Int("total_ips", len(ips)),
		zap.Int("workers", pool.Stats().Workers))

	// Create probe tasks for all IPs
	probeTasks := make([]*ProbeTask, len(ips))
	// Index tasks by their ID for O(1) result lookup (avoids O(n²) linear scan).
	taskByID := make(map[string]*ProbeTask, len(ips))
	for i, ip := range ips {
		probeTasks[i] = &ProbeTask{
			TaskID:   taskID,
			IP:       ip,
			Port:     config.Port,
			Username: config.Username,
			Password: config.Password,
			Timeout:  timeout,
		}
		taskByID[probeTasks[i].ID()] = probeTasks[i]
	}

	// Submit all tasks to the worker pool
	for _, task := range probeTasks {
		select {
		case <-ctx.Done():
			logger.Info("Scan cancelled during task submission",
				zap.Uint("task_id", taskID))
			return
		default:
			if err := pool.Submit(task); err != nil {
				logger.Warn("Failed to submit probe task",
					zap.String("task_id", task.ID()),
					zap.Error(err))
			}
		}
	}

	// Collect results
	var scannedIPs int32
	var foundNodes int32
	var failedIPs int32

	// Process results from worker pool
	resultsCh := pool.Results()
	expectedResults := len(ips)
	receivedResults := 0

	// Progress broadcast interval (in-memory broadcast is cheap)
	const progressInterval = 10
	lastBroadcast := 0

	// Batch discovery-result inserts instead of one INSERT per probed IP.
	const resultBatchSize = 100
	pendingResults := make([]*models.DiscoveryResult, 0, resultBatchSize)
	flushResults := func() {
		if len(pendingResults) == 0 {
			return
		}
		if err := s.service.CreateResults(pendingResults); err != nil {
			logger.Error("Failed to batch-create discovery results",
				zap.Uint("task_id", taskID),
				zap.Int("count", len(pendingResults)),
				zap.Error(err))
		}
		pendingResults = pendingResults[:0]
	}
	// Ensure buffered results are persisted even on early return (cancellation).
	defer flushResults()

	// Throttle DB progress writes by time rather than every N results.
	const progressDBInterval = 2 * time.Second
	lastProgressDB := time.Now()

	for receivedResults < expectedResults {
		select {
		case <-ctx.Done():
			logger.Info("Scan cancelled during result collection",
				zap.Uint("task_id", taskID))
			return

		case result, ok := <-resultsCh:
			if !ok {
				// Channel closed, exit
				goto done
			}

			receivedResults++
			atomic.AddInt32(&scannedIPs, 1)

			// O(1) lookup of the corresponding probe task.
			probeTask := taskByID[result.TaskID]
			if probeTask == nil {
				continue
			}

			// Count results
			var discoveredNodeID uint
			if probeTask.Status == models.ResultStatusSuccess {
				atomic.AddInt32(&foundNodes, 1)

				// Register the discovered node (returns its ID to avoid a re-query)
				discoveredNodeID = s.registerDiscoveredNode(ctx, taskID, probeTask, config.Username, config.Password)

				// Broadcast node discovered event
				s.broadcastNodeDiscovered(taskID, probeTask)
			} else {
				atomic.AddInt32(&failedIPs, 1)
			}

			// Buffer discovery result for batch insert
			pendingResults = append(pendingResults, s.buildDiscoveryResult(taskID, probeTask, discoveredNodeID))
			if len(pendingResults) >= resultBatchSize {
				flushResults()
			}

			// Broadcast progress periodically
			currentScanned := int(atomic.LoadInt32(&scannedIPs))
			if currentScanned-lastBroadcast >= progressInterval || currentScanned == len(ips) {
				s.broadcastProgress(taskID, currentScanned, len(ips),
					int(atomic.LoadInt32(&foundNodes)),
					int(atomic.LoadInt32(&failedIPs)))
				lastBroadcast = currentScanned

				// Update task progress in database, throttled by time.
				if now := time.Now(); now.Sub(lastProgressDB) >= progressDBInterval || currentScanned == len(ips) {
					s.service.UpdateTaskProgress(taskID,
						currentScanned,
						int(atomic.LoadInt32(&foundNodes)),
						int(atomic.LoadInt32(&failedIPs)))
					lastProgressDB = now
				}
			}
		}
	}

done:
	// Persist any remaining buffered results before completing.
	flushResults()

	// Mark task as completed
	completedAt := time.Now()
	s.markTaskCompleted(taskID, &completedAt,
		int(atomic.LoadInt32(&scannedIPs)),
		int(atomic.LoadInt32(&foundNodes)),
		int(atomic.LoadInt32(&failedIPs)))

	// Broadcast completion event
	s.broadcastCompleted(taskID, len(ips),
		int(atomic.LoadInt32(&foundNodes)),
		int(atomic.LoadInt32(&failedIPs)))

	logger.Info("Network scan completed",
		zap.Uint("task_id", taskID),
		zap.Int("scanned", int(atomic.LoadInt32(&scannedIPs))),
		zap.Int("found", int(atomic.LoadInt32(&foundNodes))),
		zap.Int("failed", int(atomic.LoadInt32(&failedIPs))))
}

// updateTaskStarted updates the task status to running.
func (s *Scanner) updateTaskStarted(taskID uint, startedAt *time.Time) error {
	task, err := s.service.GetTask(taskID)
	if err != nil {
		return err
	}

	task.Status = models.DiscoveryStatusRunning
	task.StartedAt = startedAt

	return s.service.GetRepository().UpdateTask(task)
}

// markTaskCompleted marks the task as completed with final statistics.
// Requirements: 8.1 (completion logging)
func (s *Scanner) markTaskCompleted(taskID uint, completedAt *time.Time, scanned, found, failed int) {
	task, err := s.service.GetTask(taskID)
	if err != nil {
		logger.Error("Failed to get task for completion",
			zap.Uint("task_id", taskID),
			zap.Error(err))
		return
	}

	task.Status = models.DiscoveryStatusCompleted
	task.CompletedAt = completedAt
	task.ScannedIPs = scanned
	task.FoundNodes = found
	task.FailedIPs = failed

	if err := s.service.GetRepository().UpdateTask(task); err != nil {
		logger.Error("Failed to mark task as completed",
			zap.Uint("task_id", taskID),
			zap.Error(err))
	}

	// Log task completion
	if activityLog := s.service.GetActivityLogService(); activityLog != nil {
		message := fmt.Sprintf("Discovery task %d completed for CIDR %s (scanned %d IPs, found %d nodes)",
			taskID, task.CIDR, scanned, found)
		activityLog.LogSystemEvent("INFO", "discovery_completed", "discovery", fmt.Sprintf("task-%d", taskID), message, nil)
	}
}

// markTaskFailed marks the task as failed with an error message.
// Requirements: 8.4 (failure logging)
func (s *Scanner) markTaskFailed(taskID uint, errorMsg string) {
	task, err := s.service.GetTask(taskID)
	if err != nil {
		logger.Error("Failed to get task for failure marking",
			zap.Uint("task_id", taskID),
			zap.Error(err))
		return
	}

	task.Status = models.DiscoveryStatusFailed
	task.ErrorMsg = errorMsg
	now := time.Now()
	task.CompletedAt = &now

	if err := s.service.GetRepository().UpdateTask(task); err != nil {
		logger.Error("Failed to mark task as failed",
			zap.Uint("task_id", taskID),
			zap.Error(err))
	}

	// Log task failure - Requirement 8.4
	if activityLog := s.service.GetActivityLogService(); activityLog != nil {
		message := fmt.Sprintf("Discovery task %d failed for CIDR %s: %s",
			taskID, task.CIDR, errorMsg)
		activityLog.LogSystemEvent("ERROR", "discovery_failed", "discovery", fmt.Sprintf("task-%d", taskID), message, nil)
	}
}

// registerDiscoveredNode creates a new node record for a discovered Supervisor.
// Returns the created node's ID (0 if the node already existed or creation failed),
// allowing the caller to avoid re-querying by name.
// Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 8.2
func (s *Scanner) registerDiscoveredNode(ctx context.Context, taskID uint, probe *ProbeTask, username, password string) uint {
	nodeRepo := s.service.GetNodeRepository()

	// Check if node already exists by host:port (Requirement 5.3)
	exists, err := nodeRepo.ExistsByHostPort(probe.IP, probe.Port)
	if err != nil {
		logger.Error("Failed to check if node exists by host:port",
			zap.String("host", probe.IP),
			zap.Int("port", probe.Port),
			zap.Error(err))
		return 0
	}

	if exists {
		logger.Debug("Node with same host:port already exists, marking as duplicate",
			zap.String("host", probe.IP),
			zap.Int("port", probe.Port))
		return 0
	}

	// Generate node name from IP (Requirement 5.2)
	nodeName := generateNodeName(probe.IP)

	// Create new node with status "discovered" (Requirements 5.1, 5.4, 5.5)
	node := &models.Node{
		Name:     nodeName,
		Host:     probe.IP,
		Port:     probe.Port,
		Username: username,
		Password: password,
		Status:   "discovered",
	}

	if err := nodeRepo.Create(node); err != nil {
		logger.Error("Failed to create discovered node",
			zap.String("node_name", nodeName),
			zap.Error(err))
		return 0
	}

	logger.Info("Discovered node registered in database",
		zap.Uint("task_id", taskID),
		zap.String("node_name", nodeName),
		zap.String("host", probe.IP),
		zap.Int("port", probe.Port))

	// Add node to SupervisorService memory
	supervisorSvc := s.service.GetSupervisorService()
	if supervisorSvc != nil {
		if err := supervisorSvc.AddNode(nodeName, "discovered", probe.IP, probe.Port, username, password); err != nil {
			logger.Warn("Failed to add discovered node to supervisor service",
				zap.String("node_name", nodeName),
				zap.Error(err))
			// Not fatal - node is in database, can be loaded on restart
		} else {
			logger.Info("Discovered node loaded into supervisor service",
				zap.String("node_name", nodeName))
		}
	}

	// Log node registration - Requirement 8.2
	if activityLog := s.service.GetActivityLogService(); activityLog != nil {
		message := fmt.Sprintf("Node %s discovered and registered at %s:%d (task %d)",
			nodeName, probe.IP, probe.Port, taskID)
		activityLog.LogSystemEvent("INFO", "node_discovered", "discovery", nodeName, message, nil)
	}

	return node.ID
}

// generateNodeName creates a node name from an IP address.
// Format: node-{ip-with-dashes} (e.g., node-192-168-1-100)
func generateNodeName(ip string) string {
	return "node-" + strings.ReplaceAll(ip, ".", "-")
}

// buildDiscoveryResult builds a discovery result record for batch insertion.
// nodeID, when non-zero, is the just-created node's ID (avoids a re-query);
// otherwise it falls back to looking the node up by name.
func (s *Scanner) buildDiscoveryResult(taskID uint, probe *ProbeTask, nodeID uint) *models.DiscoveryResult {
	result := &models.DiscoveryResult{
		TaskID:   taskID,
		IP:       probe.IP,
		Port:     probe.Port,
		Status:   probe.Status,
		Version:  probe.Version,
		ErrorMsg: probe.ErrorMsg,
		Duration: probe.Duration.Milliseconds(),
	}

	// If successful, record the node name and ID
	if probe.Status == models.ResultStatusSuccess {
		nodeName := generateNodeName(probe.IP)
		result.NodeName = nodeName

		if nodeID != 0 {
			result.NodeID = &nodeID
		} else {
			// Node already existed: look it up by name to set NodeID
			nodeRepo := s.service.GetNodeRepository()
			node, err := nodeRepo.GetByName(nodeName)
			if err == nil && node != nil {
				result.NodeID = &node.ID
			}
		}
	}

	return result
}

// WebSocket event types for discovery
const (
	EventTypeDiscoveryProgress  = "discovery_progress"
	EventTypeNodeDiscovered     = "node_discovered"
	EventTypeDiscoveryCompleted = "discovery_completed"
)

// DiscoveryProgressEvent represents a progress update event.
type DiscoveryProgressEvent struct {
	TaskID     uint    `json:"task_id"`
	ScannedIPs int     `json:"scanned_ips"`
	TotalIPs   int     `json:"total_ips"`
	FoundNodes int     `json:"found_nodes"`
	FailedIPs  int     `json:"failed_ips"`
	Percent    float64 `json:"percent"`
}

// NodeDiscoveredEvent represents a node discovery event.
type NodeDiscoveredEvent struct {
	TaskID   uint   `json:"task_id"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	NodeName string `json:"node_name"`
	Version  string `json:"version"`
}

// DiscoveryCompletedEvent represents a scan completion event.
type DiscoveryCompletedEvent struct {
	TaskID          uint   `json:"task_id"`
	Status          string `json:"status"`
	TotalIPs        int    `json:"total_ips"`
	FoundNodes      int    `json:"found_nodes"`
	DurationSeconds int64  `json:"duration_seconds"`
}

// broadcastProgress sends a progress update via WebSocket.
// Requirements: 6.1
func (s *Scanner) broadcastProgress(taskID uint, scanned, total, found, failed int) {
	hub := s.service.GetHub()
	if hub == nil {
		return
	}

	percent := float64(0)
	if total > 0 {
		percent = float64(scanned) / float64(total) * 100
	}

	event := struct {
		Type string                 `json:"type"`
		Data DiscoveryProgressEvent `json:"data"`
	}{
		Type: EventTypeDiscoveryProgress,
		Data: DiscoveryProgressEvent{
			TaskID:     taskID,
			ScannedIPs: scanned,
			TotalIPs:   total,
			FoundNodes: found,
			FailedIPs:  failed,
			Percent:    percent,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		logger.Error("Failed to marshal progress event",
			zap.Uint("task_id", taskID),
			zap.Error(err))
		return
	}

	hub.Broadcast(data)
}

// broadcastNodeDiscovered sends a node discovered event via WebSocket.
// Requirements: 6.2
func (s *Scanner) broadcastNodeDiscovered(taskID uint, probe *ProbeTask) {
	hub := s.service.GetHub()
	if hub == nil {
		return
	}

	event := struct {
		Type string              `json:"type"`
		Data NodeDiscoveredEvent `json:"data"`
	}{
		Type: EventTypeNodeDiscovered,
		Data: NodeDiscoveredEvent{
			TaskID:   taskID,
			IP:       probe.IP,
			Port:     probe.Port,
			NodeName: generateNodeName(probe.IP),
			Version:  probe.Version,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		logger.Error("Failed to marshal node discovered event",
			zap.Uint("task_id", taskID),
			zap.Error(err))
		return
	}

	hub.Broadcast(data)
}

// broadcastCompleted sends a scan completion event via WebSocket.
// Requirements: 6.3
func (s *Scanner) broadcastCompleted(taskID uint, total, found, failed int) {
	hub := s.service.GetHub()
	if hub == nil {
		return
	}

	// Get task to calculate duration
	task, err := s.service.GetTask(taskID)
	var durationSeconds int64
	if err == nil && task != nil && task.StartedAt != nil && task.CompletedAt != nil {
		durationSeconds = int64(task.CompletedAt.Sub(*task.StartedAt).Seconds())
	}

	event := struct {
		Type string                  `json:"type"`
		Data DiscoveryCompletedEvent `json:"data"`
	}{
		Type: EventTypeDiscoveryCompleted,
		Data: DiscoveryCompletedEvent{
			TaskID:          taskID,
			Status:          models.DiscoveryStatusCompleted,
			TotalIPs:        total,
			FoundNodes:      found,
			DurationSeconds: durationSeconds,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		logger.Error("Failed to marshal completion event",
			zap.Uint("task_id", taskID),
			zap.Error(err))
		return
	}

	hub.Broadcast(data)
}
