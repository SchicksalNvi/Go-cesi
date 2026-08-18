package repository

import (
	"gorm.io/gorm"
	"superview/internal/models"
)

// UserRepository 用户数据访问接口
type UserRepository interface {
	Create(user *models.User) error
	GetByID(id string) (*models.User, error)
	GetByUsername(username string) (*models.User, error)
	GetByEmail(email string) (*models.User, error)
	Update(user *models.User) error
	UpdateFields(id string, values map[string]interface{}) error
	UpdateFieldsWithVersion(id string, values map[string]interface{}, tokenVersion uint64) error
	UpdateFieldsAndRevokeSessions(id string, values map[string]interface{}) error
	Delete(id string) error
	List(offset, limit int) ([]*models.User, int64, error)
	ExistsByUsername(username string) (bool, error)
	ExistsByEmail(email string) (bool, error)
}

// NodeRepository 节点数据访问接口
type NodeRepository interface {
	Create(node *models.Node) error
	GetByID(id uint) (*models.Node, error)
	GetByName(name string) (*models.Node, error)
	Update(node *models.Node) error
	Delete(id uint) error
	List(offset, limit int) ([]*models.Node, int64, error)
	GetByStatus(status string) ([]*models.Node, error)
	ExistsByName(name string) (bool, error)
	ExistsByHostPort(host string, port int) (bool, error)
}

// AlertRepository 告警数据访问接口
type AlertRepository interface {
	CreateRule(rule *models.AlertRule) error
	GetRuleByID(id uint) (*models.AlertRule, error)
	UpdateRule(rule *models.AlertRule) error
	DeleteRule(id uint) error
	ListRules(offset, limit int) ([]*models.AlertRule, int64, error)
	GetActiveRules() ([]*models.AlertRule, error)

	CreateAlert(alert *models.Alert) error
	GetAlertByID(id uint) (*models.Alert, error)
	UpdateAlert(alert *models.Alert) error
	DeleteAlert(id uint) error
	ListAlerts(offset, limit int) ([]*models.Alert, int64, error)
	GetActiveAlerts() ([]*models.Alert, error)
	GetAlertsByRuleID(ruleID uint) ([]*models.Alert, error)
}

// DiscoveryRepository 节点发现数据访问接口
type DiscoveryRepository interface {
	// Task operations
	CreateTask(task *models.DiscoveryTask) error
	GetTask(id uint) (*models.DiscoveryTask, error)
	UpdateTask(task *models.DiscoveryTask) error
	DeleteTask(id uint) error
	ListTasks(offset, limit int, status string) ([]*models.DiscoveryTask, int64, error)

	// Result operations
	CreateResult(result *models.DiscoveryResult) error
	CreateResults(results []*models.DiscoveryResult) error
	GetResultsByTaskID(taskID uint) ([]*models.DiscoveryResult, error)
}

// Repository 仓库接口集合
type Repository struct {
	User      UserRepository
	Node      NodeRepository
	Alert     AlertRepository
	Discovery DiscoveryRepository
}

// NewRepository 创建新的仓库实例
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		User:      NewUserRepository(db),
		Node:      NewNodeRepository(db),
		Alert:     NewAlertRepository(db),
		Discovery: NewDiscoveryRepository(db),
	}
}
