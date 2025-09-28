package worker

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/rizqitaufiqf/go-scheduler/config"
	dto "github.com/rizqitaufiqf/go-scheduler/dto"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaskProcessor interface {
	Process(task *dto.TaskScheduler) error
}

// Worker is the core component responsible for polling, claiming, and processing scheduled tasks.
type Worker struct {
	db           *gorm.DB
	redis        *redis.Client
	ctx          context.Context
	pollInterval time.Duration
	concurrency  int
	sem          chan struct{}            // Semaphore to limit concurrency
	processors   map[string]TaskProcessor // Key format: "ENTITY_NAME:ACTION_NAME"

	// Timing and resilience configurations
	lockTTL               time.Duration
	periodicReconInterval time.Duration
	maxFailRefresh        int
	backoffBaseDelay      time.Duration
	backoffMaxDelay       time.Duration
}

// NewWorker creates and initializes a new Worker instance.
func NewWorker(ctx context.Context, db *gorm.DB, redis *redis.Client, cfg *config.Config) *Worker {
	w := &Worker{
		db:           db,
		redis:        redis,
		ctx:          ctx,
		pollInterval: cfg.WorkerPollInterval,
		concurrency:  cfg.WorkerConcurrency,
		sem:          make(chan struct{}, cfg.WorkerConcurrency),
		processors:   make(map[string]TaskProcessor),

		lockTTL:               cfg.LockTTL,
		periodicReconInterval: cfg.PeriodicReconInterval,
		maxFailRefresh:        cfg.MaxFailRefresh,
		backoffBaseDelay:      cfg.BackoffBaseDelay,
		backoffMaxDelay:       cfg.BackoffMaxDelay,
	}
	w.registerProcessors()
	return w
}

// Start begins the worker's processing loop.
func (w *Worker) Start() {
	log.Printf("Starting worker with concurrency=%d and poll_interval=%s...", w.concurrency, w.pollInterval)
	// On startup, run a one-time reconciliation to recover tasks from a potential previous crash.
	w.reconcileTasks()

	// Start a background process for periodic self-healing of the task queue.
	go w.startPeriodicReconciliation()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		w.processDueTasks()
	}
}

// startPeriodicReconciliation launches a background goroutine that periodically checks for inconsistencies
// between the database (the source of truth) and the Redis queue.
func (w *Worker) startPeriodicReconciliation() {
	ticker := time.NewTicker(w.periodicReconInterval)
	defer ticker.Stop()

	log.Printf("Starting periodic reconciler to run every %v", w.periodicReconInterval)

	for range ticker.C {
		w.reconcileMissingQueueTasks()
	}
}

// reconcileMissingQueueTasks is a self-healing mechanism. It finds tasks in the database that are in a
// queueable state ('pending', 'retrying') but are either missing from the Redis queue or have an incorrect score, and corrects them.
func (w *Worker) reconcileMissingQueueTasks() {
	log.Println("Running periodic check for tasks missing from queue...")

	var tasks []dto.TaskScheduler
	// Look for tasks in a queueable state scheduled within the last 24 hours.
	// This window prevents checking very old, likely irrelevant tasks.
	err := w.db.WithContext(w.ctx).
		Where("status IN ? AND scheduled_at > ?",
			[]dto.TaskStatus{dto.StatusPending, dto.StatusRetrying},
			time.Now().Add(-24*time.Hour),
		).Find(&tasks).Error

	if err != nil {
		log.Printf("Periodic Reconciler: Error fetching tasks: %v", err)
		return
	}

	for _, task := range tasks {
		// Check if the task exists in the Redis sorted set and if its score is correct.
		score, err := w.redis.ZScore(w.ctx, repo.TasksQueueKey(), task.ID.String()).Result()
		expectedScore := float64(task.ScheduledAt.Unix())

		// If the task is missing from the queue (err == redis.Nil) or its score (schedule time)
		// is out of sync with the database, we correct it by re-adding it with the proper score.
		if err == redis.Nil || (err == nil && score != expectedScore) {
			if err == redis.Nil {
				log.Printf("Periodic Reconciler: Found missing task %s in queue. Re-queuing.", task.ID)
			} else {
				log.Printf("Periodic Reconciler: Found task %s with incorrect score. Updating score from %f to %f.", task.ID, score, expectedScore)
			}
			_ = w.redis.ZAdd(w.ctx, repo.TasksQueueKey(), &redis.Z{Score: expectedScore, Member: task.ID.String()}).Err()
		}
	}
}

