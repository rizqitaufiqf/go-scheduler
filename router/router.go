package router

import (
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/rizqitaufiqf/go-scheduler/config"
	handler "github.com/rizqitaufiqf/go-scheduler/handler"
	"github.com/rizqitaufiqf/go-scheduler/middleware"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

// RouterConfig holds the dependencies for the router, primarily the handlers.
type RouterConfig struct {
	Cfg                 *config.Config
	Db                  *gorm.DB
	RedisClient         *redis.Client
	AuthHandler         *handler.AuthHandler
	NotificationHandler *handler.NotificationHandler
	WebsocketHandler    *handler.WebSocketHandler
	TaskHandler         *handler.TaskHandler
	ProductHandler      *handler.ProductHandler
}

// SetupRouter configures the application's routes using the provided handlers.
func SetupRouter(router *RouterConfig) *gin.Engine {
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Swagger endpoint
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Main API group
	v1 := r.Group("/api/v1")
	{
		// Group for authentication
		authRoutes := v1.Group("/auth")
		{
			// Simple login endpoint to get a JWT for testing
			authRoutes.POST("/login", router.AuthHandler.Login)
		}

		// Group for direct product management
		productRoutes := v1.Group("/products")
		{
			productRoutes.POST("", router.ProductHandler.CreateProduct)
			productRoutes.GET("", router.ProductHandler.GetProducts)
			productRoutes.PUT("/:id", router.ProductHandler.UpdateProduct)
			productRoutes.DELETE("/:id", router.ProductHandler.DeleteProduct)
		}

		// Group for task scheduling and viewing
		schedulerRoutes := v1.Group("/scheduler")
		{
			schedulerRoutes.Use(middleware.JWTAuth(router.Cfg))
			schedulerRoutes.POST("/tasks", router.TaskHandler.ScheduleGenericTask)
			schedulerRoutes.GET("/tasks", router.TaskHandler.GetTasks)
			schedulerRoutes.GET("/tasks/:id", router.TaskHandler.GetTaskByID)
			schedulerRoutes.POST("/tasks/:id/pause", router.TaskHandler.PauseTask)
			schedulerRoutes.POST("/tasks/:id/resume", router.TaskHandler.ResumeTask)
			schedulerRoutes.POST("/tasks/:id/cancel", router.TaskHandler.CancelTask)
			schedulerRoutes.POST("/tasks/:id/run", router.TaskHandler.RunTaskNow)
			schedulerRoutes.POST("/tasks/:id/retry", router.TaskHandler.RetryFailedTask)
		}

		// Group for notifications
		// These routes are only registered if the notification handler is provided.
		if router.Cfg.NotificationEnabled {
			notifications := v1.Group("/notifications")
			notifications.Use(middleware.JWTAuth(router.Cfg))
			{
				notifications.GET("", router.NotificationHandler.GetNotifications)
				notifications.POST("/read", router.NotificationHandler.MarkAsRead)
				notifications.GET("/unread-count", router.NotificationHandler.GetUnreadCount)
			}
		}

		// WebSocket route
		if router.Cfg.NotificationEnabled {
			r.GET("/ws/notifications", router.WebsocketHandler.HandleWebSocket)
			r.GET("/ws/health", router.WebsocketHandler.HealthCheck)
		}
	}

	return r
}
