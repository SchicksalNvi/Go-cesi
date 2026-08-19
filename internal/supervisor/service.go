package supervisor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"superview/internal/config"
	"superview/internal/errors"
	"superview/internal/logger"
	"superview/internal/supervisor/xmlrpc"
	"go.uber.org/zap"
)

// ActivityLogger 活动日志记录器接口
type ActivityLogger interface {
	LogSystemEvent(level, action, resource, target, message string, extraInfo interface{}) error
}

type SupervisorService struct {
	nodes              map[string]*Node
	mu                 sync.RWMutex
	stopChan           chan struct{}
	wg                 sync.WaitGroup
	shutdown           int32  // atomic flag
	activityLogger     ActivityLogger
	processStates      map[string]map[string]int // nodeName -> processName -> state
	nodeStates         map[string]bool            // nodeName -> isConnected
	statesMu           sync.RWMutex
	
	// Connection management - configurable
	connectionSemaphore chan struct{} // Configurable concurrent connections limit
	config             *config.PerformanceConfig
	
	// Timeout management
	timeoutManager     *TimeoutManager
}

func NewSupervisorService() *SupervisorService {
	return &SupervisorService{
		nodes:               make(map[string]*Node),
		stopChan:            make(chan struct{}),
		processStates:       make(map[string]map[string]int),
		nodeStates:          make(map[string]bool),
		connectionSemaphore: make(chan struct{}, 100), // Default fallback
		timeoutManager:      NewTimeoutManager(nil),   // Use default config
	}
}

// NewSupervisorServiceWithConfig creates service with performance config
func NewSupervisorServiceWithConfig(perfConfig *config.PerformanceConfig) *SupervisorService {
	maxConn := perfConfig.MaxConcurrentConnections
	if maxConn <= 0 {
		maxConn = 100 // Fallback
	}
	
	return &SupervisorService{
		nodes:               make(map[string]*Node),
		stopChan:            make(chan struct{}),
		processStates:       make(map[string]map[string]int),
		nodeStates:          make(map[string]bool),
		connectionSemaphore: make(chan struct{}, maxConn),
		config:              perfConfig,
		timeoutManager:      NewTimeoutManager(nil), // Use default config
	}
}

// SetActivityLogger 设置活动日志记录器
func (s *SupervisorService) SetActivityLogger(logger ActivityLogger) {
	s.activityLogger = logger
}

// StartMonitoring 启动状态监控，返回停止通道
func (s *SupervisorService) StartMonitoring(interval time.Duration) chan struct{} {
	stopChan := make(chan struct{})
	
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopChan:
				return
			case <-stopChan:
				logger.Debug("Stopping monitoring goroutine")
				return
			case <-ticker.C:
				s.monitorStates()
			}
		}
	}()
	
	return stopChan
}

// StopMonitoring 停止状态监控
func (s *SupervisorService) StopMonitoring(stopChan chan struct{}) {
	if stopChan != nil {
		close(stopChan)
	}
}

// monitorStates 监控节点和进程状态变化
func (s *SupervisorService) monitorStates() {
	s.mu.RLock()
	nodes := make([]*Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, node)
	}
	s.mu.RUnlock()

	for _, node := range nodes {
		// 检查节点连接状态
		s.checkNodeConnectionState(node)

		// 检查进程状态
		if connected, _ := node.GetConnectionStatus(); connected {
			s.checkProcessStates(node)
		}
	}
}

// checkNodeConnectionState 检查节点连接状态变化
func (s *SupervisorService) checkNodeConnectionState(node *Node) {
	s.statesMu.RLock()
	previousState, exists := s.nodeStates[node.Name]
	s.statesMu.RUnlock()

	currentState := s.checkNodeStatus(node)

	// 状态发生变化
	if !exists || previousState != currentState {
		s.statesMu.Lock()
		s.nodeStates[node.Name] = currentState
		s.statesMu.Unlock()

		// 记录日志
		if s.activityLogger != nil {
			if currentState {
				// 节点恢复连接
				message := fmt.Sprintf("Node %s reconnected at %s:%d", node.Name, node.Host, node.Port)
				s.activityLogger.LogSystemEvent("INFO", "node_connected", "node", node.Name, message, nil)
			} else if exists {
				// 节点断开连接（仅在之前存在状态时记录）
				message := fmt.Sprintf("Node %s disconnected at %s:%d", node.Name, node.Host, node.Port)
				s.activityLogger.LogSystemEvent("WARNING", "node_disconnected", "node", node.Name, message, nil)
			}
		}
	}
}

