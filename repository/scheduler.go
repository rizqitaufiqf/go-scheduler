package repository

import (
	"context"

	"github.com/google/uuid"
	dto "github.com/rizqitaufiqf/go-scheduler/dto"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Scheduler is responsible for adding tasks to the queue.
type Scheduler struct {
	db    *gorm.DB
	redis *redis.Client
	ctx   context.Context
}

func NewScheduler(ctx context.Context, db *gorm.DB, redis *redis.Client) *Scheduler {
	return &Scheduler{
		db:    db,
		redis: redis,
		ctx:   ctx,
	}
}

// ScheduleTask creates a task in PostgreSQL and adds it to the Redis sorted set.
func (r *Scheduler) ScheduleTask(task *dto.TaskScheduler) error {
	// 1. Save the task to PostgreSQL as the source of truth
	if err := r.db.WithContext(r.ctx).Create(task).Error; err != nil {
		return err
	}

	// 2. Add the task to Redis sorted set, scored by its execution time.
	// This makes polling for due tasks extremely efficient.
	return r.redis.ZAdd(r.ctx, TasksQueueKey(), &redis.Z{
		Score:  float64(task.ScheduledAt.Unix()),
		Member: task.ID.String(),
	}).Err()
}

// FindTasks retrieves tasks from the database, with an optional status filter.
func (r *Scheduler) FindTasks(status string) ([]dto.TaskResponse, error) {
	var tasks []dto.TaskResponse

	// Build a query that joins the necessary tables and selects specific fields
	// for a more efficient database operation compared to Preload for this case.
	query := r.db.WithContext(r.ctx).Model(&dto.TaskScheduler{}).
		Select("task_schedulers.id, task_schedulers.payload, task_schedulers.scheduled_at, task_schedulers.priority, task_schedulers.max_retries, task_schedulers.status, task_schedulers.result, task_schedulers.created_at, task_schedulers.updated_at, te.name as task_entity_name, ta.name as task_action_name").
		Joins("JOIN public.task_entities te ON te.id = task_schedulers.task_entity_id").
		Joins("JOIN public.task_actions ta ON ta.id = task_schedulers.task_action_id").
		Order("task_schedulers.created_at desc")

	// Apply status filter if provided
	if status != "" {
		query = query.Where("task_schedulers.status = ?", status)
	}

	// Scan the results into the TaskResponse struct. GORM will map the aliased columns.
	err := query.Scan(&tasks).Error

	return tasks, err
}

// PauseTask changes a task's status to 'paused' and removes it from the Redis queue.
func (r *Scheduler) PauseTask(taskID uuid.UUID) (*dto.TaskResponse, error) {
	var task dto.TaskScheduler

	err := r.db.WithContext(r.ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Find the task and lock the row. Only 'pending' tasks can be paused.
		// Preload the entity and action to return the full response.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("TaskEntity").
			Preload("TaskAction").First(&task, "id = ? AND status = ?", taskID, dto.StatusPending).Error; err != nil {
			return err // Returns gorm.ErrRecordNotFound if not found or not pending
		}

		// 2. Update the status in the database.
		task.Status = dto.StatusPaused
		if err := tx.Save(&task).Error; err != nil {
			return err
		}

		// 3. Remove the task from the Redis queue.
		if err := r.redis.ZRem(r.ctx, TasksQueueKey(), task.ID.String()).Err(); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Construct the response DTO
	response := &dto.TaskResponse{
		ID:             task.ID,
		TaskEntityName: task.TaskEntity.Name,
		TaskActionName: task.TaskAction.Name,
		Payload:        task.Payload,
		ScheduledAt:    task.ScheduledAt,
		Priority:       task.Priority,
		MaxRetries:     task.MaxRetries,
		Status:         task.Status,
		Result:         task.Result,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}
	return response, nil
}

// ResumeTask changes a task's status to 'pending' and adds it back to the Redis queue.
func (r *Scheduler) ResumeTask(taskID uuid.UUID) (*dto.TaskResponse, error) {
	var task dto.TaskScheduler

	err := r.db.WithContext(r.ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Find the task and lock the row. Only 'paused' tasks can be resumed.
		// Preload the entity and action to return the full response.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("TaskEntity").
			Preload("TaskAction").
			First(&task, "id = ? AND status = ?", taskID, dto.StatusPaused).Error; err != nil {
			return err // Returns gorm.ErrRecordNotFound if not found or not paused
		}

		// 2. Update the status in the database.
		task.Status = dto.StatusPending
		if err := tx.Save(&task).Error; err != nil {
			return err
		}

		// 3. Add the task back to the Redis queue.
		return r.redis.ZAdd(r.ctx, TasksQueueKey(), &redis.Z{
			Score:  float64(task.ScheduledAt.Unix()),
			Member: task.ID.String(),
		}).Err()
	})

	if err != nil {
		return nil, err
	}

	// Construct the response DTO
	response := &dto.TaskResponse{
		ID:             task.ID,
		TaskEntityName: task.TaskEntity.Name,
		TaskActionName: task.TaskAction.Name,
		Payload:        task.Payload,
		ScheduledAt:    task.ScheduledAt,
		Priority:       task.Priority,
		MaxRetries:     task.MaxRetries,
		Status:         task.Status,
		Result:         task.Result,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}
	return response, nil
}
