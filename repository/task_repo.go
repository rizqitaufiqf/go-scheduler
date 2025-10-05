package repository

import (
	"context"
	"time"

	"github.com/rizqitaufiqf/go-scheduler/dto"
	"gorm.io/gorm"
)

// TaskRepository provides an interface for database operations on TaskScheduler.
type TaskRepository interface {
	FindTaskByTaskID(ctx context.Context, taskID string) (*dto.TaskScheduler, error)
	ListTasks(ctx context.Context, queue, status string, page, pageSize int) ([]dto.TaskScheduler, int64, error)
	FindTasksByStatuses(ctx context.Context, statuses []string, page, pageSize int) ([]dto.TaskScheduler, error)
	UpdateTask(ctx context.Context, taskID string, updates map[string]interface{}) error
	UpdateTaskStatus(ctx context.Context, taskID string, status dto.TaskStatus) error
	MarkTaskAsProcessing(ctx context.Context, taskID string) error
	MarkTaskAsCompleted(ctx context.Context, taskID, result string) error
	MarkTaskAsFailed(ctx context.Context, taskID, lastError string) error
	MarkTaskAsArchived(ctx context.Context, taskID, finalError string) error
	FindTaskWithDetails(ctx context.Context, taskID string) (*dto.TaskWithDetails, error)
	FindEntityByName(ctx context.Context, name string) (*dto.TaskEntity, error)
	FindActionByName(ctx context.Context, name string) (*dto.TaskAction, error)
	CreateTask(ctx context.Context, task *dto.TaskScheduler) error
}

type taskRepository struct {
	db *gorm.DB
}

// NewTaskRepository creates a new instance of TaskRepository.
func NewTaskRepository(db *gorm.DB) TaskRepository {
	return &taskRepository{db: db}
}

// FindTaskByTaskID finds a task in the database by its Asynq Task ID (which is our primary key).
func (r *taskRepository) FindTaskByTaskID(ctx context.Context, taskID string) (*dto.TaskScheduler, error) {
	var task dto.TaskScheduler
	if err := r.db.WithContext(ctx).Where("id = ?", taskID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// ListTasks retrieves a list of tasks from the database with filtering and pagination.
func (r *taskRepository) ListTasks(ctx context.Context, queue, status string, page, pageSize int) ([]dto.TaskScheduler, int64, error) {
	var tasks []dto.TaskScheduler
	var total int64

	query := r.db.WithContext(ctx).Model(&dto.TaskScheduler{})

	if queue != "" {
		query = query.Where("queue = ?", queue)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Count total records before pagination
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination and retrieve the data
	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&tasks).Error

	return tasks, total, err
}

// FindTasksByStatuses finds tasks that match one of the given statuses.
func (r *taskRepository) FindTasksByStatuses(ctx context.Context, statuses []string, page, pageSize int) ([]dto.TaskScheduler, error) {
	var tasks []dto.TaskScheduler
	query := r.db.WithContext(ctx).Model(&dto.TaskScheduler{})

	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}

	offset := (page - 1) * pageSize
	err := query.Order("created_at ASC").Offset(offset).Limit(pageSize).Find(&tasks).Error

	return tasks, err
}

// UpdateTask updates a task in the database with the given fields.
func (r *taskRepository) UpdateTask(ctx context.Context, taskID string, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&dto.TaskScheduler{}).Where("id = ?", taskID).Updates(updates).Error
}

// UpdateTaskStatus updates only the status of a task.
func (r *taskRepository) UpdateTaskStatus(ctx context.Context, taskID string, status dto.TaskStatus) error {
	return r.db.WithContext(ctx).Model(&dto.TaskScheduler{}).Where("id = ?", taskID).Update("status", status).Error
}

// MarkTaskAsProcessing updates the task status to 'processing' and sets the start time.
func (r *taskRepository) MarkTaskAsProcessing(ctx context.Context, taskID string) error {
	return r.db.WithContext(ctx).Model(&dto.TaskScheduler{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":     dto.StatusProcessing,
		"started_at": time.Now(),
	}).Error
}

// FindEntityByName finds a task entity by its name.
func (r *taskRepository) FindEntityByName(ctx context.Context, name string) (*dto.TaskEntity, error) {
	var entity dto.TaskEntity
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&entity).Error
	return &entity, err
}

// FindActionByName finds a task action by its name.
func (r *taskRepository) FindActionByName(ctx context.Context, name string) (*dto.TaskAction, error) {
	var action dto.TaskAction
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&action).Error
	return &action, err
}

// CreateTask creates a new task record in the database.
func (r *taskRepository) CreateTask(ctx context.Context, task *dto.TaskScheduler) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// FindTaskWithDetails finds a task and joins its entity and action names.
func (r *taskRepository) FindTaskWithDetails(ctx context.Context, taskID string) (*dto.TaskWithDetails, error) {
	var result dto.TaskWithDetails
	err := r.db.WithContext(ctx).
		Table("public.task_schedulers AS ts").
		Select("ts.*, te.name as entity_name, ta.name as action_name").
		Joins("JOIN public.task_entities te ON ts.task_entity_id = te.id").
		Joins("JOIN public.task_actions ta ON ts.task_action_id = ta.id").
		Where("ts.id = ?", taskID).
		First(&result).Error

	if err != nil {
		return nil, err
	}
	return &result, nil
}

// MarkTaskAsCompleted updates the task status to 'completed' and sets the finish time and result.
func (r *taskRepository) MarkTaskAsCompleted(ctx context.Context, taskID, result string) error {
	return r.db.WithContext(ctx).Model(&dto.TaskScheduler{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":      dto.StatusCompleted,
		"finished_at": time.Now(),
		"result":      result,
	}).Error
}

// MarkTaskAsFailed updates the task status, increments retry count, and logs the error.
func (r *taskRepository) MarkTaskAsFailed(ctx context.Context, taskID, lastError string) error {
	return r.db.WithContext(ctx).Model(&dto.TaskScheduler{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":        dto.StatusRetrying, // Or 'failed' if max retries is reached
		"last_error_at": time.Now(),
		"result":        lastError,
		"retry_count":   gorm.Expr("retry_count + 1"),
	}).Error
}

// MarkTaskAsArchived updates the task status to 'archived' after all retries have failed.
func (r *taskRepository) MarkTaskAsArchived(ctx context.Context, taskID, finalError string) error {
	return r.db.WithContext(ctx).Model(&dto.TaskScheduler{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":      dto.StatusArchived,
		"result":      finalError,
		"finished_at": time.Now(),
	}).Error
}