// checkProcessStates 检查进程状态变化
func (s *SupervisorService) checkProcessStates(node *Node) {
	if err := node.RefreshProcesses(); err != nil {
		return
	}

	s.statesMu.Lock()
	defer s.statesMu.Unlock()

	// 初始化节点的进程状态映射
	if s.processStates[node.Name] == nil {
		s.processStates[node.Name] = make(map[string]int)
	}

	// 在锁内读取进程列表副本,避免与 RefreshProcesses 写入产生竞态
	node.mu.RLock()
	processes := make([]Process, len(node.Processes))
	copy(processes, node.Processes)
	node.mu.RUnlock()

	for _, process := range processes {
		processKey := process.Name
		previousState, exists := s.processStates[node.Name][processKey]
		currentState := process.State

		// 状态发生变化
		if !exists {
			// 首次发现进程，记录当前状态
			s.processStates[node.Name][processKey] = currentState
		} else if previousState != currentState {
			// 状态变化，记录日志
			s.processStates[node.Name][processKey] = currentState

			if s.activityLogger != nil {
				target := fmt.Sprintf("%s:%s", node.Name, process.Name)
				
				// 根据状态变化记录不同的日志
				if currentState == 20 && previousState != 20 {
					// 进程启动
					message := fmt.Sprintf("Process %s started on node %s (state: %s -> %s)", 
						process.Name, node.Name, getStateName(previousState), process.StateString)
					s.activityLogger.LogSystemEvent("INFO", "process_started", "process", target, message, nil)
				} else if currentState == 0 && previousState == 20 {
					// 进程停止
					message := fmt.Sprintf("Process %s stopped on node %s (state: %s -> %s)", 
						process.Name, node.Name, getStateName(previousState), process.StateString)
					s.activityLogger.LogSystemEvent("WARNING", "process_stopped", "process", target, message, nil)
				} else if currentState == 100 || currentState == 200 {
					// 进程异常退出
					message := fmt.Sprintf("Process %s exited abnormally on node %s (state: %s -> %s, exit: %d)", 
						process.Name, node.Name, getStateName(previousState), process.StateString, process.ExitStatus)
					s.activityLogger.LogSystemEvent("ERROR", "process_failed", "process", target, message, nil)
				} else {
					// 其他状态变化
					message := fmt.Sprintf("Process %s state changed on node %s (state: %s -> %s)", 
						process.Name, node.Name, getStateName(previousState), process.StateString)
					s.activityLogger.LogSystemEvent("INFO", "process_state_changed", "process", target, message, nil)
				}
			}
		}
	}
}

// getStateName 获取状态名称
func getStateName(state int) string {
	switch state {
	case 0:
		return "STOPPED"
	case 10:
		return "STARTING"
	case 20:
		return "RUNNING"
	case 30:
		return "BACKOFF"
	case 40:
		return "STOPPING"
	case 100:
		return "EXITED"
	case 200:
		return "FATAL"
	case 1000:
		return "UNKNOWN"
	default:
		return fmt.Sprintf("STATE_%d", state)
	}
}