// reconcileTasks is a one-time recovery process that runs on worker startup. It finds tasks that
// may have been left in an inconsistent state (e.g., 'processing') due to a crash and requeues them.
func (w *Worker) reconcileTasks() {
	log.Println("Reconciling tasks...")

	var tasksToReconcile []dto.TaskScheduler
	// 1. Find all tasks that are in a state that might require recovery.
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
	var (
		idsToSetPending   []uuid.UUID
		idsToResetRetries []uuid.UUID
	)

	for _, task := range tasksToReconcile {
		// 2. Ensure the task exists in the Redis queue. ZADD is idempotent and will just update the score if it already exists.
		w.redis.ZAdd(w.ctx, repo.TasksQueueKey(), &redis.Z{
			Score:  float64(task.ScheduledAt.Unix()),
			Member: task.ID.String(),
		})

		// 3. Collect IDs for bulk status updates based on their state.
		switch task.Status {
		case dto.StatusProcessing:
			// A task is stuck in 'processing' but its lock has expired. This indicates a crash.
			// It's safe to move it back to 'pending' for re-processing.
			if val, _ := w.redis.Get(w.ctx, repo.TaskLockKey(task.ID.String())).Result(); val == "" {
				idsToSetPending = append(idsToSetPending, task.ID)
			}
		case dto.StatusRetrying:
			idsToResetRetries = append(idsToResetRetries, task.ID)
		}
	}

	// 4. Atomically update the status of all recovered 'processing' tasks back to 'pending'.
	if len(idsToSetPending) > 0 {
		if err := w.db.WithContext(w.ctx).Model(&dto.TaskScheduler{}).Where("id IN ?", idsToSetPending).Update("status", dto.StatusPending).Error; err != nil {
			log.Printf("Error updating status for reconciled tasks: %v", err)
			// Do not return here, try to process the other updates
		}
	}

	// 5. As requested by business logic, reset the retry_count for all 'retrying' tasks upon restart.
	// if len(idsToResetRetries) > 0 {
	// 	if err := w.db.WithContext(w.ctx).Model(&dto.TaskScheduler{}).Where("id IN ?", idsToResetRetries).Update("retry_count", 0).Error; err != nil {
	// 		log.Printf("Error resetting retry_count for reconciled tasks: %v", err)
	// 		return
	// 	}
	// }

	log.Println("Reconciliation complete.")
}

