package repository

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	dto "github.com/rizqitaufiqf/go-scheduler/dto"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Scheduler provides the core API for creating and managing tasks.
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

// ScheduleTaskIdemRedis schedules a task with Redis-only idempotency.
// Returns (resp, createdNew, inFlight, err).
func (r *Scheduler) ScheduleTask(task *dto.TaskScheduler, dedupKey string, pendingTTL, finalTTL time.Duration) (*dto.TaskResponse, bool, bool, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	idemKey := IdempotencyKey(dedupKey)
	log.Println("IdemKey:", idemKey)
	token := uuid.NewString()

	// 1) Reserve idempotency key (P:<token>)
	reserved, existing, err := ReserveIdem(ctx, r.redis, idemKey, token, pendingTTL)
	if err != nil {
		return nil, false, false, fmt.Errorf("idempotency reserve failed: %w", err)
	}
	if !reserved {
		// Key already exists
		if strings.HasPrefix(existing, "T:") {
			idStr := strings.TrimPrefix(existing, "T:")
			id, perr := uuid.Parse(idStr)
			if perr != nil {
				// Corrupt value; treat as not found
				return nil, false, false, nil
			}
			resp, ferr := r.FindTaskByID(id)
			return resp, false, false, ferr // duplicate; return original
		}
		// "P:<tokenX>" => in-flight by another request
		return nil, false, true, nil
	}

	// 2) Create the task in DB (DB-first)
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		// Optional: release reservation early; or let pending TTL expire
		_ = r.redis.Del(ctx, idemKey).Err()
		return nil, false, false, err
	}

	// 3) Finalize idempotency to T:<taskID>
	if err := FinalizeIdem(ctx, r.redis, idemKey, token, task.ID.String(), finalTTL); err != nil {
		log.Printf("CRITICAL: finalizeIdem failed for %s: %v", task.ID, err)
		// continue; not fatal
	}

	// 4) Enqueue to Redis ZSET (post-commit)
	if rErr := r.redis.ZAdd(ctx, TasksQueueKey(), &redis.Z{
		Score:  float64(task.ScheduledAt.Unix()),
		Member: task.ID.String(),
	}).Err(); rErr != nil {
		log.Printf("CRITICAL: Failed to queue task %s: %v", task.ID, rErr)
		// periodic reconciler will heal it
	}

	// 5) Build response
	resp, ferr := r.FindTaskByID(task.ID)
	return resp, true, false, ferr
}

