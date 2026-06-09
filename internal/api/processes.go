package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"superview/internal/services"
	"superview/internal/supervisor"
	"superview/internal/validation"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ProcessesAPI struct {
	service            *supervisor.SupervisorService
	db                 *gorm.DB
	activityLogService *services.ActivityLogService
}

func NewProcessesAPI(service *supervisor.SupervisorService, db *gorm.DB, activityLogService *services.ActivityLogService) *ProcessesAPI {
	return &ProcessesAPI{
		service:            service,
		db:                 db,
		activityLogService: activityLogService,
	}
}

func (api *ProcessesAPI) getAccessibleNodes(c *gin.Context, action string) ([]*supervisor.Node, bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		return nil, false
	}

	nodes, err := filterNodesByAction(api.db, api.service.GetAllNodes(), user, action)
	if err != nil {
		handleAppError(c, err)
		return nil, false
	}

	return nodes, true
}

// AggregatedProcess 聚合的进程信息
type AggregatedProcess struct {
	Name             string            `json:"name"`
	TotalInstances   int               `json:"total_instances"`
	RunningInstances int               `json:"running_instances"`
	StoppedInstances int               `json:"stopped_instances"`
	Instances        []ProcessInstance `json:"instances"`
}

// ProcessInstance 进程实例信息
type ProcessInstance struct {
	NodeName    string  `json:"node_name"`
	NodeHost    string  `json:"node_host"`
	NodePort    int     `json:"node_port"`
	State       int     `json:"state"`
	StateString string  `json:"state_string"`
	PID         int     `json:"pid"`
	Uptime      float64 `json:"uptime"`
	UptimeHuman string  `json:"uptime_human"`
	Group       string  `json:"group"`
}

// GetAggregatedProcesses 获取聚合的进程列表
func (api *ProcessesAPI) GetAggregatedProcesses(c *gin.Context) {
	nodes, ok := api.getAccessibleNodes(c, "read")
	if !ok {
		return
	}
	if nodes == nil {
		c.JSON(http.StatusOK, gin.H{
			"status":    "success",
			"processes": []AggregatedProcess{},
		})
		return
	}

	// 按进程名聚合
	processMap := make(map[string]*AggregatedProcess)

	for _, node := range nodes {
		if !node.IsConnected {
			continue
		}

		// 刷新进程信息
		if err := node.RefreshProcesses(); err != nil {
			continue
		}

		for _, process := range node.Processes {
			// 获取或创建聚合进程
			aggProc, exists := processMap[process.Name]
			if !exists {
				aggProc = &AggregatedProcess{
					Name:      process.Name,
					Instances: make([]ProcessInstance, 0),
				}
				processMap[process.Name] = aggProc
			}

			// 添加实例
			instance := ProcessInstance{
				NodeName:    node.Name,
				NodeHost:    node.Host,
				NodePort:    node.Port,
				State:       process.State,
				StateString: process.StateString,
				PID:         process.PID,
				Uptime:      process.Uptime.Seconds(),
				UptimeHuman: formatDuration(process.Uptime),
				Group:       process.Group,
			}
			aggProc.Instances = append(aggProc.Instances, instance)

			// 更新统计
			aggProc.TotalInstances++
			if process.State == 20 { // RUNNING
				aggProc.RunningInstances++
			} else if process.State == 0 { // STOPPED
				aggProc.StoppedInstances++
			}
		}
	}

	// 转换为数组
	processes := make([]AggregatedProcess, 0, len(processMap))
	for _, proc := range processMap {
		processes = append(processes, *proc)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"processes": processes,
	})
}

// BatchOperationResult 批量操作结果
type BatchOperationResult struct {
	ProcessName    string                    `json:"process_name"`
	TotalInstances int                       `json:"total_instances"`
	SuccessCount   int                       `json:"success_count"`
	FailureCount   int                       `json:"failure_count"`
	Results        []InstanceOperationResult `json:"results"`
}