func (s *SupervisorService) AddNode(name, environment, host string, port int, username, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return errors.NewInternalError("service is shutting down", nil)
	}
	
	if _, exists := s.nodes[name]; exists {
		return errors.NewConflictError("node", "node "+name+" already exists")
	}

	logger.Info("Creating node",
		zap.String("name", name),
		zap.String("host", host),
		zap.Int("port", port))
	node, err := NewNode(name, environment, host, port, username, password)
	if err != nil {
		logger.Error("Failed to create node",
			zap.String("name", name),
			zap.Error(err))
		return err
	}

	// 尝试连接
	logger.Info("Attempting to connect to node",
		zap.String("name", name),
		zap.String("host", host),
		zap.Int("port", port))
	if err := node.Connect(); err != nil {
		// 连接失败时仍然添加节点，但标记为未连接
		logger.Warn("Failed to connect to node",
			zap.String("name", name),
			zap.Error(err))
		node.SetConnected(false)
	} else {
		logger.Info("Successfully connected to node",
			zap.String("name", name))
		node.SetConnectionStatus(true, time.Now())
		
		// 连接成功后立即刷新进程列表
		if err := node.RefreshProcesses(); err != nil {
			logger.Warn("Failed to refresh processes on initial connection",
				zap.String("name", name),
				zap.Error(err))
		}
	}

	s.nodes[name] = node
	connected, _ := node.GetConnectionStatus()
	logger.Info("Node added to service",
		zap.String("name", name),
		zap.Bool("connected", connected))
	return nil
}

func (s *SupervisorService) GetNode(name string) (*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return nil, errors.NewInternalError("service is shutting down", nil)
	}
	
	node, exists := s.nodes[name]
	if !exists {
		return nil, errors.NewNotFoundError("node", name)
	}
	return node, nil
}

// UpdateNodeInfo updates a node's name and/or environment in memory.
// If the name changes, the node is re-keyed in the map.
func (s *SupervisorService) UpdateNodeInfo(oldName, newName, environment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if atomic.LoadInt32(&s.shutdown) != 0 {
		return errors.NewInternalError("service is shutting down", nil)
	}

	node, exists := s.nodes[oldName]
	if !exists {
		return errors.NewNotFoundError("node", oldName)
	}

	if newName != oldName {
		if _, dup := s.nodes[newName]; dup {
			return errors.NewConflictError("node", "node "+newName+" already exists")
		}
		delete(s.nodes, oldName)
		node.Name = newName
		s.nodes[newName] = node
	}

	node.Environment = environment
	return nil
}

func (s *SupervisorService) GetAllNodes() []*Node {
	// 检查是否已关闭
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return nil
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// 再次检查关闭状态（双重检查）
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return nil
	}
	
	// 简单返回节点列表，不做异步连接检查
	allNodes := make([]*Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		allNodes = append(allNodes, node)
	}
	
	return allNodes
}

func (s *SupervisorService) checkNodeStatus(node *Node) bool {
	return node.Connect() == nil
}

func (s *SupervisorService) GetNodeProcesses(nodeName string) ([]Process, error) {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return nil, err
	}

	if err := node.RefreshProcesses(); err != nil {
		return nil, err
	}

	// 安全地获取进程列表副本
	node.mu.RLock()
	processes := make([]Process, len(node.Processes))
	copy(processes, node.Processes)
	node.mu.RUnlock()

	return processes, nil
}

func (s *SupervisorService) StartProcess(nodeName, processName string) error {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return err
	}
	
	// 使用超时管理器执行操作
	ctx := context.Background()
	operationName := fmt.Sprintf("start_process_%s_%s", nodeName, processName)
	
	return s.timeoutManager.ExecuteWithRetry(ctx, operationName, func(ctx context.Context) error {
		return node.StartProcess(processName)
	})
}

func (s *SupervisorService) StopProcess(nodeName, processName string) error {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return err
	}
	
	// 使用超时管理器执行操作
	ctx := context.Background()
	operationName := fmt.Sprintf("stop_process_%s_%s", nodeName, processName)
	
	return s.timeoutManager.ExecuteWithRetry(ctx, operationName, func(ctx context.Context) error {
		return node.StopProcess(processName)
	})
}

func (s *SupervisorService) GetProcessLogs(nodeName, processName string) (map[string][]string, error) {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return nil, err
	}
	return node.GetProcessLogs(processName)
}