// FindTasks retrieves a list of all tasks from the database, with an optional status filter.
func (r *Scheduler) FindTasks(status string) ([]dto.TaskResponse, error) {
	var tasks []dto.TaskResponse

	// Build a query that joins tables and selects specific fields for an efficient response,
	// avoiding the N+1 problem of a simple Preload.
	query := r.db.WithContext(r.ctx).Model(&dto.TaskScheduler{}).
		Select("task_schedulers.id, task_schedulers.payload, task_schedulers.scheduled_at, task_schedulers.priority, task_schedulers.retry_count, task_schedulers.max_retries, task_schedulers.status, task_schedulers.result, task_schedulers.started_at, task_schedulers.finished_at, task_schedulers.last_error_at, task_schedulers.created_at, task_schedulers.updated_at, te.name as task_entity_name, ta.name as task_action_name").
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

// FindTaskByID retrieves a single task by its ID for detailed viewing.
func (r *Scheduler) FindTaskByID(taskID uuid.UUID) (*dto.TaskResponse, error) {
	var task dto.TaskResponse

	// Use the same efficient join query as FindTasks, but filter by a single ID.
	query := r.db.WithContext(r.ctx).Model(&dto.TaskScheduler{}).
		Select("task_schedulers.id, task_schedulers.payload, task_schedulers.scheduled_at, task_schedulers.priority, task_schedulers.retry_count, task_schedulers.max_retries, task_schedulers.status, task_schedulers.result, task_schedulers.started_at, task_schedulers.finished_at, task_schedulers.last_error_at, task_schedulers.created_at, task_schedulers.updated_at, te.name as task_entity_name, ta.name as task_action_name").
		Joins("JOIN public.task_entities te ON te.id = task_schedulers.task_entity_id").
		Joins("JOIN public.task_actions ta ON ta.id = task_schedulers.task_action_id").
		Where("task_schedulers.id = ?", taskID)

	err := query.First(&task).Error // Use First to get a single record
	if err != nil {
		return nil, err // Returns gorm.ErrRecordNotFound if not found
	}

	return &task, nil
}

// FindEntityByName looks up a task entity by its unique name.
func (r *Scheduler) FindEntityByName(name string) (*dto.TaskEntity, error) {
	var entity dto.TaskEntity
	if err := r.db.WithContext(r.ctx).Where("name = ?", name).First(&entity).Error; err != nil {
		return nil, err // Returns gorm.ErrRecordNotFound if not found
	}
	return &entity, nil
}

// FindActionByName looks up a task action by its unique name.
func (r *Scheduler) FindActionByName(name string) (*dto.TaskAction, error) {
	var action dto.TaskAction
	if err := r.db.WithContext(r.ctx).Where("name = ?", name).First(&action).Error; err != nil {
		return nil, err // Returns gorm.ErrRecordNotFound if not found
	}
	return &action, nil
}

// PauseTask changes a task's status to 'paused' and removes it from the Redis queue.
func (r *Scheduler) PauseTask(taskID uuid.UUID) (*dto.TaskResponse, error) {
	var task dto.TaskScheduler

	// 1. Perform DB operations within a transaction.
	err := r.db.WithContext(r.ctx).Transaction(func(tx *gorm.DB) error {
		// Find the task and acquire a row-level lock. Only 'pending' tasks can be paused.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("TaskEntity").
			Preload("TaskAction").First(&task, "id = ? AND status = ?", taskID, dto.StatusPending).Error; err != nil {
			return err // Returns gorm.ErrRecordNotFound if not found or not pending
		}

		// Update the status in the database.
		task.Status = dto.StatusPaused
		return tx.Save(&task).Error
	})

	if err != nil {
		return nil, err
	}

	// 2. After successful DB commit, remove the task from the Redis queue.
	if rErr := r.redis.ZRem(r.ctx, TasksQueueKey(), task.ID.String()).Err(); rErr != nil {
		log.Printf("CRITICAL: Failed to remove paused task %s from Redis queue: %v", task.ID, rErr)
	}

	// Construct the response DTO from the retrieved task data.
	response := &dto.TaskResponse{
		ID:             task.ID,
		TaskEntityName: task.TaskEntity.Name,
		TaskActionName: task.TaskAction.Name,
		Payload:        task.Payload,
		ScheduledAt:    task.ScheduledAt,
		Priority:       task.Priority,
		RetryCount:     task.RetryCount,
		MaxRetries:     task.MaxRetries,
		Status:         task.Status,
		Result:         task.Result,
		StartedAt:      task.StartedAt,
		FinishedAt:     task.FinishedAt,
		LastErrorAt:    task.LastErrorAt,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}
	return response, nil
}

// RetryFailedTask resets a 'failed' task to 'retrying' and re-queues it to run immediately.
func (r *Scheduler) RetryFailedTask(taskID uuid.UUID) (*dto.TaskResponse, error) {
	var task dto.TaskScheduler

	// 1. Perform DB operations within a transaction.
	err := r.db.WithContext(r.ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("TaskEntity").
			Preload("TaskAction").
			First(&task, "id = ? AND status = ?", taskID, dto.StatusFailed).Error; err != nil {
			return err
		}

		// Change status to 'retrying' to allow it to be picked up by workers again.
		task.Status = dto.StatusRetrying
		task.Result = "Manually retried by user." // Clear the old failure result.
		task.RetryCount = 0                       // Reset the retry counter for a fresh start.
		task.ScheduledAt = time.Now()             // Set schedule time to now for consistency.
		return tx.Save(&task).Error
	})
	if err != nil {
		return nil, err
	}

	// 2. After successful DB commit, re-queue the task for immediate execution.
	if rErr := r.redis.ZAdd(r.ctx, TasksQueueKey(), &redis.Z{Score: 0, Member: task.ID.String()}).Err(); rErr != nil {
		log.Printf("CRITICAL: Failed to queue retried task %s in Redis: %v", task.ID, rErr)
	}

	return r.FindTaskByID(taskID)
}

