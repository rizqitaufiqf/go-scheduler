package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	dto "github.com/rizqitaufiqf/go-scheduler/dto"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const lockTTL = 5 * time.Minute // How long to lock a task for processing

// TaskProcessor defines the interface for processing a specific task type.
type TaskProcessor interface {
	Process(task *dto.TaskScheduler) error
}

// Worker is responsible for picking up and processing tasks.
type Worker struct {
	db           *gorm.DB
	redis        *redis.Client
	ctx          context.Context
	pollInterval time.Duration
	concurrency  int
	sem          chan struct{}            // Semaphore to limit concurrency
	processors   map[string]TaskProcessor // Key is "ENTITY_NAME:ACTION_NAME"
}

func NewWorker(ctx context.Context, db *gorm.DB, redis *redis.Client, concurrency int, pollInterval time.Duration) *Worker {
	w := &Worker{
		db:           db,
		redis:        redis,
		ctx:          ctx,
		pollInterval: pollInterval,
		concurrency:  concurrency,
		sem:          make(chan struct{}, concurrency),
		processors:   make(map[string]TaskProcessor),
	}
	w.registerProcessors()
	return w
}

// Start begins the worker's processing loop.
func (w *Worker) Start() {
	log.Printf("Starting worker with concurrency=%d and poll_interval=%s...", w.concurrency, w.pollInterval)
	// On startup, reconcile any tasks that were 'processing' in case of a crash.
	w.reconcileTasks()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		w.processDueTasks()
	}
}

// reconcileTasks finds tasks stuck in 'processing' or 'pending' state on startup and requeues them.
func (w *Worker) reconcileTasks() {
	log.Println("Reconciling tasks...")

	var tasksToReconcile []dto.TaskScheduler
	// 1. Find all tasks that were stuck in 'processing' OR are 'pending' and past their scheduled time.
	// This covers both crashed workers and tasks that might have been missed if Redis lost data.
	statuses := []dto.TaskStatus{dto.StatusProcessing, dto.StatusPending}
	if err := w.db.WithContext(w.ctx).
		Where("status IN ?", statuses).
		Find(&tasksToReconcile).Error; err != nil {
		log.Printf("Error finding tasks to reconcile: %v", err)
		return
	}

	if len(tasksToReconcile) == 0 {
		log.Println("No tasks to reconcile.")
		return
	}

	log.Printf("Found %d tasks to reconcile. Re-queuing in Redis and ensuring status is 'pending'...", len(tasksToReconcile))
	for _, task := range tasksToReconcile {
		// 2. Re-add the task to the Redis sorted set.
		// The worker will pick it up based on its original scheduled_at time.
		w.redis.ZAdd(w.ctx, repo.TasksQueueKey(), &redis.Z{
			Score:  float64(task.ScheduledAt.Unix()),
			Member: task.ID.String(),
		})
	}

	// 3. Bulk update the status of all reconciled tasks back to 'pending' in the database.
	if err := w.db.WithContext(w.ctx).Model(&dto.TaskScheduler{}).Where("id IN ?", getTaskIDs(tasksToReconcile)).Update("status", dto.StatusPending).Error; err != nil {
		log.Printf("Error updating status for reconciled tasks: %v", err)
		return
	}

	log.Println("Reconciliation complete.")
}

// processDueTasks fetches and processes tasks that are scheduled to run.
func (w *Worker) processDueTasks() {
	// Fetch tasks from Redis that are due (score <= now)
	taskIDs, err := w.redis.ZRangeByScore(w.ctx, repo.TasksQueueKey(), &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", time.Now().Unix()),
	}).Result()

	if err != nil {
		if err != redis.Nil {
			log.Printf("Error fetching due tasks from Redis: %v", err)
		}
		return
	}

	for _, taskIDStr := range taskIDs {
		log.Println("Processing task:", taskIDStr)
		w.sem <- struct{}{} // Acquire a token
		go func(id string) {
			defer func() { <-w.sem }() // Release token
			w.executeTask(id)
		}(taskIDStr)
	}
}