// processDueTasks is the main polling function. It attempts to claim and dispatch due tasks from the queue.
func (w *Worker) processDueTasks() {
	// If the concurrency limit is already reached, don't bother trying to claim more tasks.
	if len(w.sem) == cap(w.sem) {
		log.Println("Concurrency limit reached, pausing claims for this tick.")
		return
	}

	// This Lua script atomically finds a due task, acquires a lock, and removes it from the queue.
	// This is a critical optimization to prevent the "thundering herd" problem, where multiple
	// workers might fetch and try to process the same task simultaneously.
	const luaClaim = `
		-- Find the next due task
		local ids = redis.call("ZRANGEBYSCORE", KEYS[1], 0, ARGV[1], "LIMIT", 0, 1)
		if #ids == 0 then
			return nil
		end
		local id = ids[1]

		-- Try to acquire a lock for this task ID
		if redis.call("SET", KEYS[2]..id, ARGV[2], "PX", ARGV[3], "NX") then
			-- Lock acquired, now remove from the queue
			redis.call("ZREM", KEYS[1], id)
			return id
		end

		-- Could not acquire lock (another worker was faster), so return nil
		return nil
    `
	claimScript := redis.NewScript(luaClaim)

	// Attempt to claim new tasks until the worker's concurrency capacity is full.
	for i := 0; i < w.concurrency; i++ {
		// If we've reached the concurrency limit, stop trying to claim more tasks.
		if len(w.sem) == cap(w.sem) {
			break
		}

		// Use a unique token for the lock to ensure we can safely release it later.
		token := uuid.NewString()
		now := time.Now().Unix()
		lockMillis := w.lockTTL.Milliseconds()

		taskID, err := claimScript.Run(w.ctx, w.redis, []string{repo.TasksQueueKey(), repo.TaskLockKeyPrefix()}, now, token, lockMillis).Result()
		if err == redis.Nil {
			break // No more due tasks
		}
		if err != nil {
			log.Printf("Error running claim script: %v", err)
			continue
		}

		if taskIDStr, ok := taskID.(string); ok {
			log.Println("Claimed task:", taskIDStr)
			w.sem <- struct{}{} // Acquire a semaphore slot
			go func(id string, lockToken string) {
				defer func() { <-w.sem }() // Release semaphore slot
				w.executeTask(id, lockToken)
			}(taskIDStr, token)
		}
	}
}