// RunTaskNow manually triggers a task to run immediately by updating its score in Redis.
func (r *Scheduler) RunTaskNow(taskID uuid.UUID) (*dto.TaskResponse, error) {
	var task dto.TaskScheduler

	// 1. Perform DB operations within a transaction.
	err := r.db.WithContext(r.ctx).Transaction(func(tx *gorm.DB) error {
		// A task can be forced to run now from 'pending', 'paused', or 'retrying' states.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("TaskEntity").
			Preload("TaskAction").
			First(&task, "id = ? AND status IN ?", taskID, []dto.TaskStatus{
				dto.StatusPending, dto.StatusPaused, dto.StatusRetrying,
			}).Error; err != nil {
			return err
		}

		// If the task was paused, it's not in the queue. Change its status so it can be queued.
		if task.Status == dto.StatusPaused {
			task.Status = dto.StatusPending
			return tx.Save(&task).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 2. After successful DB commit, add/update the task in the queue for immediate processing.
	if rErr := r.redis.ZAdd(r.ctx, TasksQueueKey(), &redis.Z{Score: 0, Member: task.ID.String()}).Err(); rErr != nil {
		log.Printf("CRITICAL: Failed to queue task-now %s in Redis: %v", task.ID, rErr)
	}

	return r.FindTaskByID(taskID)
}

// ResumeTask changes a task's status to 'pending' and adds it back to the Redis queue.
func (r *Scheduler) ResumeTask(taskID uuid.UUID) (*dto.TaskResponse, error) {
	var task dto.TaskScheduler

	// 1. Perform DB operations within a transaction.
	err := r.db.WithContext(r.ctx).Transaction(func(tx *gorm.DB) error {
		// Find the task and lock the row. Only 'paused' tasks are eligible to be resumed.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("TaskEntity").
			Preload("TaskAction").
			First(&task, "id = ? AND status = ?", taskID, dto.StatusPaused).Error; err != nil {
			return err // Returns gorm.ErrRecordNotFound if not found or not paused
		}

		// Update the status in the database.
		task.Status = dto.StatusPending
		return tx.Save(&task).Error
	})

	if err != nil {
		return nil, err
	}

	// 2. After successful DB commit, add the task back to the Redis queue.
	if rErr := r.redis.ZAdd(r.ctx, TasksQueueKey(), &redis.Z{
		Score:  float64(task.ScheduledAt.Unix()),
		Member: task.ID.String(),
	}).Err(); rErr != nil {
		log.Printf("CRITICAL: Failed to re-queue resumed task %s in Redis: %v", task.ID, rErr)
	}

	// Construct the response DTO from the retrieved task data.
	response := &dto.TaskResponse{
		ID:             task.ID,
		TaskEntityName: task.TaskEntity.Name,
		TaskActionName: task.TaskAction.Name,
		Payload:        task.Payload,
		ScheduledAt:    task.ScheduledAt,
		Priority:       task.Priority,
		RetryCount:     task.RetryCount,
		MaxRetries:     task.MaxRetries,
		Status:         task.Status,
		Result:         task.Result,
		StartedAt:      task.StartedAt,
		FinishedAt:     task.FinishedAt,
		LastErrorAt:    task.LastErrorAt,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}
	return response, nil
}

// CancelTask moves a task to the terminal 'canceled' state and removes it from the queue if present.
func (r *Scheduler) CancelTask(taskID uuid.UUID) (*dto.TaskResponse, error) {
	var task dto.TaskScheduler
	var prevStatus dto.TaskStatus

	// 1. Perform DB operations within a transaction.
	err := r.db.WithContext(r.ctx).Transaction(func(tx *gorm.DB) error {
		// Find the task. Only tasks in a non-terminal, non-processing state can be canceled.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("TaskEntity").
			Preload("TaskAction").
			First(&task, "id = ? AND status IN ?", taskID, []dto.TaskStatus{dto.StatusPending, dto.StatusRetrying, dto.StatusPaused}).Error; err != nil {
			return err // Returns gorm.ErrRecordNotFound if not found or not in a cancelable state
		}

		// Note the previous status to determine if it needs to be removed from the Redis queue.
		prevStatus = task.Status

		// Update the status in the database to the terminal 'canceled' state.
		task.Status = dto.StatusCanceled
		return tx.Save(&task).Error
	})

	if err != nil {
		return nil, err
	}

	// 2. After successful DB commit, if the task was in the queue, remove it.
	if prevStatus == dto.StatusPending || prevStatus == dto.StatusRetrying {
		if rErr := r.redis.ZRem(r.ctx, TasksQueueKey(), task.ID.String()).Err(); rErr != nil {
			log.Printf("CRITICAL: ZRem failed for canceled task %s: %v", task.ID, rErr)
		}
	}

	// Construct the response DTO for the canceled task.
	response := &dto.TaskResponse{
		ID:             task.ID,
		TaskEntityName: task.TaskEntity.Name,
		TaskActionName: task.TaskAction.Name,
		Payload:        task.Payload,
		ScheduledAt:    task.ScheduledAt,
		StartedAt:      task.StartedAt,
		FinishedAt:     task.FinishedAt,
		LastErrorAt:    task.LastErrorAt,
		Status:         task.Status,
		Result:         "Task was canceled by the user.",
		UpdatedAt:      task.UpdatedAt,
	}
	return response, nil
}
