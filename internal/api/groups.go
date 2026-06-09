package api

import (
	"fmt"
	"net/http"

	"superview/internal/services"
	"superview/internal/supervisor"
	"superview/internal/validation"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type GroupsAPI struct {
	service            *supervisor.SupervisorService
	db                 *gorm.DB
	activityLogService *services.ActivityLogService
}

func NewGroupsAPI(service *supervisor.SupervisorService, db *gorm.DB, activityLogService ...*services.ActivityLogService) *GroupsAPI {
	api := &GroupsAPI{service: service, db: db}
	if len(activityLogService) > 0 {
		api.activityLogService = activityLogService[0]
	}
	return api
}

func buildAllowedNodeSet(nodes []*supervisor.Node) map[string]bool {
	allowed := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		allowed[node.Name] = true
	}
	return allowed
}

func filterGroupsByNodeAccess(groups []map[string]interface{}, allowedNodes map[string]bool) []map[string]interface{} {
	filteredGroups := make([]map[string]interface{}, 0, len(groups))
	for _, group := range groups {
		environmentsValue, ok := group["environments"].([]map[string]interface{})
		if !ok {
			continue
		}

		filteredEnvironments := make([]map[string]interface{}, 0, len(environmentsValue))
		for _, environment := range environmentsValue {
			processesValue, ok := environment["processes"].([]map[string]interface{})
			if !ok {
				continue
			}

			filteredProcesses := make([]map[string]interface{}, 0, len(processesValue))
			for _, process := range processesValue {
				nodeName, _ := process["node"].(string)
				if allowedNodes[nodeName] {
					filteredProcesses = append(filteredProcesses, process)
				}
			}

			if len(filteredProcesses) == 0 {
				continue
			}

			filteredEnvironment := make(map[string]interface{}, len(environment))
			for key, value := range environment {
				filteredEnvironment[key] = value
			}
			filteredEnvironment["processes"] = filteredProcesses
			filteredEnvironment["members"] = buildMembersFromProcesses(filteredProcesses)
			filteredEnvironments = append(filteredEnvironments, filteredEnvironment)
		}

		if len(filteredEnvironments) == 0 {
			continue
		}

		filteredGroup := make(map[string]interface{}, len(group))
		for key, value := range group {
			filteredGroup[key] = value
		}
		filteredGroup["environments"] = filteredEnvironments
		filteredGroups = append(filteredGroups, filteredGroup)
	}

	return filteredGroups
}

func buildMembersFromProcesses(processes []map[string]interface{}) []string {
	memberSet := make(map[string]bool)
	for _, process := range processes {
		nodeName, _ := process["node"].(string)
		if nodeName != "" {
			memberSet[nodeName] = true
		}
	}

	members := make([]string, 0, len(memberSet))
	for nodeName := range memberSet {
		members = append(members, nodeName)
	}
	return members
}

func (g *GroupsAPI) getAccessibleNodes(c *gin.Context, action string) ([]*supervisor.Node, bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		return nil, false
	}

	nodes, err := filterNodesByAction(g.db, g.service.GetAllNodes(), user, action)
	if err != nil {
		handleAppError(c, err)
		return nil, false
	}

	return nodes, true
}

