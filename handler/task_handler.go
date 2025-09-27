package handler

import (
	"encoding/json"

	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	dto "github.com/rizqitaufiqf/go-scheduler/dto"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type TaskHandler struct {
	db        *gorm.DB
	scheduler *repo.Scheduler
}

func NewTaskHandler(db *gorm.DB, s *repo.Scheduler) *TaskHandler {
	return &TaskHandler{db: db, scheduler: s}
}

type ScheduleRequest struct {
	ScheduledAt time.Time       `json:"scheduled_at" binding:"required" example:"2025-12-01T15:04:05Z"`
	MaxRetries  *int            `json:"max_retries" example:"5"`
	Priority    *int            `json:"priority" example:"10"`
	Payload     json.RawMessage `json:"payload" binding:"required" swaggertype:"object"`
}

func (h *TaskHandler) schedule(c *gin.Context, entityName, actionName string) {
	var req ScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	// Find Entity and Action IDs from the database
	var entity dto.TaskEntity
	if err := h.db.Where("name = ?", entityName).First(&entity).Error; err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task entity not found: " + entityName})
		return
	}

	var action dto.TaskAction
	if err := h.db.Where("name = ?", actionName).First(&action).Error; err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task action not found: " + actionName})
		return
	}

	task := &dto.TaskScheduler{
		TaskEntityID: entity.ID,
		TaskActionID: action.ID,
		Payload:      datatypes.JSON(req.Payload),
		ScheduledAt:  req.ScheduledAt,
		Status:       dto.StatusPending,
	}

	if req.MaxRetries != nil {
		task.MaxRetries = *req.MaxRetries
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}

	if err := h.scheduler.ScheduleTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to schedule task"})
		return
	}

	c.JSON(http.StatusAccepted, task)
}

// ScheduleCreateProduct schedules a product creation task.
// The payload should be a JSON object matching the dto.Product structure (without the ID).
// Example: {"name": "New Gadget", "price": 99.99, "stock": 100}
// @Summary      Schedule a product creation task
// @Description  Schedules a task to create a new product at a specified time.
// @Tags         Scheduler
// @Accept       json
// @Produce      json
// @Param        task  body      ScheduleRequest  true  "Scheduling Details"
// @Success      202   {object}  dto.TaskScheduler
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /scheduler/products/create [post]
func (h *TaskHandler) ScheduleCreateProduct(c *gin.Context) {
	h.schedule(c, "PRODUCT", "CREATE")
}

// ScheduleUpdateProduct schedules a product update task.
// The payload should be a JSON object matching the dto.Product structure, including the ID of the product to update.
// Example: {"id": "...", "name": "Updated Gadget", "price": 109.99}
// @Summary      Schedule a product update task
// @Description  Schedules a task to update an existing product at a specified time.
// @Tags         Scheduler
// @Accept       json
// @Produce      json
// @Param        task  body      ScheduleRequest  true  "Scheduling Details"
// @Success      202   {object}  dto.TaskScheduler
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /scheduler/products/update [post]
func (h *TaskHandler) ScheduleUpdateProduct(c *gin.Context) {
	h.schedule(c, "PRODUCT", "UPDATE")
}

// ScheduleDeleteProduct schedules a product deletion task.
// The payload should be a JSON object containing the ID of the product to delete.
// Example: {"id": "..."}
// @Summary      Schedule a product deletion task
// @Description  Schedules a task to delete an existing product at a specified time.
// @Tags         Scheduler
// @Accept       json
// @Produce      json
// @Param        task  body      ScheduleRequest  true  "Scheduling Details"
// @Success      202   {object}  dto.TaskScheduler
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /scheduler/products/delete [post]
func (h *TaskHandler) ScheduleDeleteProduct(c *gin.Context) {
	h.schedule(c, "PRODUCT", "DELETE")
}

// GetTasks lists all scheduled tasks, with optional status filtering.
// @Summary      List all scheduled tasks
// @Description  Gets a list of all tasks, with an optional filter by status.
// @Tags         Scheduler
// @Produce      json
// @Param        status  query     string  false  "Filter tasks by status"  Enums(pending, processing, completed, failed, canceled, paused)
// @Success      200     {array}   dto.TaskScheduler
// @Failure      400     {object}  dto.ErrorResponse
// @Failure      500     {object}  dto.ErrorResponse
// @Router       /scheduler/tasks [get]
func (h *TaskHandler) GetTasks(c *gin.Context) {
	status := c.Query("status")

	if status != "" {
		switch dto.TaskStatus(status) {
		case dto.StatusPending, dto.StatusProcessing, dto.StatusCompleted, dto.StatusFailed, dto.StatusCanceled, dto.StatusPaused:
			// Status is valid, proceed.
		default:
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid status query parameter."})
			return
		}
	}

	tasks, err := h.scheduler.FindTasks(status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve tasks"})
		return
	}

	c.JSON(http.StatusOK, tasks)
}

// PauseTask pauses a scheduled task.
// @Summary      Pause a task
// @Description  Pauses a 'pending' task, preventing it from being executed.
// @Tags         Scheduler
// @Produce      json
// @Param        id   path      string  true  "Task ID (UUID)" Format(uuid)
// @Success      200  {object}  dto.TaskScheduler
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse "Task not found or not in 'pending' state"
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /scheduler/tasks/{id}/pause [post]
func (h *TaskHandler) PauseTask(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid task ID"})
		return
	}

	task, err := h.scheduler.PauseTask(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task not found or not in 'pending' state"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to pause task: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// ResumeTask resumes a paused task.
// @Summary      Resume a task
// @Description  Resumes a 'paused' task, making it eligible for execution again.
// @Tags         Scheduler
// @Produce      json
// @Param        id   path      string  true  "Task ID (UUID)" Format(uuid)
// @Success      200  {object}  dto.TaskScheduler
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse "Task not found or not in 'paused' state"
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /scheduler/tasks/{id}/resume [post]
func (h *TaskHandler) ResumeTask(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid task ID"})
		return
	}

	task, err := h.scheduler.ResumeTask(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task not found or not in 'paused' state"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to resume task: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}