func (w *Worker) executeTask(taskIDStr string, token string) {
	now := time.Now()
	lockKey := repo.TaskLockKey(taskIDStr)

	// Start a watchdog goroutine to periodically refresh the lock's TTL. This is essential for
	// long-running jobs to prevent the lock from expiring, which could lead to another worker
	// erroneously picking up the same task.
	done := make(chan struct{})
	go func() {
		// Ticker fires at half the lock's TTL to ensure timely refresh.
		ticker := time.NewTicker(w.lockTTL / 2)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Lua script to safely extend the lock's TTL only if we still own it.
				const luaRefresh = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("PEXPIRE", KEYS[1], ARGV[2]) else return 0 end`
				w.redis.Eval(w.ctx, luaRefresh, []string{lockKey}, token, int64(w.lockTTL/time.Millisecond)).Err()
			case <-done:
				// The main function has finished, so stop the watchdog.
				return
			}
		}
	}()

	// Defer the lock release. This Lua script ensures we only delete the lock if we still own it
	// (i.e., the token matches), preventing the accidental deletion of a lock acquired by another worker.
	defer func() {
		const lua = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
		_ = w.redis.Eval(w.ctx, lua, []string{lockKey}, token).Err()
	}()

	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		log.Printf("Invalid task ID '%s': %v", taskIDStr, err)
		return
	}

	// Ensure the watchdog is stopped when the function exits.
	// This defer runs *before* the lock release defer because of Go's LIFO (Last-In, First-Out) defer order.
	defer close(done)

	// 1. Begin a database transaction and acquire a pessimistic row-level lock (FOR UPDATE).
	// This prevents any other process from modifying this task record in the DB while we work.
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

	// 2. Validate preconditions using the authoritative data from the database.
	if task.DeletedAt.Valid {
		tx.Rollback()
		// Task has been soft-deleted, so there's nothing to do.
		return
	}
	// If not in a runnable state (pending or retrying), skip.
	if task.Status != dto.StatusPending && task.Status != dto.StatusRetrying {
		tx.Rollback()
		// The task is not in a runnable state (e.g., it was paused or canceled). Stop processing.
		return
	}

	// 3. Update task state to 'processing' and commit before execution.
	task.Status = dto.StatusProcessing
	// Increment the retry count for this attempt.
	task.RetryCount++
	// Record the start time for observability.
	task.StartedAt = &now
	if err := tx.Save(&task).Error; err != nil {
		tx.Rollback()
		log.Printf("Error marking task %s processing: %v", task.ID, err)
		return
	}
	if err := tx.Commit().Error; err != nil {
		log.Printf("Error committing tx for task %s processing: %v", task.ID, err)
		return
	}

	log.Printf("Processing task %s (%s:%s), attempt %d/%d", task.ID, task.TaskEntity.Name, task.TaskAction.Name, task.RetryCount, task.MaxRetries)

	time.Sleep(10 * time.Second)
	// 4. Execute the actual task logic. This is done outside the database transaction.
	procErr := w.process(&task)

	// 5. Handle the outcome of the task execution.
	if procErr != nil {
		if task.RetryCount >= task.MaxRetries {
			now := time.Now()
			task.FinishedAt = &now
			task.Result = procErr.Error()
			// errMsg := procErr.Error()
			// if len(errMsg) > 2000 {
			// 	errMsg = errMsg[:2000]
			// }
			// task.Result = errMsg

			task.Status = dto.StatusFailed
			if err := w.db.WithContext(w.ctx).Save(&task).Error; err != nil {
				log.Printf("Error saving failed task %s: %v", task.ID, err)
			}
			log.Printf("Task %s failed permanently after %d attempts: %v", task.ID, task.RetryCount, procErr)
			return
		}

		// The task failed but can be retried. Calculate the next attempt time using exponential backoff with jitter.
		// Calculate exponential backoff: base * 2^(retry_count-1), capped at maxDelay.
		backoff := min(w.backoffBaseDelay*time.Duration(1<<uint(task.RetryCount-1)), w.backoffMaxDelay)
		// Add jitter (a random duration up to 20% of the backoff) to spread out retries.
		jitter := time.Duration(rand.Int63n(int64(backoff / 5)))
		totalDelay := backoff + jitter
		newScheduledAt := time.Now().Add(totalDelay)

		// Update the task object with the new schedule, status, and the latest error details.
		task.ScheduledAt = newScheduledAt
		task.Status = dto.StatusRetrying
		now := time.Now()
		task.LastErrorAt = &now
		task.Result = procErr.Error()
		errMsg := procErr.Error()
		if len(errMsg) > 2000 {
			errMsg = errMsg[:2000]
		}
		task.Result = errMsg

		// Persist the 'retrying' state to the database BEFORE re-adding it to the Redis queue.
		if err := w.db.WithContext(w.ctx).Save(&task).Error; err != nil {
			log.Printf("Error saving retry schedule for task %s: %v", task.ID, err)
			return
		}

		// Re-queue the task in Redis with the new future execution time.
		if err := w.redis.ZAdd(w.ctx, repo.TasksQueueKey(), &redis.Z{
			Score:  float64(newScheduledAt.Unix()),
			Member: task.ID.String(),
		}).Err(); err != nil {
			log.Printf("CRITICAL: Failed to re-queue task %s for retry: %v", task.ID, err)
			// This is a critical but recoverable error. The task is in the DB as 'retrying' but not in the queue.
			// The reconcile process on next startup will fix this, but it's worth logging as critical.
		}

		log.Printf("Task %s failed, re-queued for retry in %v. Error: %v", task.ID, totalDelay.Round(time.Second), procErr)
		return
	}

	// Task completed successfully.
	now = time.Now()
	task.FinishedAt = &now
	task.Status = dto.StatusCompleted
	task.Result = "Success"
	if err := w.db.WithContext(w.ctx).Save(&task).Error; err != nil {
		log.Printf("Error saving completed status for task %s: %v", task.ID, err)
		return
	}
	log.Printf("Task %s completed successfully", task.ID)
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

// process looks up and dispatches a task to its registered processor.
func (w *Worker) process(task *dto.TaskScheduler) error {
	// Create a key from the loaded entity and action names
	key := getProcessorKey(task.TaskEntity.Name, task.TaskAction.Name)
	processor, exists := w.processors[key]
	if !exists {
		return fmt.Errorf("unknown task processor for key: %s", key)
	}
	return processor.Process(task)
}