// ReadProcessLogs 按偏移读取进程历史日志(分页) - 符合官方 API 规范
func (s *SupervisorService) ReadProcessLogs(nodeName, processName, logType string, offset, length int) (string, int, bool, error) {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return "", 0, false, err
	}
	return node.ReadProcessLogs(processName, logType, offset, length)
}

// ClearProcessLogs 清空并重开指定进程的 stdout/stderr 日志 - 符合官方 API 规范
func (s *SupervisorService) ClearProcessLogs(nodeName, processName string) error {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return err
	}
	ctx := context.Background()
	operationName := fmt.Sprintf("clear_logs_%s_%s", nodeName, processName)
	return s.timeoutManager.ExecuteWithRetry(ctx, operationName, func(ctx context.Context) error {
		return node.ClearProcessLogs(processName)
	})
}

// GetAllConfigInfo 获取节点所有进程配置信息 - 符合官方 API 规范
func (s *SupervisorService) GetAllConfigInfo(nodeName string) ([]xmlrpc.ProcessConfigInfo, error) {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return nil, err
	}
	return node.GetAllConfigInfo()
}

// ReloadConfig 重载节点 supervisord 配置 - 符合官方 API 规范
func (s *SupervisorService) ReloadConfig(nodeName string) (*xmlrpc.ReloadResult, error) {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	operationName := fmt.Sprintf("reload_config_%s", nodeName)
	var res *xmlrpc.ReloadResult
	err = s.timeoutManager.ExecuteWithRetry(ctx, operationName, func(ctx context.Context) error {
		r, e := node.ReloadConfig()
		if e != nil {
			return e
		}
		res = r
		return nil
	})
	return res, err
}

// StartAllProcesses starts all processes on a specific node with timeout management
func (s *SupervisorService) StartAllProcesses(nodeName string) error {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return err
	}
	
	if err := node.RefreshProcesses(); err != nil {
		return err
	}
	
	// 创建批量操作
	var operations []BatchOperation
	node.mu.RLock()
	for _, process := range node.Processes {
		processName := process.Name // 捕获循环变量
		operations = append(operations, BatchOperation{
			Name: fmt.Sprintf("start_%s_%s", nodeName, processName),
			Execute: func(ctx context.Context) error {
				return node.StartProcess(processName)
			},
		})
	}
	node.mu.RUnlock()
	
	// 执行批量操作
	ctx := context.Background()
	results := s.timeoutManager.ExecuteBatchWithTimeout(ctx, operations)
	
	// 检查结果
	var errors []error
	for _, result := range results {
		if result.Error != nil {
			errors = append(errors, result.Error)
			logger.Error("Failed to start process in batch",
				zap.String("node_name", nodeName),
				zap.Int("operation_index", result.Index),
				zap.Error(result.Error))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("batch start failed with %d errors", len(errors))
	}
	
	return nil
}

// StopAllProcesses stops all processes on a specific node with timeout management
func (s *SupervisorService) StopAllProcesses(nodeName string) error {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return err
	}
	
	if err := node.RefreshProcesses(); err != nil {
		return err
	}
	
	// 创建批量操作
	var operations []BatchOperation
	node.mu.RLock()
	for _, process := range node.Processes {
		processName := process.Name // 捕获循环变量
		operations = append(operations, BatchOperation{
			Name: fmt.Sprintf("stop_%s_%s", nodeName, processName),
			Execute: func(ctx context.Context) error {
				return node.StopProcess(processName)
			},
		})
	}
	node.mu.RUnlock()
	
	// 执行批量操作
	ctx := context.Background()
	results := s.timeoutManager.ExecuteBatchWithTimeout(ctx, operations)
	
	// 检查结果
	var errors []error
	for _, result := range results {
		if result.Error != nil {
			errors = append(errors, result.Error)
			logger.Error("Failed to stop process in batch",
				zap.String("node_name", nodeName),
				zap.Int("operation_index", result.Index),
				zap.Error(result.Error))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("batch stop failed with %d errors", len(errors))
	}
	
	return nil
}

