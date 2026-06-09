package api

import (
	"net/http"

	"superview/internal/supervisor"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type EnvironmentsAPI struct {
	service *supervisor.SupervisorService
	db      *gorm.DB
}

func NewEnvironmentsAPI(service *supervisor.SupervisorService, db *gorm.DB) *EnvironmentsAPI {
	return &EnvironmentsAPI{service: service, db: db}
}

func buildEnvironmentResponses(nodes []*supervisor.Node) []map[string]interface{} {
	environmentMap := make(map[string][]map[string]interface{})
	for _, node := range nodes {
		isConnected, lastPing := node.GetConnectionStatus()
		nodeInfo := map[string]interface{}{
			"name":         node.Name,
			"host":         node.Host,
			"port":         node.Port,
			"is_connected": isConnected,
			"last_ping":    lastPing,
		}
		environmentMap[node.Environment] = append(environmentMap[node.Environment], nodeInfo)
	}

	environments := make([]map[string]interface{}, 0, len(environmentMap))
	for envName, members := range environmentMap {
		environments = append(environments, map[string]interface{}{
			"name":    envName,
			"members": members,
		})
	}

	return environments
}

func buildEnvironmentDetail(nodes []*supervisor.Node, environmentName string) map[string]interface{} {
	members := make([]map[string]interface{}, 0)
	for _, node := range nodes {
		if node.Environment != environmentName {
			continue
		}

		isConnected, lastPing := node.GetConnectionStatus()
		members = append(members, map[string]interface{}{
			"name":         node.Name,
			"host":         node.Host,
			"port":         node.Port,
			"is_connected": isConnected,
			"last_ping":    lastPing,
			"processes":    node.GetProcessCount(),
		})
	}

	if len(members) == 0 {
		return nil
	}

	return map[string]interface{}{
		"name":    environmentName,
		"members": members,
	}
}

func (e *EnvironmentsAPI) getAccessibleNodes(c *gin.Context) ([]*supervisor.Node, bool) {
	user, ok := getCurrentUser(c)
	if !ok {
		return nil, false
	}

	nodes, err := filterNodesByAction(e.db, e.service.GetAllNodes(), user, "read")
	if err != nil {
		handleAppError(c, err)
		return nil, false
	}

	return nodes, true
}

// GetEnvironments 获取所有环境列表
func (e *EnvironmentsAPI) GetEnvironments(c *gin.Context) {
	nodes, ok := e.getAccessibleNodes(c)
	if !ok {
		return
	}

	environments := buildEnvironmentResponses(nodes)

	c.JSON(http.StatusOK, gin.H{
		"status":       "success",
		"environments": environments,
	})
}

// GetEnvironmentDetails 获取特定环境的详细信息
func (e *EnvironmentsAPI) GetEnvironmentDetails(c *gin.Context) {
	environmentName := c.Param("environment_name")

	nodes, ok := e.getAccessibleNodes(c)
	if !ok {
		return
	}

	environment := buildEnvironmentDetail(nodes, environmentName)
	if environment == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  "error",
			"message": "Environment not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"environment": environment,
	})
}