func (w *Worker) executeTask(taskIDStr string) {
	lockKey := repo.TaskLockKey(taskIDStr)
	// Try to acquire a distributed lock for this task.
	// This prevents multiple workers from processing the same task.
	locked, err := w.redis.SetNX(w.ctx, lockKey, "processing", lockTTL).Result()
	if err != nil || !locked {
		// Could not acquire lock, another worker is on it.
		return
	}
	defer w.redis.Del(w.ctx, lockKey)

	// Now that we have the lock, we can safely remove the task from the queue.
	// This prevents other workers or other poll cycles from trying to process this same task.
	w.redis.ZRem(w.ctx, repo.TasksQueueKey(), taskIDStr)

	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		log.Printf("Invalid task ID format '%s': %v", taskIDStr, err)
		return
	}

	var task dto.TaskScheduler
	tx := w.db.WithContext(w.ctx).Begin()

	// Find a pending task and lock it for update.
	// Preload entity and action names for the processor key.
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
		Preload("TaskEntity").
		Preload("TaskAction").
		Where("id = ? AND status = ?", taskID, dto.StatusPending).
		First(&task).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			// Task already processed or deleted, which is fine.
		} else {
			log.Printf("Error finding task %s: %v", taskID, err)
		}
		return
	}

	// Mark task as processing
	task.Status = dto.StatusProcessing
	task.RetryCount++ // Increment retry count for this attempt
	if err := tx.Save(&task).Error; err != nil {
		tx.Rollback()
		log.Printf("Error marking task %s as processing: %v", task.ID, err)
		return
	}
	tx.Commit()

	log.Printf("Processing task %s (%s:%s), attempt %d/%d", task.ID, task.TaskEntity.Name, task.TaskAction.Name, task.RetryCount, task.MaxRetries)

	// Process the task
	err = w.process(&task)

	// Update task status based on processing outcome
	if err != nil {
		task.Result = err.Error()
		if task.RetryCount >= task.MaxRetries {
			task.Status = dto.StatusFailed
			log.Printf("Task %s failed after %d attempts: %v", task.ID, task.MaxRetries, err)
		} else {
			// Re-schedule for retry with exponential backoff
			task.Status = dto.StatusPending
			backoffDuration := time.Duration(task.RetryCount*10) * time.Second
			task.ScheduledAt = time.Now().Add(backoffDuration)
			log.Printf("Task %s failed, will retry in %v. Error: %v", task.ID, backoffDuration, err)
			// Re-add to Redis queue for retry
			w.redis.ZAdd(w.ctx, repo.TasksQueueKey(), &redis.Z{
				Score:  float64(task.ScheduledAt.Unix()),
				Member: task.ID.String(),
			})
		}
	} else {
		task.Status = dto.StatusCompleted
		task.Result = "Success"
		log.Printf("Task %s completed successfully", task.ID)
	}

	if err := w.db.WithContext(w.ctx).Save(&task).Error; err != nil {
		log.Printf("Error updating final task status for %s: %v", task.ID, err)
	}
}

// getTaskIDs is a helper function to extract IDs from a slice of tasks.
func getTaskIDs(tasks []dto.TaskScheduler) []uuid.UUID {
	ids := make([]uuid.UUID, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	return ids
}

// getProcessorKey creates a consistent key for the processors map.
func getProcessorKey(entityName, actionName string) string {
	return fmt.Sprintf("%s:%s", entityName, actionName)
}

// registerProcessors initializes and maps task types to their processors.
func (w *Worker) registerProcessors() {
	productRepo := repo.NewProductRepository(w.db)
	w.processors[getProcessorKey(dto.TaskEntityProduct, dto.TaskActionCreate)] = &ProductCreateProcessor{repo: productRepo}
	w.processors[getProcessorKey(dto.TaskEntityProduct, dto.TaskActionUpdate)] = &ProductUpdateProcessor{repo: productRepo}
	w.processors[getProcessorKey(dto.TaskEntityProduct, dto.TaskActionDelete)] = &ProductDeleteProcessor{repo: productRepo}
}

func (w *Worker) process(task *dto.TaskScheduler) error {
	// Create a key from the loaded entity and action names
	key := getProcessorKey(task.TaskEntity.Name, task.TaskAction.Name)
	processor, exists := w.processors[key]
	if !exists {
		return fmt.Errorf("unknown task processor for key: %s", key)
	}
	return processor.Process(task)
}