// RestartAllProcesses restarts all processes on a specific node
func (s *SupervisorService) RestartAllProcesses(nodeName string) error {
	node, err := s.GetNode(nodeName)
	if err != nil {
		return err
	}
	
	if err := node.RefreshProcesses(); err != nil {
		return err
	}
	
	var errs []string
	node.mu.RLock()
	processes := make([]Process, len(node.Processes))
	copy(processes, node.Processes)
	node.mu.RUnlock()

	for _, process := range processes {
		if err := node.RestartProcess(process.Name); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", process.Name, err))
			logger.Error("Failed to restart process",
				zap.String("process", process.Name),
				zap.Error(err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to restart %d processes: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

// 移除不再需要的辅助函数

// GetEnvironments 获取所有环境列表
func (s *SupervisorService) GetEnvironments() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	environmentMap := make(map[string][]map[string]interface{})
	
	// 按环境分组节点
	for _, node := range s.nodes {
		isConnected, lastPing := node.GetConnectionStatus()
		nodeInfo := map[string]interface{}{
			"name": node.Name,
			"host": node.Host,
			"port": node.Port,
			"is_connected": isConnected,
			"last_ping": lastPing,
		}
		environmentMap[node.Environment] = append(environmentMap[node.Environment], nodeInfo)
	}
	
	// 转换为环境列表格式
	environments := make([]map[string]interface{}, 0)
	for envName, members := range environmentMap {
		environment := map[string]interface{}{
			"name": envName,
			"members": members,
		}
		environments = append(environments, environment)
	}
	
	return environments
}

// GetEnvironmentDetails 获取特定环境的详细信息
func (s *SupervisorService) GetEnvironmentDetails(environmentName string) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	members := make([]map[string]interface{}, 0)
	
	for _, node := range s.nodes {
		if node.Environment == environmentName {
			isConnected, lastPing := node.GetConnectionStatus()
			processCount := node.GetProcessCount()
			nodeInfo := map[string]interface{}{
				"name": node.Name,
				"host": node.Host,
				"port": node.Port,
				"is_connected": isConnected,
				"last_ping": lastPing,
				"processes": processCount,
			}
			members = append(members, nodeInfo)
		}
	}
	
	if len(members) == 0 {
		return nil
	}
	
	return map[string]interface{}{
		"name": environmentName,
		"members": members,
	}
}

// GetGroups 获取所有进程分组
func (s *SupervisorService) GetGroups() []map[string]interface{} {
	// 收集已连接的节点列表,避免在持有锁时进行网络操作
	s.mu.RLock()
	var nodes []*Node
	for _, node := range s.nodes {
		isConnected, _ := node.GetConnectionStatus()
		if !isConnected {
			continue
		}
		nodes = append(nodes, node)
	}
	s.mu.RUnlock()

	groupMap := make(map[string]map[string][]map[string]interface{})

	// 在锁外刷新进程信息并组织分组
	for _, node := range nodes {
		// 刷新进程信息
		node.RefreshProcesses()
		
		// 获取进程列表的副本以避免竞态条件
		processes := node.SerializeProcesses()
		
		for _, processData := range processes {
			processName, _ := processData["name"].(string)
			groupName, _ := processData["group"].(string)
			state, _ := processData["state"].(int)
			pid, _ := processData["pid"].(int)
			uptime, _ := processData["uptime"].(float64)
			
			if groupName == "" {
				groupName = "default"
			}
			
			if groupMap[groupName] == nil {
				groupMap[groupName] = make(map[string][]map[string]interface{})
			}
			
			processInfo := map[string]interface{}{
				"name": processName,
				"state": state,
				"node": node.Name,
				"pid": pid,
				"uptime": time.Duration(uptime * float64(time.Second)),
			}
			
			groupMap[groupName][node.Environment] = append(groupMap[groupName][node.Environment], processInfo)
		}
	}
	
	// 转换为分组列表格式
	groups := make([]map[string]interface{}, 0)
	for groupName, environments := range groupMap {
		group := map[string]interface{}{
			"name": groupName,
			"environments": make([]map[string]interface{}, 0),
		}
		
		for envName, processes := range environments {
			environment := map[string]interface{}{
				"name": envName,
				"processes": processes,
				"members": s.getUniqueNodeNames(processes),
			}
			group["environments"] = append(group["environments"].([]map[string]interface{}), environment)
		}
		
		groups = append(groups, group)
	}
	
	return groups
}

// getUniqueNodeNames 获取进程列表中的唯一节点名称
func (s *SupervisorService) getUniqueNodeNames(processes []map[string]interface{}) []string {
	nodeSet := make(map[string]bool)
	for _, process := range processes {
		if nodeName, ok := process["node"].(string); ok {
			nodeSet[nodeName] = true
		}
	}
	
	nodes := make([]string, 0, len(nodeSet))
	for nodeName := range nodeSet {
		nodes = append(nodes, nodeName)
	}
	return nodes
}

// GetGroupDetails 获取特定分组的详细信息
func (s *SupervisorService) GetGroupDetails(groupName string) map[string]interface{} {
	groups := s.GetGroups()
	
	for _, group := range groups {
		if group["name"] == groupName {
			return group
		}
	}
	
	return nil
}

// StartGroupProcesses 启动分组中的所有进程
func (s *SupervisorService) StartGroupProcesses(groupName, environmentName string) error {
	return s.operateGroupProcesses(groupName, environmentName, "start")
}

// StopGroupProcesses 停止分组中的所有进程
func (s *SupervisorService) StopGroupProcesses(groupName, environmentName string) error {
	return s.operateGroupProcesses(groupName, environmentName, "stop")
}

// RestartGroupProcesses 重启分组中的所有进程
func (s *SupervisorService) RestartGroupProcesses(groupName, environmentName string) error {
	return s.operateGroupProcesses(groupName, environmentName, "restart")
}

// operateGroupProcesses 对分组中的进程执行操作
func (s *SupervisorService) operateGroupProcesses(groupName, environmentName, operation string) error {
	// 收集符合条件的节点列表,避免在持有锁时进行网络操作
	s.mu.RLock()
	var nodes []*Node
	for _, node := range s.nodes {
		if environmentName != "" && node.Environment != environmentName {
			continue
		}
		isConnected, _ := node.GetConnectionStatus()
		if !isConnected {
			continue
		}
		nodes = append(nodes, node)
	}
	s.mu.RUnlock()
	
	for _, node := range nodes {
		node.RefreshProcesses()
		
		// 获取进程列表的副本
		processes := node.SerializeProcesses()
		
		for _, processData := range processes {
			processName, _ := processData["name"].(string)
			processGroupName, _ := processData["group"].(string)
			
			if processGroupName == "" {
				processGroupName = "default"
			}
			
			if processGroupName == groupName {
				switch operation {
				case "start":
					node.StartProcess(processName)
				case "stop":
					node.StopProcess(processName)
				case "restart":
					node.RestartProcess(processName)
				}
			}
		}
	}
	
	return nil
}

func (s *SupervisorService) StartAutoRefresh(interval time.Duration) chan struct{} {
	ticker := time.NewTicker(interval)
	stopChan := make(chan struct{})

	go func() {
		defer ticker.Stop() // 确保ticker被正确清理
		for {
			select {
			case <-ticker.C:
				// 刷新所有节点状态
				logger.Debug("Auto-refreshing node connections")
				
				// 收集节点列表，避免在持有锁时进行网络操作
				s.mu.RLock()
				nodes := make([]*Node, 0, len(s.nodes))
				for _, node := range s.nodes {
					nodes = append(nodes, node)
				}
				s.mu.RUnlock()
				
				// 在锁外进行网络操作
				for _, node := range nodes {
					prevConnected, _ := node.GetConnectionStatus()
					if err := node.Connect(); err == nil {
						// Connect() 内部已在节点锁内设置 IsConnected 和 LastPing
						if !prevConnected {
							logger.Info("Node reconnected",
								zap.String("node_name", node.Name))
						}
					} else {
						if prevConnected {
							logger.Warn("Node disconnected",
								zap.String("node_name", node.Name),
								zap.Error(err))
						}
					}
				}
			case <-stopChan:
				logger.Debug("Stopping auto-refresh goroutine")
				return
			}
		}
	}()

	return stopChan
}

// StopAutoRefresh 停止自动刷新
func (s *SupervisorService) StopAutoRefresh(stopChan chan struct{}) {
	if stopChan != nil {
		close(stopChan)
	}
}

// Shutdown 优雅关闭SupervisorService，清理所有资源
func (s *SupervisorService) Shutdown(ctx context.Context) error {
	// 使用原子操作设置shutdown标志
	if !atomic.CompareAndSwapInt32(&s.shutdown, 0, 1) {
		return nil // 已经关闭
	}
	
	logger.Info("Shutting down SupervisorService")
	
	// 关闭stopChan，通知所有goroutine停止
	close(s.stopChan)
	
	// 等待所有goroutine完成，带超时控制
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		logger.Info("All SupervisorService goroutines stopped gracefully")
	case <-ctx.Done():
		logger.Warn("SupervisorService shutdown timeout, some goroutines may still be running")
		// 不返回错误，继续清理
	}
	
	// 清理超时管理器
	if s.timeoutManager != nil {
		s.timeoutManager.Cleanup()
	}
	
	// 清理所有节点连接
	s.mu.Lock()
	for name, node := range s.nodes {
		if node.client != nil {
			// 假设Node有Close方法来清理连接
			logger.Debug("Closing node connection", zap.String("node_name", name))
		}
	}
	s.nodes = make(map[string]*Node) // 清空节点映射
	s.mu.Unlock()
	
	logger.Info("SupervisorService shutdown completed")
	return nil
}

// IsShutdown 检查服务是否已关闭
func (s *SupervisorService) IsShutdown() bool {
	return atomic.LoadInt32(&s.shutdown) != 0
}

// Lifecycle 接口实现

// Start 启动 SupervisorService
func (s *SupervisorService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if atomic.LoadInt32(&s.shutdown) != 0 {
		return errors.NewInternalError("service already shutdown", nil)
	}

	logger.Info("Starting SupervisorService")

	// 初始化 stopChan（如果需要）
	if s.stopChan == nil {
		s.stopChan = make(chan struct{})
	}

	// 尝试连接所有节点
	for _, node := range s.nodes {
		if err := node.Connect(); err != nil {
			logger.Warn("Failed to connect to node during startup",
				zap.String("node_name", node.Name),
				zap.Error(err))
		} else {
			logger.Info("Node connected successfully",
				zap.String("node_name", node.Name))
		}
	}

	logger.Info("SupervisorService started successfully")
	return nil
}

// Stop 停止 SupervisorService（实现 Lifecycle 接口）
func (s *SupervisorService) Stop(ctx context.Context) error {
	return s.Shutdown(ctx)
}

// Health 健康检查（实现 Lifecycle 接口）
func (s *SupervisorService) Health() HealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if atomic.LoadInt32(&s.shutdown) != 0 {
		return HealthStatus{
			Status:    "unhealthy",
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"reason": "service is shutdown",
			},
		}
	}

	// 统计节点状态
	totalNodes := len(s.nodes)
	connectedNodes := 0
	for _, node := range s.nodes {
		isConnected, _ := node.GetConnectionStatus()
		if isConnected {
			connectedNodes++
		}
	}

	// 确定健康状态
	status := "healthy"
	if connectedNodes == 0 && totalNodes > 0 {
		status = "unhealthy"
	} else if connectedNodes < totalNodes {
		status = "degraded"
	}

	return HealthStatus{
		Status:    status,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"total_nodes":     totalNodes,
			"connected_nodes": connectedNodes,
		},
	}
}

// HealthStatus 健康状态（与 lifecycle.HealthStatus 兼容）
type HealthStatus struct {
	Status    string
	Timestamp time.Time
	Details   map[string]interface{}
}
