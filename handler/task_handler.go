package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	dto "github.com/rizqitaufiqf/go-scheduler/dto"
	repo "github.com/rizqitaufiqf/go-scheduler/repository"
	"gorm.io/gorm"
)

type TaskHandler struct {
	db        *gorm.DB
	scheduler *repo.Scheduler
}

func NewTaskHandler(db *gorm.DB, s *repo.Scheduler) *TaskHandler {
	return &TaskHandler{db: db, scheduler: s}
}

// ScheduleGenericTask schedules any valid task based on the provided entity and action names.
// @Summary      Schedule a generic task
// @Description  Schedules any valid task by providing entity, action, and payload in the request body.
// @Tags         Scheduler
// @Accept       json
// @Produce      json
// @Param        task  body      dto.GenericScheduleRequest  true  "Generic Task Scheduling Details"
// @Success      202   {object}  dto.TaskScheduler
// @Failure      400   {object}  dto.ErrorResponse
// @Failure      404   {object}  dto.ErrorResponse "If entity or action is not found"
// @Failure      500   {object}  dto.ErrorResponse
// @Router       /scheduler/tasks [post]
func (h *TaskHandler) ScheduleGenericTask(c *gin.Context) {
	var req dto.GenericScheduleRequest
	// 1. Bind and validate the incoming JSON request against the DTO.
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	// Validate entity & action from master tables
	entity, err := h.scheduler.FindEntityByName(req.Entity)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task entity not found: " + req.Entity})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to validate entity: " + err.Error()})
		return
	}
	action, err := h.scheduler.FindActionByName(req.Action)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task action not found: " + req.Action})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to validate action: " + err.Error()})
		return
	}

	// 3. Determine the scheduled time, defaulting to now if not provided.
	if req.ScheduledAt.IsZero() {
		req.ScheduledAt = time.Now()
	}

	// 4. Build the core task model from the validated request data.
	task := &dto.TaskScheduler{
		TaskEntityID: entity.ID,
		TaskActionID: action.ID,
		Payload:      req.Payload,
		ScheduledAt:  req.ScheduledAt,
		Status:       dto.StatusPending,
		CreatedBy:    req.UserID,
	}
	// Apply optional parameters if they were provided.
	if req.MaxRetries != nil {
		task.MaxRetries = *req.MaxRetries
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}

	// 5. Determine the idempotency key.
	// First, respect the standard 'Idempotency-Key' header if the client provides it.
	dedupKey := c.GetHeader("Idempotency-Key")
	if dedupKey == "" {
		// If no header is present, generate a key automatically from the request content.
		var temp map[string]any
		// The best-effort key is based on the 'id' field within the payload, common for updates/deletes.
		_ = json.Unmarshal(req.Payload, &temp)
		if id, ok := temp["id"].(string); ok && id != "" {
			dedupKey = fmt.Sprintf("%s:%s:%s", req.Entity, req.Action, id)
		} else {
			sum := sha256.Sum256(req.Payload)
			dedupKey = fmt.Sprintf("%s:%s:%x", req.Entity, req.Action, sum)
		}
	}

	// 6. Define TTLs for the two-stage idempotency lock.
	const (
		// pendingTTL is a short lock placed while the request is in-flight.
		pendingTTL = 30 * time.Second
		// finalTTL is the long-term lock placed after the task is successfully created.
		finalTTL = 24 * time.Hour
	)

	// 7. Schedule the task using the idempotent repository method.
	resp, created, inFlight, err := h.scheduler.ScheduleTask(task, dedupKey, pendingTTL, finalTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to schedule: " + err.Error()})
		return
	}

	// 8. Handle the response based on the outcome of the idempotent operation.
	switch {
	case inFlight:
		// Another identical request is currently reserving or finalizing.
		// The client should wait and retry later (e.g., by polling the task ID).
		c.JSON(http.StatusAccepted, gin.H{
			"idempotent": true,
			"in_flight":  true,
			"message":    "Identical request is being processed",
		})
		return

	case created:
		// This is the first request — the task has been created and enqueued successfully.
		// Return HTTP 202 Accepted with task details.
		c.JSON(http.StatusAccepted, resp)
		return

	default:
		// Duplicate request for an existing task (T:<task_id>).
		// If the response is nil, it's likely still being prepared — respond as in-flight.
		if resp == nil {
			c.JSON(http.StatusAccepted, gin.H{
				"idempotent": true,
				"in_flight":  true,
				"message":    "Task is being prepared",
			})
			return
		}

		// If the task is in a terminal state (completed, failed, canceled) → return 200 + task details.
		// If it's still pending/processing/retrying → return 202 Accepted with in-flight info.
		switch resp.Status {
		case dto.StatusCompleted, dto.StatusFailed, dto.StatusCanceled:
			c.JSON(http.StatusOK, resp)
		default: // pending / processing / retrying
			c.JSON(http.StatusAccepted, gin.H{
				"idempotent": true,
				"in_flight":  true,
				"task_id":    resp.ID,
				"status":     resp.Status,
			})
		}
	}
}