// InstanceOperationResult 单个实例操作结果
type InstanceOperationResult struct {
	NodeName string `json:"node_name"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// BatchStartProcess 批量启动进程
func (api *ProcessesAPI) BatchStartProcess(c *gin.Context) {
	processName := c.Param("process_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateProcessName("process_name", processName)
	validator.ValidateNoSQLInjection("process_name", processName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	// 执行批量操作
	nodes, ok := api.getAccessibleNodes(c, "execute")
	if !ok {
		return
	}

	result := api.batchOperation(nodes, processName, "start")

	// 记录日志
	if api.activityLogService != nil {
		message := fmt.Sprintf("Batch started process %s on %d instances (%d succeeded, %d failed)",
			processName, result.TotalInstances, result.SuccessCount, result.FailureCount)
		api.activityLogService.LogWithContext(c, "INFO", "start_process", "process", processName, message, nil)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"result": result,
	})
}

// BatchStopProcess 批量停止进程
func (api *ProcessesAPI) BatchStopProcess(c *gin.Context) {
	processName := c.Param("process_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateProcessName("process_name", processName)
	validator.ValidateNoSQLInjection("process_name", processName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	// 执行批量操作
	nodes, ok := api.getAccessibleNodes(c, "execute")
	if !ok {
		return
	}

	result := api.batchOperation(nodes, processName, "stop")

	// 记录日志
	if api.activityLogService != nil {
		message := fmt.Sprintf("Batch stopped process %s on %d instances (%d succeeded, %d failed)",
			processName, result.TotalInstances, result.SuccessCount, result.FailureCount)
		api.activityLogService.LogWithContext(c, "INFO", "stop_process", "process", processName, message, nil)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"result": result,
	})
}

// BatchRestartProcess 批量重启进程
func (api *ProcessesAPI) BatchRestartProcess(c *gin.Context) {
	processName := c.Param("process_name")

	// 输入验证
	validator := validation.NewValidator()
	validator.ValidateProcessName("process_name", processName)
	validator.ValidateNoSQLInjection("process_name", processName)

	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	// 执行批量操作
	nodes, ok := api.getAccessibleNodes(c, "execute")
	if !ok {
		return
	}

	result := api.batchOperation(nodes, processName, "restart")

	// 记录日志
	if api.activityLogService != nil {
		message := fmt.Sprintf("Batch restarted process %s on %d instances (%d succeeded, %d failed)",
			processName, result.TotalInstances, result.SuccessCount, result.FailureCount)
		api.activityLogService.LogWithContext(c, "INFO", "restart_process", "process", processName, message, nil)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"result": result,
	})
}

// batchOperation 执行批量操作
func (api *ProcessesAPI) batchOperation(nodes []*supervisor.Node, processName, operation string) BatchOperationResult {
	result := BatchOperationResult{
		ProcessName: processName,
		Results:     make([]InstanceOperationResult, 0),
	}

	if nodes == nil {
		return result
	}

	// 创建超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 并行处理
	var wg sync.WaitGroup
	resultChan := make(chan InstanceOperationResult, len(nodes))

	for _, node := range nodes {
		if !node.IsConnected {
			continue
		}

		// 检查进程是否存在
		if err := node.RefreshProcesses(); err != nil {
			continue
		}

		hasProcess := false
		for _, proc := range node.Processes {
			if proc.Name == processName {
				hasProcess = true
				break
			}
		}

		if !hasProcess {
			continue
		}

		result.TotalInstances++

		wg.Add(1)
		go func(n *supervisor.Node) {
			defer wg.Done()

			opResult := InstanceOperationResult{
				NodeName: n.Name,
				Success:  false,
			}

			// 使用 channel 处理超时
			done := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "start":
					err = api.service.StartProcess(n.Name, processName)
				case "stop":
					err = api.service.StopProcess(n.Name, processName)
				case "restart":
					err = api.service.StopProcess(n.Name, processName)
					if err == nil {
						time.Sleep(100 * time.Millisecond)
						err = api.service.StartProcess(n.Name, processName)
					}
				}
				done <- err
			}()

			select {
			case err := <-done:
				if err != nil {
					opResult.Error = err.Error()
				} else {
					opResult.Success = true
				}
			case <-ctx.Done():
				opResult.Error = "operation timeout"
			}

			resultChan <- opResult
		}(node)
	}

	// 等待所有操作完成
	wg.Wait()
	close(resultChan)

	// 收集结果
	for opResult := range resultChan {
		result.Results = append(result.Results, opResult)
		if opResult.Success {
			result.SuccessCount++
		} else {
			result.FailureCount++
		}
	}

	return result
}

// formatDuration 格式化时间间隔
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	if d < time.Hour {
		return d.Round(time.Minute).String()
	}
	if d < 24*time.Hour {
		return d.Round(time.Hour).String()
	}
	return d.Round(24 * time.Hour).String()
}
