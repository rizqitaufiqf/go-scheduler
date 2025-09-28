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

// reconcileTasks finds tasks stuck in 'processing' or 'pending' or 'retrying' state on startup and requeues them.
func (w *Worker) reconcileTasks() {
	log.Println("Reconciling tasks...")

	var tasksToReconcile []dto.TaskScheduler
	// 1. Find all tasks that were stuck in 'processing' OR are 'pending' and past their scheduled time.
	// This covers both crashed workers and tasks that might have been missed if Redis lost data.
	statuses := []dto.TaskStatus{dto.StatusProcessing, dto.StatusPending, dto.StatusRetrying}
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

	log.Printf("Found %d tasks to reconcile. Re-queuing in Redis and updating status...", len(tasksToReconcile))
	var idsToSetPending []uuid.UUID
	for _, task := range tasksToReconcile {
		// 2. Re-add the task to the Redis sorted set.
		// The worker will pick it up based on its original scheduled_at time.
		w.redis.ZAdd(w.ctx, repo.TasksQueueKey(), &redis.Z{
			Score:  float64(task.ScheduledAt.Unix()),
			Member: task.ID.String(),
		})

		// Only change status if it was 'processing'. 'pending' and 'retrying' are already valid queueable states.
		if task.Status == dto.StatusProcessing {
			idsToSetPending = append(idsToSetPending, task.ID)
		}
	}

	// 3. Bulk update the status of all tasks that were stuck in 'processing' back to 'pending'.
	// We leave 'retrying' tasks as they are.
	if len(idsToSetPending) > 0 {
		if err := w.db.WithContext(w.ctx).Model(&dto.TaskScheduler{}).Where("id IN ?", idsToSetPending).Update("status", dto.StatusPending).Error; err != nil {
			log.Printf("Error updating status for reconciled tasks: %v", err)
			return
		}
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
		go func(id string, attempt int) {
			defer func() { <-w.sem }() // Release token
			w.executeTask(id, attempt)
		}(taskIDStr, 1) // First attempt is always 1
	}
}

func (w *Worker) executeTask(taskIDStr string, attempt int) {
	lockKey := repo.TaskLockKey(taskIDStr)

	// 1) Acquire Redis lock fast to avoid thundering herd
	locked, err := w.redis.SetNX(w.ctx, lockKey, "processing", lockTTL).Result()
	if err != nil {
		log.Printf("Redis SetNX error for %s: %v", taskIDStr, err)
		return
	}
	if !locked {
		// someone else is processing
		return
	}
	// always attempt to delete lock; if you need refresh logic for long jobs, implement it
	defer func() {
		if _, err := w.redis.Del(w.ctx, lockKey).Result(); err != nil {
			log.Printf("Warning: failed to delete lock %s: %v", lockKey, err)
		}
	}()

	// parse ID
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		log.Printf("Invalid task ID '%s': %v", taskIDStr, err)
		return
	}

	// 2) Start DB tx and lock row FOR UPDATE to prevent concurrent DB updates
	tx := w.db.WithContext(w.ctx).Begin()
	var task dto.TaskScheduler
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("TaskEntity").
		Preload("TaskAction").
		Where("id = ?", taskID).
		First(&task).Error; err != nil {
		tx.Rollback()
		if err != gorm.ErrRecordNotFound {
			log.Printf("Error selecting task %s: %v", taskID, err)
		}
		return
	}

	// 3) Check preconditions (DB is source of truth)
	if task.DeletedAt.Valid {
		tx.Rollback()
		// task deleted -> nothing to do
		return
	}
	// If not in a runnable state (pending or retrying), skip.
	if task.Status != dto.StatusPending && task.Status != dto.StatusRetrying {
		tx.Rollback()
		// If you removed from Redis earlier, prefer enqueuer to re-add;
		// do NOT re-add automatically here to avoid races with admin actions.
		return
	}

	// 4) Remove from Redis queue AFTER verifying DB (we hold Redis lock)
	if _, err := w.redis.ZRem(w.ctx, repo.TasksQueueKey(), taskIDStr).Result(); err != nil {
		// Log, but continue — DB authoritative
		log.Printf("Warning: ZREM failed for %s: %v", taskIDStr, err)
	}

	// 5) Mark as processing; persist BEFORE actual processing
	task.Status = dto.StatusProcessing
	if err := tx.Save(&task).Error; err != nil {
		tx.Rollback()
		log.Printf("Error marking task %s processing: %v", task.ID, err)
		return
	}
	if err := tx.Commit().Error; err != nil {
		log.Printf("Error committing tx for task %s processing: %v", task.ID, err)
		return
	}

	log.Printf("Processing task %s (%s:%s), attempt %d/%d", task.ID, task.TaskEntity.Name, task.TaskAction.Name, attempt, task.MaxRetries)

	time.Sleep(10 * time.Second)
	// 6) Execute processing outside transaction (so long-running tasks don't block DB)
	procErr := w.process(&task)

	// 7) Outcome handling — update DB and requeue if needed
	if procErr != nil {
		task.Result = procErr.Error()
		if attempt >= task.MaxRetries {
			task.Status = dto.StatusFailed
			if err := w.db.WithContext(w.ctx).Save(&task).Error; err != nil {
				log.Printf("Error saving failed task %s: %v", task.ID, err)
			}
			log.Printf("Task %s failed permanently after %d attempts: %v", task.ID, attempt, procErr)
			return
		}

		// schedule retry with backoff; persist BEFORE pushing to Redis
		backoffDuration := time.Duration(attempt*10) * time.Second
		task.ScheduledAt = time.Now().Add(backoffDuration)
		task.Status = dto.StatusRetrying

		if err := w.db.WithContext(w.ctx).Save(&task).Error; err != nil {
			log.Printf("Error saving retry schedule for task %s: %v", task.ID, err)
			return
		}

		// Re-queue the task for the next attempt.
		// We do this by spawning a new goroutine that will execute the task again
		// after the backoff duration. This avoids re-adding to Redis and keeps the
		// retry logic self-contained within the worker process that holds the lock.
		go func(id string, nextAttempt int) {
			time.Sleep(backoffDuration)
			// We don't release the semaphore here, as this goroutine is a continuation of the parent.
			// The lock is also still held.
			w.executeTask(id, nextAttempt)
		}(taskIDStr, attempt+1)
		log.Printf("Task %s failed, scheduled retry in %v. Error: %v", task.ID, backoffDuration, procErr)
		return
	}

	// success
	task.Status = dto.StatusCompleted
	task.Result = "Success"
	if err := w.db.WithContext(w.ctx).Save(&task).Error; err != nil {
		log.Printf("Error saving completed status for task %s: %v", task.ID, err)
		return
	}
	log.Printf("Task %s completed successfully", task.ID)
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
	// Register Product task processors
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