// GetTasks lists all scheduled tasks, with optional status filtering.
// @Summary      List all scheduled tasks
// @Description  Gets a list of all tasks, with an optional filter by status.
// @Tags         Scheduler
// @Produce      json
// @Param        status  query     string  false  "Filter tasks by status"  Enums(pending, processing, completed, failed, canceled, paused)
// @Success      200     {array}   dto.TaskResponse
// @Failure      400     {object}  dto.ErrorResponse
// @Failure      500     {object}  dto.ErrorResponse
// @Router       /scheduler/tasks [get]
func (h *TaskHandler) GetTasks(c *gin.Context) {
	status := c.Query("status")

	if status != "" {
		switch dto.TaskStatus(status) {
		case dto.StatusPending, dto.StatusProcessing, dto.StatusCompleted, dto.StatusFailed, dto.StatusCanceled, dto.StatusPaused, dto.StatusRetrying:
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

// GetTaskByID retrieves a single task by its ID.
// @Summary      Get a single task
// @Description  Gets the full details of a single scheduled task by its ID.
// @Tags         Scheduler
// @Produce      json
// @Param        id   path      string  true  "Task ID (UUID)"
// @Success      200  {object}  dto.TaskResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse "Task not found"
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /scheduler/tasks/{id} [get]
func (h *TaskHandler) GetTaskByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid task ID"})
		return
	}

	task, err := h.scheduler.FindTaskByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve task: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

// PauseTask pauses a scheduled task.
// @Summary      Pause a task
// @Description  Pauses a 'pending' task, preventing it from being executed.
// @Tags         Scheduler
// @Produce      json
// @Param        id   path      string  true  "Task ID (UUID)"
// @Success      200  {object}  dto.TaskResponse
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

// RunTaskNow manually triggers a task to run as soon as possible.
// @Summary      Run a task now
// @Description  Manually triggers a 'pending' or 'paused' task to be executed immediately by the next available worker.
// @Tags         Scheduler
// @Produce      json
// @Param        id   path      string  true  "Task ID (UUID)"
// @Success      200  {object}  dto.TaskResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse "Task not found or not in a runnable state ('pending' or 'paused')"
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /scheduler/tasks/{id}/run [post]
func (h *TaskHandler) RunTaskNow(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid task ID"})
		return
	}

	task, err := h.scheduler.RunTaskNow(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task not found or not in a runnable state ('pending' or 'paused')"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to run task: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// ResumeTask resumes a paused task.
// @Summary      Resume a task
// @Description  Resumes a 'paused' task, making it eligible for execution again.
// @Tags         Scheduler
// @Produce      json
// @Param        id   path      string  true  "Task ID (UUID)"
// @Success      200  {object}  dto.TaskResponse
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

// CancelTask cancels a scheduled task.
// @Summary      Cancel a task
// @Description  Cancels a 'pending' or 'paused' task, preventing it from being executed.
// @Tags         Scheduler
// @Produce      json
// @Param        id   path      string  true  "Task ID (UUID)"
// @Success      200  {object}  dto.TaskResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse "Task not found or not in a cancelable state ('pending' or 'paused')"
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /scheduler/tasks/{id}/cancel [post]
func (h *TaskHandler) CancelTask(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid task ID"})
		return
	}

	task, err := h.scheduler.CancelTask(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task not found or not in a cancelable state ('pending' or 'paused')"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to cancel task: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}

// RetryFailedTask manually re-queues a failed task for execution.
// @Summary      Retry a failed task
// @Description  Resets a 'failed' task to 'pending' and queues it for immediate execution.
// @Tags         Scheduler
// @Produce      json
// @Param        id   path      string  true  "Task ID (UUID)"
// @Success      200  {object}  dto.TaskResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse "Task not found or not in 'failed' state"
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /scheduler/tasks/{id}/retry [post]
func (h *TaskHandler) RetryFailedTask(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid task ID"})
		return
	}

	task, err := h.scheduler.RetryFailedTask(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Task not found or not in 'failed' state"})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retry task: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, task)
}