func (g *GroupsAPI) operateGroupProcesses(nodes []*supervisor.Node, groupName, environmentName, operation string) error {
	for _, node := range nodes {
		if environmentName != "" && node.Environment != environmentName {
			continue
		}

		isConnected, _ := node.GetConnectionStatus()
		if !isConnected {
			continue
		}

		if err := node.RefreshProcesses(); err != nil {
			return err
		}

		processes := node.SerializeProcesses()
		for _, processData := range processes {
			processName, _ := processData["name"].(string)
			processGroupName, _ := processData["group"].(string)

			if processGroupName == "" {
				processGroupName = "default"
			}

			if processGroupName != groupName {
				continue
			}

			var err error
			switch operation {
			case "start":
				err = node.StartProcess(processName)
			case "stop":
				err = node.StopProcess(processName)
			case "restart":
				err = node.RestartProcess(processName)
			}
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// GetGroups 获取所有进程分组
func (g *GroupsAPI) GetGroups(c *gin.Context) {
	nodes, ok := g.getAccessibleNodes(c, "read")
	if !ok {
		return
	}

	groups := filterGroupsByNodeAccess(g.service.GetGroups(), buildAllowedNodeSet(nodes))

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"groups": groups,
	})
}

// GetGroupDetails 获取特定分组的详细信息
func (g *GroupsAPI) GetGroupDetails(c *gin.Context) {
	groupName := c.Param("group_name")

	validator := validation.NewValidator()
	validator.ValidateProcessName("group_name", groupName)
	validator.ValidateNoSQLInjection("group_name", groupName)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	nodes, ok := g.getAccessibleNodes(c, "read")
	if !ok {
		return
	}

	filteredGroups := filterGroupsByNodeAccess(g.service.GetGroups(), buildAllowedNodeSet(nodes))
	var group map[string]interface{}
	for _, candidate := range filteredGroups {
		if candidate["name"] == groupName {
			group = candidate
			break
		}
	}
	if group == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "Group not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"group":  group,
	})
}

// StartGroupProcesses 启动分组中的所有进程
func (g *GroupsAPI) StartGroupProcesses(c *gin.Context) {
	groupName := c.Param("group_name")
	environmentName := c.Query("environment")

	validator := validation.NewValidator()
	validator.ValidateProcessName("group_name", groupName)
	validator.ValidateNoSQLInjection("group_name", groupName)
	validator.ValidateNoSQLInjection("environment", environmentName)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	nodes, ok := g.getAccessibleNodes(c, "execute")
	if !ok {
		return
	}

	err := g.operateGroupProcesses(nodes, groupName, environmentName, "start")
	if err != nil {
		handleAppError(c, err)
		return
	}

	if g.activityLogService != nil {
		msg := fmt.Sprintf("Started all processes in group %s", groupName)
		g.activityLogService.LogWithContext(c, "INFO", "start_group", "group", groupName, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Group processes started successfully",
	})
}

// StopGroupProcesses 停止分组中的所有进程
func (g *GroupsAPI) StopGroupProcesses(c *gin.Context) {
	groupName := c.Param("group_name")
	environmentName := c.Query("environment")

	validator := validation.NewValidator()
	validator.ValidateProcessName("group_name", groupName)
	validator.ValidateNoSQLInjection("group_name", groupName)
	validator.ValidateNoSQLInjection("environment", environmentName)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	nodes, ok := g.getAccessibleNodes(c, "execute")
	if !ok {
		return
	}

	err := g.operateGroupProcesses(nodes, groupName, environmentName, "stop")
	if err != nil {
		handleAppError(c, err)
		return
	}

	if g.activityLogService != nil {
		msg := fmt.Sprintf("Stopped all processes in group %s", groupName)
		g.activityLogService.LogWithContext(c, "INFO", "stop_group", "group", groupName, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Group processes stopped successfully",
	})
}

// RestartGroupProcesses 重启分组中的所有进程
func (g *GroupsAPI) RestartGroupProcesses(c *gin.Context) {
	groupName := c.Param("group_name")
	environmentName := c.Query("environment")

	validator := validation.NewValidator()
	validator.ValidateProcessName("group_name", groupName)
	validator.ValidateNoSQLInjection("group_name", groupName)
	validator.ValidateNoSQLInjection("environment", environmentName)
	if validator.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "输入验证失败",
			"errors":  validator.Errors(),
		})
		return
	}

	nodes, ok := g.getAccessibleNodes(c, "execute")
	if !ok {
		return
	}

	err := g.operateGroupProcesses(nodes, groupName, environmentName, "restart")
	if err != nil {
		handleAppError(c, err)
		return
	}

	if g.activityLogService != nil {
		msg := fmt.Sprintf("Restarted all processes in group %s", groupName)
		g.activityLogService.LogWithContext(c, "INFO", "restart_group", "group", groupName, msg, nil)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Group processes restarted successfully",
	})
}
