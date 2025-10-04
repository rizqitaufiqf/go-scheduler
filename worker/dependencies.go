package worker

import (
	"github.com/hibiken/asynq"
	"github.com/rizqitaufiqf/go-scheduler/config"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"
	"github.com/rizqitaufiqf/go-scheduler/tasks"
	processor "github.com/rizqitaufiqf/go-scheduler/tasks/processors"
	"gorm.io/gorm"
)

// DependencyContainer holds all the dependencies for the worker.
// This makes it easy to manage repositories and processors.
type DependencyContainer struct {
	TaskRepo                repo.TaskRepository
	ProductProcessor        *processor.ProductProcessor
	ReconciliationProcessor *processor.ReconciliationProcessor
	// Jika Anda punya processor lain, tambahkan di sini:
	// OrderProcessor *tasks.OrderProcessor
	// UserProcessor  *tasks.UserProcessor
}

// NewDependencyContainer initializes all repositories and processors.
// When you add a new domain (e.g., Order), you initialize its repo and processor here.
func NewDependencyContainer(db *gorm.DB, client *tasks.Client, cfg *config.Config) *DependencyContainer {
	// Initialize Repositories
	productRepo := repo.NewProductRepository(db)
	taskRepo := repo.NewTaskRepository(db)
	// orderRepo := repository.NewOrderRepository(db)
	// userRepo := repository.NewUserRepository(db)

	// Initialize Processors
	productProcessor := processor.NewProductProcessor(productRepo)
	reconciliationProcessor := processor.NewReconciliationProcessor(taskRepo, client, cfg)
	// orderProcessor := tasks.NewOrderProcessor(orderRepo)
	// userProcessor := tasks.NewUserProcessor(userRepo)

	return &DependencyContainer{
		TaskRepo:                taskRepo,
		ProductProcessor:        productProcessor,
		ReconciliationProcessor: reconciliationProcessor,
		// OrderProcessor: orderProcessor,
		// UserProcessor:  userProcessor,
	}
}

// RegisterAllHandlers registers all task handlers from the container to the mux.
func (c *DependencyContainer) RegisterAllHandlers(mux *asynq.ServeMux, scheduler *asynq.Scheduler) {
	// Register Product Handlers
	mux.HandleFunc(tasks.TypeProductCreate, c.ProductProcessor.ProcessProductCreate)
	mux.HandleFunc(tasks.TypeProductUpdate, c.ProductProcessor.ProcessProductUpdate)
	mux.HandleFunc(tasks.TypeProductDelete, c.ProductProcessor.ProcessProductDelete)

	// Register System Handlers
	mux.HandleFunc(tasks.TypeSystemReconcile, c.ReconciliationProcessor.ProcessReconciliationTask)

	// Register Periodic Tasks
	// Run reconciliation task every 10 minutes.
	scheduler.Register("@every 1m", asynq.NewTask(tasks.TypeSystemReconcile, nil), asynq.Queue("low"))

	// c.OrderProcessor.RegisterHandlers(mux)
	// c.UserProcessor.RegisterHandlers(mux)
}
