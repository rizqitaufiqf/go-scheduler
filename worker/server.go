package worker

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rizqitaufiqf/go-scheduler/config"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"
	"github.com/rizqitaufiqf/go-scheduler/tasks"
	processors "github.com/rizqitaufiqf/go-scheduler/tasks/processors"
	"gorm.io/gorm"
)

type Server struct {
	asynqServer             *asynq.Server
	mux                     *asynq.ServeMux
	scheduler               *asynq.Scheduler //nolint:staticcheck
	taskRepo                repo.TaskRepository
	reconciliationProcessor *processors.ReconciliationProcessor
}

// NewServer creates a new Asynq server with configured workers
func NewServer(redisOpt asynq.RedisClientOpt, cfg *config.Config, db *gorm.DB, client *tasks.Client) *Server {
	// Initialize repository for database operations
	taskRepo := repo.NewTaskRepository(db)

	// Initialize the periodic task scheduler
	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{})

	// Configure server
	serverConfig := asynq.Config{
		// Number of concurrent workers
		Concurrency: cfg.WorkerConcurrency,

		// Queue priorities (higher number = higher priority)
		Queues: cfg.Queues,

		// Retry configuration
		RetryDelayFunc: getRetryDelayFunc(cfg.RetryDelayFunc),

		// Error handler
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			retried, _ := asynq.GetRetryCount(ctx)
			maxRetry, _ := asynq.GetMaxRetry(ctx)
			if retried >= maxRetry {
				log.Printf("[DEAD] Task %s, ID %s, has failed permanently: %v", task.Type(), task.ResultWriter().TaskID(), err)
			}
		}),

		// Graceful shutdown timeout
		ShutdownTimeout: 30 * time.Second,

		// Health check function
		HealthCheckFunc: func(err error) {
			if err != nil {
				log.Printf("[HEALTH] Unhealthy: %v", err)
			}
		},
	}

	// Create server
	server := asynq.NewServer(redisOpt, serverConfig)

	// Create a new mux (task router)
	mux := asynq.NewServeMux()

	// Initialize all dependencies (repos and processors)
	deps := NewDependencyContainer(db, client, cfg)

	// Register all handlers from our dependency container
	deps.RegisterAllHandlers(mux, scheduler)

	return &Server{
		asynqServer:             server,
		mux:                     mux,
		scheduler:               scheduler,
		taskRepo:                taskRepo,
		reconciliationProcessor: deps.ReconciliationProcessor,
	}
}

// Run starts the Asynq server and the periodic task scheduler.
func (s *Server) Run() error {
	// Trigger initial reconciliation on startup in a separate goroutine
	// Wrap the mux with our custom middleware before running
	var handler asynq.Handler = s.mux
	handler = s.dbLoggingMiddleware(handler) // This middleware will now handle DB status updates

	go func() {
		log.Println("[RECONCILE_INIT] Performing initial task reconciliation on startup...")
		if err := s.reconciliationProcessor.ProcessReconciliationTask(context.Background(), nil); err != nil {
			log.Printf("[ERROR] Initial reconciliation failed: %v", err)
		}
	}()

	go func() {
		if err := s.scheduler.Run(); err != nil {
			log.Fatalf("Could not run scheduler: %v", err)
		}
	}()
	return s.asynqServer.Run(handler)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() {
	log.Println("Shutting down Asynq worker server...")
	s.scheduler.Shutdown()
	s.asynqServer.Shutdown()
	log.Println("Asynq worker server stopped")
}

// Middleware functions
func (s *Server) dbLoggingMiddleware(next asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
		taskID := t.ResultWriter().TaskID() // This is the consistent UUID

		// 1. Update status to 'processing' when the task starts
		if err := s.taskRepo.MarkTaskAsProcessing(ctx, taskID); err != nil {
			log.Printf("[DB_ERROR] Failed to update task %s to processing: %v", taskID, err)
			// We can choose to continue or halt, for now we log and continue.
		}

		log.Printf("[START] Task: %s, ID: %s", t.Type(), taskID)
		start := time.Now()

		err := next.ProcessTask(ctx, t)

		duration := time.Since(start)
		log.Printf("[TIMING] Task: %s, ID: %s, took %v", t.Type(), taskID, duration)

		if strings.Split(taskID, ":")[0] == "SYSTEM" {
			return err
		}

		if err != nil {
			// 2. Task failed, check if it will be retried
			retryCount, ok := asynq.GetRetryCount(ctx)
			if !ok {
				retryCount = 0
			}
			maxRetry, ok := asynq.GetMaxRetry(ctx)
			if !ok {
				maxRetry = 0
			}

			log.Printf("[FAILED] Task: %s, ID: %s, Error: %v, Retry: %d/%d", t.Type(), taskID, err, retryCount, maxRetry)

			if retryCount < maxRetry {
				// It will be retried. Update status and retry_count.
				// We use MarkTaskAsFailed which sets the status to 'retrying' and increments the count.
				if dbErr := s.taskRepo.MarkTaskAsFailed(ctx, taskID, err.Error()); dbErr != nil {
					log.Printf("[DB_ERROR] Failed to update task %s to retrying: %v", taskID, dbErr)
				}
			} else {
				// It has failed permanently.
				if dbErr := s.taskRepo.MarkTaskAsArchived(ctx, taskID, err.Error()); dbErr != nil {
					log.Printf("[DB_ERROR] Failed to update task %s to archived: %v", taskID, dbErr)
				}
			}
		} else {
			// 3. Task succeeded
			log.Printf("[SUCCESS] Task: %s, ID: %s", t.Type(), taskID)
			s.taskRepo.MarkTaskAsCompleted(ctx, taskID, "success")
		}
		return err
	})
}

func loggingMiddleware() asynq.MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			log.Printf("[START] Task: %s, ID: %s", task.Type(), task.ResultWriter().TaskID())
			err := next.ProcessTask(ctx, task)
			if err != nil {
				log.Printf("[FAILED] Task: %s, Error: %v", task.Type(), err)
			} else {
				log.Printf("[SUCCESS] Task: %s", task.Type())
			}
			return err
		})
	}
}

func timingMiddleware() asynq.MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			start := time.Now()
			err := next.ProcessTask(ctx, task)
			duration := time.Since(start)
			log.Printf("[TIMING] Task: %s took %v", task.Type(), duration)
			return err
		})
	}
}

func recoveryMiddleware() asynq.MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) (err error) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[PANIC] Task: %s, Panic: %v", task.Type(), r)
					err = asynq.SkipRetry
				}
			}()
			return next.ProcessTask(ctx, task)
		})
	}
}

// getRetryDelayFunc returns retry delay function based on config
func getRetryDelayFunc(delayType string) func(n int, e error, t *asynq.Task) time.Duration {
	switch delayType {
	case "constant":
		return func(n int, e error, t *asynq.Task) time.Duration {
			return 5 * time.Second // Constant 5 seconds
		}
	case "linear":
		return func(n int, e error, t *asynq.Task) time.Duration {
			return time.Duration(n) * 5 * time.Second // Linear: 5s, 10s, 15s, ...
		}
	default: // exponential
		return func(n int, e error, t *asynq.Task) time.Duration {
			// Exponential backoff: 5s, 10s, 20s, 40s, 80s, capped at 5 minutes
			delay := time.Duration(1<<uint(n)) * 5 * time.Second
			if delay > 5*time.Minute {
				delay = 5 * time.Minute
			}
			return delay
		}
	}
}
