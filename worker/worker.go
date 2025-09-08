package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	dto "github.com/rizqitaufiqf/go-scheduler/dto"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const lockTTL = 5 * time.Minute // How long to lock a task for processing

// TaskProcessor defines the interface for processing a specific task type.
type TaskProcessor interface {
	Process(task *dto.ScheduledTask) error
}

// Worker is responsible for picking up and processing tasks.
type Worker struct {
	db           *gorm.DB
	redis        *redis.Client
	ctx          context.Context
	pollInterval time.Duration
	concurrency  int
	sem          chan struct{} // Semaphore to limit concurrency
	processors   map[string]TaskProcessor
}

func NewWorker(db *gorm.DB, redis *redis.Client, concurrency int, pollInterval time.Duration) *Worker {
	w := &Worker{
		db:           db,
		redis:        redis,
		ctx:          context.Background(),
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
	var tasksToReconcile []dto.ScheduledTask
	statuses := []dto.TaskStatus{dto.StatusProcessing, dto.StatusPending}
	if err := w.db.Where("status IN ?", statuses).Find(&tasksToReconcile).Error; err != nil {
		log.Printf("Error reconciling tasks: %v", err)
		return
	}

	if len(tasksToReconcile) > 0 {
		log.Printf("Reconciling %d tasks stuck in processing or pending state...", len(tasksToReconcile))
		for _, task := range tasksToReconcile {
			// Re-add to Redis sorted set. The score ensures it will be picked up if due.
			w.redis.ZAdd(w.ctx, repo.TasksQueueKey(), &redis.Z{
				Score:  float64(task.ScheduledAt.Unix()),
				Member: task.ID.String(),
			})
		}
	}
}

// processDueTasks fetches and processes tasks that are scheduled to run.
func (w *Worker) processDueTasks() {
	now := time.Now().Unix()
	// Fetch tasks from Redis that are due (score <= now)
	taskIDs, err := w.redis.ZRangeByScore(w.ctx, repo.TasksQueueKey(), &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", now),
	}).Result()
	if err != nil {
		log.Printf("Error fetching due tasks from Redis: %v", err)
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
		log.Printf("Invalid task ID format '%s': %v. Removing from queue.", taskIDStr, err)
		return
	}
	var task dto.ScheduledTask

	// Use a transaction to ensure atomicity
	tx := w.db.Begin()
	// Find a task that is either pending or was stuck in processing.
	statuses := []dto.TaskStatus{dto.StatusProcessing, dto.StatusPending}
	if err := tx.Where("id = ? AND status IN ?", taskID, statuses).First(&task).Error; err != nil {
		tx.Rollback()
		if err == gorm.ErrRecordNotFound {
			// Task was not found in the DB, but was in Redis. It might have been deleted. Nothing more to do.
		}
		return
	}

	// Mark task as processing
	task.Status = dto.StatusProcessing
	if err := tx.Save(&task).Error; err != nil {
		tx.Rollback()
		return
	}
	tx.Commit()

	// Process the task
	err = w.process(&task)

	// Update task status based on processing outcome
	if err != nil {
		task.Status = dto.StatusFailed
		task.Result = err.Error()
		log.Printf("Task %s failed: %v", task.ID, err)
	} else {
		task.Status = dto.StatusCompleted
		task.Result = "Success"
		log.Printf("Task %s completed successfully", task.ID)
	}

	if err := w.db.Save(&task).Error; err != nil {
		log.Printf("Error updating final task status for %s: %v", task.ID, err)
		// The task is processed, but we failed to update its status.
		// The reconcile logic will pick it up on next restart.
		return
	}
}

// registerProcessors initializes and maps task types to their processors.
func (w *Worker) registerProcessors() {
	productRepo := repo.NewProductRepository(w.db)
	w.processors[dto.TaskTypeProductCreate] = &ProductCreateProcessor{repo: productRepo}
	w.processors[dto.TaskTypeProductUpdate] = &ProductUpdateProcessor{repo: productRepo}
	w.processors[dto.TaskTypeProductDelete] = &ProductDeleteProcessor{repo: productRepo}
}

func (w *Worker) process(task *dto.ScheduledTask) error {
	processor, exists := w.processors[task.TaskType]
	if !exists {
		return fmt.Errorf("unknown task type: %s", task.TaskType)
	}
	return processor.Process(task)
}

// --- Concrete Task Processor Implementations ---

type ProductCreateProcessor struct {
	repo repo.ProductRepository
}

func (p *ProductCreateProcessor) Process(task *dto.ScheduledTask) error {
	var product dto.Product
	if err := json.Unmarshal(task.Payload, &product); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	product.ID = uuid.New() // Ensure a new UUID is generated for creation
	time.Sleep(20 * time.Second)
	return p.repo.Create(&product)
}

type ProductUpdateProcessor struct {
	repo repo.ProductRepository
}

func (p *ProductUpdateProcessor) Process(task *dto.ScheduledTask) error {
	var product dto.Product
	if err := json.Unmarshal(task.Payload, &product); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return p.repo.Update(&product)
}

type ProductDeleteProcessor struct {
	repo repo.ProductRepository
}

func (p *ProductDeleteProcessor) Process(task *dto.ScheduledTask) error {
	var payload struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return p.repo.Delete(payload.ID)
}
