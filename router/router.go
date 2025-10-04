package router

import (
	"github.com/gin-gonic/gin"
	"github.com/rizqitaufiqf/go-scheduler/config"
	"github.com/rizqitaufiqf/go-scheduler/docs"
	"github.com/rizqitaufiqf/go-scheduler/handler"
	"github.com/rizqitaufiqf/go-scheduler/repository"
	"github.com/rizqitaufiqf/go-scheduler/tasks"
	"gorm.io/gorm"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// SetupRouter configures and returns the Gin router
func SetupRouter(client *tasks.Client, db *gorm.DB, cfg *config.Config) *gin.Engine {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	r := gin.Default()

	// Initialize handlers
	taskHandler := handler.NewTaskHandler(client, db, cfg)
	productRepo := repository.NewProductRepository(db)
	productHandler := handler.NewProductHandler(productRepo)

	// Swagger documentation
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/docs/doc.json")))

	r.GET("/docs/doc.json", func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Content-Type", "application/json")
		ctx.Writer.WriteHeader(200)
		ctx.Writer.Write([]byte(docs.SwaggerInfo.ReadDoc()))
	})
	// API v1 routes
	v1 := r.Group("/api/v1")
	{
		scheduler := v1.Group("/scheduler")
		{
			// Task operations
			taskRoutes := scheduler.Group("/tasks")
			{
				taskRoutes.POST("", taskHandler.ScheduleTask)
				taskRoutes.POST("/enqueue", taskHandler.EnqueueTask)
				taskRoutes.GET("", taskHandler.ListTasks)
				taskRoutes.GET("/:id", taskHandler.GetTask)
				taskRoutes.POST("/:id/cancel", taskHandler.CancelTask)
				taskRoutes.POST("/:id/pause", taskHandler.PauseTask)
				taskRoutes.POST("/:id/resume", taskHandler.ResumeTask)
			}

			// Queue operations
			queueRoutes := scheduler.Group("/queues")
			{
				queueRoutes.POST("/:name/pause", taskHandler.PauseQueue)
				queueRoutes.POST("/:name/resume", taskHandler.ResumeQueue)
			}

			// Statistics
			scheduler.GET("/stats/queues", taskHandler.GetQueueStats)
		}

		products := v1.Group("/products")
		{
			products.POST("", productHandler.CreateProduct)
			products.GET("", productHandler.GetProducts)
			products.PUT("/:id", productHandler.UpdateProduct)
			products.DELETE("/:id", productHandler.DeleteProduct)
		}
	}

	// Health check
	r.GET("/health", taskHandler.HealthCheck)

	return r
}
