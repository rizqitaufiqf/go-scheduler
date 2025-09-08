package handler

import (
	"encoding/json"

	dto "github.com/rizqitaufiqf/go-scheduler/dto"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"

	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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
	Payload     json.RawMessage `json:"payload" binding:"required" swaggertype:"object"`
}

func (h *TaskHandler) schedule(c *gin.Context, taskType string) {
	var req ScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	task := &dto.ScheduledTask{
		TaskType:    taskType,
		Payload:     datatypes.JSON(req.Payload),
		ScheduledAt: req.ScheduledAt,
		Status:      dto.StatusPending,
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
// @Success      202   {object}  dto.ScheduledTask
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /scheduler/products/create [post]
func (h *TaskHandler) ScheduleCreateProduct(c *gin.Context) {
	h.schedule(c, dto.TaskTypeProductCreate)
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
// @Success      202   {object}  dto.ScheduledTask
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /scheduler/products/update [post]
func (h *TaskHandler) ScheduleUpdateProduct(c *gin.Context) {
	h.schedule(c, dto.TaskTypeProductUpdate)
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
// @Success      202   {object}  dto.ScheduledTask
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /scheduler/products/delete [post]
func (h *TaskHandler) ScheduleDeleteProduct(c *gin.Context) {
	h.schedule(c, dto.TaskTypeProductDelete)
}

// GetTasks lists all scheduled tasks, with optional status filtering.
// @Summary      List all scheduled tasks
// @Description  Gets a list of all tasks, with an optional filter by status.
// @Tags         Scheduler
// @Produce      json
// @Param        status  query     string  false  "Filter tasks by status"  Enums(pending, processing, completed, failed)
// @Success      200     {array}   dto.ScheduledTask
// @Failure      400     {object}  dto.ErrorResponse
// @Failure      500     {object}  dto.ErrorResponse
// @Router       /scheduler/tasks [get]
func (h *TaskHandler) GetTasks(c *gin.Context) {
	status := c.Query("status")

	var tasks []dto.ScheduledTask
	query := h.db.Order("created_at desc")

	if status != "" {
		switch dto.TaskStatus(status) {
		case dto.StatusPending, dto.StatusProcessing, dto.StatusCompleted, dto.StatusFailed:
			query = query.Where("status = ?", status)
		default:
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid status query parameter. Use one of: pending, processing, completed, failed."})
			return
		}
	}

	if err := query.Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve tasks"})
		return
	}

	c.JSON(http.StatusOK, tasks)
}
