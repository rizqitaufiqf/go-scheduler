package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rizqitaufiqf/go-scheduler/config"
	"github.com/rizqitaufiqf/go-scheduler/database"
	"github.com/rizqitaufiqf/go-scheduler/router"
	"github.com/rizqitaufiqf/go-scheduler/tasks"
	"github.com/rizqitaufiqf/go-scheduler/worker"
	"gorm.io/gorm"
)

// @title Go Task Scheduler API (Asynq)
// @version 2.0
// @description A robust task scheduler using hibiken/asynq
// @host localhost:8080
// @BasePath /api/v1
func main() {
	// 1. Define and parse the --role flag. This is where Go reads the argument.
	role := flag.String("role", "server", "The role of this instance: 'server' for API, 'worker' for task processor.")
	flag.Parse()

	log.Printf("Starting application in '%s' mode", *role)

	// Load configuration
	cfg := config.LoadConfig() // Renamed for clarity

	// Initialize database (untuk audit/reporting, opsional)
	db, err := database.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize Asynq client (untuk enqueue tasks)
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	// 2. Use a switch statement to decide what to run based on the role.
	switch *role {
	case "server":
		// Run migrations only for the server role to avoid race conditions.
		// NOTE: AutoMigrate is convenient for development but not recommended for production.
		// For production environments, use a dedicated migration tool like golang-migrate/migrate
		// to have better control over schema changes and rollbacks.
		if err := database.RunMigrations(db); err != nil {
			log.Fatalf("Failed to run migrations: %v", err)
		}
		runApiServer(cfg, asynqClient, redisOpt, db)
	case "worker":
		runWorker(cfg, asynqClient, redisOpt, db)
	default:
		log.Fatalf("Invalid role specified: %s. Use 'server' or 'worker'.", *role)
	}
}

func runApiServer(cfg *config.Config, asynqClient *asynq.Client, redisOpt asynq.RedisClientOpt, db *gorm.DB) {
	// Wrap asynq client with our custom client
	taskClient := tasks.NewClient(asynqClient, redisOpt, db)
	// Setup HTTP router
	r := router.SetupRouter(taskClient, db, cfg)

	// Start HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port), // Use cfg.Port
		Handler: r,
	}

	go func() {
		log.Printf("Starting HTTP server on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown for the API server
	<-GracefulShutdown()
	log.Println("Shutting down API server...")

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server forced to shutdown: %v", err)
	}
	log.Println("API server stopped.")
}

func runWorker(cfg *config.Config, asynqClient *asynq.Client, redisOpt asynq.RedisClientOpt, db *gorm.DB) {
	// Initialize Asynq server (worker)
	taskClient := tasks.NewClient(asynqClient, redisOpt, db)
	asynqServer := worker.NewServer(redisOpt, cfg, db, taskClient)

	go func() {
		log.Println("Starting Asynq worker...")
		if err := asynqServer.Run(); err != nil {
			log.Fatalf("Failed to start Asynq worker: %v", err)
		}
	}()

	// Graceful shutdown for the worker
	<-GracefulShutdown()
	log.Println("Shutting down Asynq worker...")

	// Shutdown Asynq server (automatically finishes in-progress tasks)
	asynqServer.Shutdown()

	log.Println("Asynq worker stopped.")
}

// GracefulShutdown waits for termination signals (SIGINT, SIGTERM)
func GracefulShutdown() <-chan os.Signal {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	return quit
}
