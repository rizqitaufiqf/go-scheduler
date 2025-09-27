package router

import (
	handler "github.com/rizqitaufiqf/go-scheduler/handler"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB, s *repo.Scheduler) *gin.Engine {
	r := gin.Default()

	// Create repositories
	productRepo := repo.NewProductRepository(db)

	// Create handlers
	productHandler := handler.NewProductHandler(productRepo)
	taskHandler := handler.NewTaskHandler(db, s)

	// Swagger endpoint
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Group for direct product management
	productRoutes := r.Group("/products")
	{
		productRoutes.POST("", productHandler.CreateProduct)
		productRoutes.GET("", productHandler.GetProducts)
		productRoutes.PUT("/:id", productHandler.UpdateProduct)
		productRoutes.DELETE("/:id", productHandler.DeleteProduct)
	}

	// Group for task scheduling and viewing
	schedulerRoutes := r.Group("/scheduler")
	{
		// Schedule operations
		schedulerRoutes.POST("/products/create", taskHandler.ScheduleCreateProduct)
		schedulerRoutes.POST("/products/update", taskHandler.ScheduleUpdateProduct)
		schedulerRoutes.POST("/products/delete", taskHandler.ScheduleDeleteProduct)

		// View tasks
		schedulerRoutes.GET("/tasks", taskHandler.GetTasks)

		// Manage task state
		schedulerRoutes.POST("/tasks/:id/pause", taskHandler.PauseTask)
		schedulerRoutes.POST("/tasks/:id/resume", taskHandler.ResumeTask)
	}

	return r
}
