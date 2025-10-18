package router

import (
	handler "github.com/rizqitaufiqf/go-scheduler/handler"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

func SetupRouter(
	db *gorm.DB,
	scheduler *repo.Scheduler,
) (*gin.Engine, repo.NotificationRepository, *handler.SSEManager) {
	r := gin.Default()

	// Create repositories
	productRepo := repo.NewProductRepository(db)
	notificationRepo := repo.NewNotificationRepository(db) // This will be returned

	// Create handlers
	productHandler := handler.NewProductHandler(productRepo)
	taskHandler := handler.NewTaskHandler(db, scheduler)
	sseManager := handler.GetSSEManager() // This will be returned
	sseHandler := handler.NewSSEHandler(notificationRepo, sseManager)

	// Swagger endpoint
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// CORS middleware for SSE support
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Main API group
	apiV1 := r.Group("/api/v1")

	// Group for direct product management
	productRoutes := apiV1.Group("/products")
	{
		productRoutes.POST("", productHandler.CreateProduct)
		productRoutes.GET("", productHandler.GetProducts)
		productRoutes.PUT("/:id", productHandler.UpdateProduct)
		productRoutes.DELETE("/:id", productHandler.DeleteProduct)
	}

	// Group for task scheduling and viewing
	schedulerRoutes := apiV1.Group("/scheduler")
	{
		// Schedule operations
		schedulerRoutes.POST("/tasks", taskHandler.ScheduleGenericTask)

		// View tasks
		schedulerRoutes.GET("/tasks", taskHandler.GetTasks)
		schedulerRoutes.GET("/tasks/:id", taskHandler.GetTaskByID)

		// Manage task state
		schedulerRoutes.POST("/tasks/:id/pause", taskHandler.PauseTask)
		schedulerRoutes.POST("/tasks/:id/resume", taskHandler.ResumeTask)
		schedulerRoutes.POST("/tasks/:id/cancel", taskHandler.CancelTask)
		schedulerRoutes.POST("/tasks/:id/run", taskHandler.RunTaskNow)
		schedulerRoutes.POST("/tasks/:id/retry", taskHandler.RetryFailedTask)
	}

	notifications := apiV1.Group("/notifications")
	{
		// SSE endpoint - establishes long-lived connection
		notifications.GET("/stream", sseHandler.HandleSSE)

		// REST endpoints for notifications
		notifications.GET("", sseHandler.GetNotifications)    // Get user's notifications
		notifications.PUT("/:id/read", sseHandler.MarkAsRead) // Mark notification as read
		notifications.GET("/stats", sseHandler.GetStats)      // Get SSE statistics
	}

	return r, notificationRepo, sseManager
}
