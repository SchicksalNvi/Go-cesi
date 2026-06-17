package repository

import (
	"fmt"

	"superview/internal/errors"
	"superview/internal/models"

	"gorm.io/gorm"
)

type discoveryRepository struct {
	db *gorm.DB
}

// NewDiscoveryRepository creates a new discovery repository instance.
func NewDiscoveryRepository(db *gorm.DB) DiscoveryRepository {
	return &discoveryRepository{db: db}
}

// CreateTask creates a new discovery task.
func (r *discoveryRepository) CreateTask(task *models.DiscoveryTask) error {
	if err := r.db.Create(task).Error; err != nil {
		return errors.NewDatabaseError("create discovery task", err)
	}
	return nil
}

// GetTask retrieves a discovery task by ID.
func (r *discoveryRepository) GetTask(id uint) (*models.DiscoveryTask, error) {
	var task models.DiscoveryTask
	if err := r.db.Where("id = ?", id).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.NewNotFoundError("discovery task", fmt.Sprintf("%d", id))
		}
		return nil, errors.NewDatabaseError("get discovery task", err)
	}
	return &task, nil
}

// UpdateTask updates an existing discovery task.
func (r *discoveryRepository) UpdateTask(task *models.DiscoveryTask) error {
	if err := r.db.Save(task).Error; err != nil {
		return errors.NewDatabaseError("update discovery task", err)
	}
	return nil
}

// DeleteTask soft-deletes a discovery task by ID.
func (r *discoveryRepository) DeleteTask(id uint) error {
	if err := r.db.Where("id = ?", id).Delete(&models.DiscoveryTask{}).Error; err != nil {
		return errors.NewDatabaseError("delete discovery task", err)
	}
	return nil
}

// ListTasks retrieves discovery tasks with pagination and optional status filter.
// If status is empty, all tasks are returned.
// Returns tasks, total count, and error.
func (r *discoveryRepository) ListTasks(offset, limit int, status string) ([]*models.DiscoveryTask, int64, error) {
	var tasks []*models.DiscoveryTask
	var total int64

	query := r.db.Model(&models.DiscoveryTask{})

	// Apply status filter if provided
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, errors.NewDatabaseError("count discovery tasks", err)
	}

	// Get paginated data, ordered by creation time descending
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&tasks).Error; err != nil {
		return nil, 0, errors.NewDatabaseError("list discovery tasks", err)
	}

	return tasks, total, nil
}

// CreateResult creates a new discovery result.
func (r *discoveryRepository) CreateResult(result *models.DiscoveryResult) error {
	if err := r.db.Create(result).Error; err != nil {
		return errors.NewDatabaseError("create discovery result", err)
	}
	return nil
}

// CreateResults batch-inserts discovery results to avoid one INSERT per probed IP.
func (r *discoveryRepository) CreateResults(results []*models.DiscoveryResult) error {
	if len(results) == 0 {
		return nil
	}
	if err := r.db.CreateInBatches(results, 100).Error; err != nil {
		return errors.NewDatabaseError("create discovery results", err)
	}
	return nil
}

// GetResultsByTaskID retrieves all results for a given task ID, ordered by creation time.
func (r *discoveryRepository) GetResultsByTaskID(taskID uint) ([]*models.DiscoveryResult, error) {
	var results []*models.DiscoveryResult
	if err := r.db.Where("task_id = ?", taskID).Order("created_at ASC").Find(&results).Error; err != nil {
		return nil, errors.NewDatabaseError("get discovery results by task id", err)
	}
	return results, nil
}
