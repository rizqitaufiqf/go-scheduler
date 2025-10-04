package tasks

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rizqitaufiqf/go-scheduler/config"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"github.com/rizqitaufiqf/go-scheduler/repository"
	"github.com/rizqitaufiqf/go-scheduler/tasks"
)

// ReconciliationProcessor handles the task reconciliation logic.
type ReconciliationProcessor struct {
	taskRepo repository.TaskRepository //nolint:revive
	client   *tasks.Client             //nolint:revive
	cfg      *config.Config            //nolint:revive
}

// NewReconciliationProcessor creates a new processor for reconciliation tasks.
func NewReconciliationProcessor(taskRepo repository.TaskRepository, client *tasks.Client, cfg *config.Config) *ReconciliationProcessor { //nolint:revive
	return &ReconciliationProcessor{
		taskRepo: taskRepo,
		client:   client,
		cfg:      cfg,
	}
}

// ProcessReconciliationTask finds tasks in the DB that are missing from Redis and re-enqueues them.
func (p *ReconciliationProcessor) ProcessReconciliationTask(ctx context.Context, t *asynq.Task) error {
	if t != nil {
		log.Println("[RECONCILE_PERIODIC] Starting periodic task reconciliation process...")
	}

	// 1. Find potentially orphaned tasks from the database.
	// We check for all tasks that should have a corresponding entry in Redis.
	var orphanedTasks []dto.TaskScheduler
	statuses := []string{
		string(dto.StatusScheduled), // Task is scheduled for the future.
		string(dto.StatusPending),
		string(dto.StatusRetrying),
	}

	orphanedTasks, err := p.taskRepo.FindTasksByStatuses(ctx, statuses, 1, 1000) // Limit to 1000 tasks per run
	if err != nil {
		return fmt.Errorf("failed to find orphaned tasks: %w", err)
	}

	if len(orphanedTasks) == 0 {
		log.Println("[RECONCILE] No potentially orphaned tasks found.")
		return nil
	}

	log.Printf("[RECONCILE] Found %d potentially orphaned tasks. Checking against Redis...", len(orphanedTasks))
	reEnqueuedCount := 0

	for _, dbTask := range orphanedTasks {
		log.Printf("[RECONCILE_CHECK] Verifying task %s (Status: %s) against Redis...", dbTask.ID, dbTask.Status)
		queueName := p.getQueueNameFromPriority(dbTask.Priority)
		if queueName == "" {
			log.Printf("[RECONCILE_SKIP] Task %s: could not determine queue name for priority %d", dbTask.ID, dbTask.Priority)
			continue
		}

		_, err := p.client.GetTaskInfo(queueName, dbTask.ID.String())
		log.Println(err)
		if err != nil && errors.Is(err, asynq.ErrTaskNotFound) {
			// Task is missing in Redis, re-enqueue it!
			log.Printf("[RECONCILE_ACTION] Re-enqueuing missing task. DB_ID: %s, Queue: %s", dbTask.ID, queueName)

			// To re-enqueue, we need the full task details including entity and action name.
			fullTask, err := p.taskRepo.FindTaskWithDetails(ctx, dbTask.ID.String())
			if err != nil {
				log.Printf("[RECONCILE_ERROR] Could not get full details for task %s: %v", dbTask.ID, err)
				continue
			}

			taskType := fullTask.EntityName + ":" + fullTask.ActionName
			opts := []asynq.Option{
				asynq.MaxRetry(fullTask.MaxRetries),
				asynq.Queue(queueName),
				asynq.TaskID(fullTask.ID.String()),
			}

			var newStatus dto.TaskStatus
			if fullTask.ScheduledAt.After(time.Now()) {
				_, err = p.client.ScheduleTask(taskType, fullTask.Payload, fullTask.ScheduledAt, opts...)
				newStatus = dto.StatusScheduled
			} else {
				_, err = p.client.EnqueueTask(taskType, fullTask.Payload, opts...)
				newStatus = dto.StatusPending
			}

			if err != nil {
				log.Printf("[RECONCILE_ERROR] Failed to re-enqueue task %s: %v", dbTask.ID, err)
			} else {
				p.taskRepo.UpdateTaskStatus(ctx, dbTask.ID.String(), newStatus)
				reEnqueuedCount++
			}
		}
	}

	log.Printf("[RECONCILE] Reconciliation complete. Re-enqueued %d tasks.", reEnqueuedCount)
	return nil
}

// getQueueNameFromPriority is a helper to find the queue name string from its integer value.
func (p *ReconciliationProcessor) getQueueNameFromPriority(priorityLevel int) string {
	for name, level := range p.cfg.Queues {
		if level == priorityLevel {
			return name
		}
	}
	return "" // Or return "default"
}
