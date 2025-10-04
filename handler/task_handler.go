package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rizqitaufiqf/go-scheduler/config"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"github.com/rizqitaufiqf/go-scheduler/repository"
	"github.com/rizqitaufiqf/go-scheduler/tasks"
	"gorm.io/gorm"
)

type TaskHandler struct {
	client   *tasks.Client
	taskRepo repository.TaskRepository
	cfg      *config.Config
}

func NewTaskHandler(client *tasks.Client, db *gorm.DB, cfg *config.Config) *TaskHandler {
	return &TaskHandler{
		client:   client,
		taskRepo: repository.NewTaskRepository(db),
		cfg:      cfg,
	}
}

// ScheduleTask schedules a new task
// @Summary Schedule a new task
// @Description Schedule a task for future execution
// @Tags tasks
// @Accept json
// @Produce json
// @Param task body dto.ScheduleTaskRequest true "Task details"
// @Success 201 {object} dto.TaskResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /scheduler/tasks [post]
func (h *TaskHandler) ScheduleTask(c *gin.Context) {
	var req dto.ScheduleTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set default priority if not provided
	if req.Priority == "" {
		req.Priority = "default"
	}

	// Validate that the queue (priority) exists in the worker's config
	priority, ok := h.cfg.Queues[req.Priority]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid priority: queue does not exist"})
		return
	}

	// 1. Validate Entity and Action against the database
	entity, err := h.taskRepo.FindEntityByName(c.Request.Context(), req.Entity)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity type"})
		return
	}
	action, err := h.taskRepo.FindActionByName(c.Request.Context(), req.Action)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action type"})
		return
	}

	// Parse scheduled time
	scheduledAt, err := time.Parse(time.RFC3339, req.ScheduledAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scheduled_at format"})
		return
	}

	// Generate one UUID to be used for both DB and Asynq Task ID
	taskID := uuid.New()

	// 2. Create the record in `task_schedulers` table first
	dbTask := &dto.TaskScheduler{
		ID:           taskID, // Use the pre-generated UUID
		TaskEntityID: entity.ID,
		TaskActionID: action.ID,
		Payload:      req.Payload,
		ScheduledAt:  scheduledAt,
		MaxRetries:   req.MaxRetries,
		Status:       dto.StatusScheduled, // Set status to scheduled
		Priority:     priority,
	}

	if err := h.taskRepo.CreateTask(c.Request.Context(), dbTask); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save task to database", "details": err.Error()})
		return
	}

	// Build task type from entity and action
	taskType := req.Entity + ":" + req.Action

	// Prepare options
	opts := []asynq.Option{
		asynq.MaxRetry(req.MaxRetries),
		asynq.Queue(req.Priority),
		asynq.TaskID(taskID.String()), // Explicitly set Asynq Task ID
		asynq.Timeout(5 * time.Minute),
	}

	// 3. Enqueue the task to Asynq.
	asynqPayload := make(map[string]interface{})
	for k, v := range req.Payload {
		asynqPayload[k] = v
	}

	info, err := h.client.ScheduleTask(taskType, asynqPayload, scheduledAt, opts...)
	if err != nil {
		// Optional: Rollback or mark the DB task as failed if enqueue fails
		// For now, we just return an error. A more robust solution could involve
		// updating the dbTask status to 'failed_to_enqueue'.
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, dto.CreateTaskResponse{
		ID:          info.ID,
		DatabaseID:  dbTask.ID,
		Type:        info.Type,
		Queue:       info.Queue,
		Status:      info.State.String(),
		StatusInDB:  string(dbTask.Status),
		MaxRetries:  info.MaxRetry,
		Retried:     info.Retried,
		ScheduledAt: info.NextProcessAt,
	})
}

// EnqueueTask enqueues a task for immediate execution
// @Summary Enqueue a task immediately
// @Description Enqueue a task for immediate processing
// @Tags tasks
// @Accept json
// @Produce json
// @Param task body dto.EnqueueTaskRequest true "Task details"
// @Success 201 {object} dto.TaskResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /scheduler/tasks/enqueue [post]
func (h *TaskHandler) EnqueueTask(c *gin.Context) {
	var req dto.EnqueueTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set default priority if not provided
	if req.Priority == "" {
		req.Priority = "default"
	}

	// Validate that the queue (priority) exists in the worker's config
	priority, ok := h.cfg.Queues[req.Priority]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid priority: queue does not exist"})
		return
	}

	// 1. Validate Entity and Action
	entity, err := h.taskRepo.FindEntityByName(c.Request.Context(), req.Entity)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entity type"})
		return
	}
	action, err := h.taskRepo.FindActionByName(c.Request.Context(), req.Action)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action type"})
		return
	}

	// Generate one UUID to be used for both DB and Asynq Task ID
	taskID := uuid.New()

	// 2. Create the record in `task_schedulers` table
	dbTask := &dto.TaskScheduler{
		ID:           taskID, // Use the pre-generated UUID
		TaskEntityID: entity.ID,
		TaskActionID: action.ID,
		Payload:      req.Payload,
		ScheduledAt:  time.Now(), // Enqueue immediately
		MaxRetries:   req.MaxRetries,
		Status:       dto.StatusPending,
		Priority:     priority,
	}

	if err := h.taskRepo.CreateTask(c.Request.Context(), dbTask); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save task to database", "details": err.Error()})
		return
	}

	// Build task type
	taskType := req.Entity + ":" + req.Action

	// Prepare options
	opts := []asynq.Option{
		asynq.MaxRetry(req.MaxRetries),
		asynq.Queue(req.Priority),
		asynq.TaskID(taskID.String()), // Explicitly set Asynq Task ID
	}

	// 3. Enqueue to Asynq.
	// We no longer need to pass db_task_id in the payload.
	// Create a copy of the payload to avoid mutating the original request data.
	asynqPayload := make(map[string]interface{})
	for k, v := range req.Payload {
		asynqPayload[k] = v
	}

	info, err := h.client.EnqueueTask(taskType, asynqPayload, opts...)
	if err != nil {
		// Optional: Rollback or mark the DB task as failed
		// For now, we just return an error.
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, dto.CreateTaskResponse{
		ID:          info.ID,
		DatabaseID:  dbTask.ID,
		Type:        info.Type,
		Queue:       info.Queue,
		Status:      info.State.String(),
		StatusInDB:  string(dbTask.Status),
		MaxRetries:  info.MaxRetry,
		Retried:     info.Retried,
		ScheduledAt: info.NextProcessAt, // Include scheduled time for consistency
	})
}

// GetTask retrieves task information
// @Summary Get task information
// @Description Get detailed information about a task
// @Tags tasks
// @Produce json
// @Param id path string true "Task ID"
// @Success 200 {object} dto.TaskResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /scheduler/tasks/{id} [get]
func (h *TaskHandler) GetTask(c *gin.Context) {
	taskID := c.Param("id")

	// 1. Find the task in the database to get its queue.
	dbTask, err := h.taskRepo.FindTaskByTaskID(c.Request.Context(), taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found in database"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to find task in database", "details": err.Error()})
		return
	}
	queue := h.getQueueNameFromPriority(dbTask.Priority)

	info, err := h.client.GetTaskInfo(queue, taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, dto.TaskResponse{
		ID:          info.ID,
		Type:        info.Type,
		Queue:       info.Queue,
		Status:      info.State.String(),
		MaxRetries:  info.MaxRetry,
		Retried:     info.Retried,
		ScheduledAt: info.NextProcessAt,
	})
}

// ListTasks lists tasks by status
// @Summary List tasks
// @Description List tasks filtered by queue and status
// @Tags tasks
// @Produce json
// @Param queue query string false "Queue name"
// @Param status query string false "Task status (pending, scheduled, retry, archived)"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} dto.TaskListResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /scheduler/tasks [get]
func (h *TaskHandler) ListTasks(c *gin.Context) {
	// NOTE: This implementation lists tasks directly from Redis via Asynq's inspector.
	// This is great for real-time status but is limited to what the inspector provides.
	// An alternative approach is to query the `task_schedulers` table in your PostgreSQL database.
	// Querying the database would allow for more complex filtering, sorting, and joining with other tables (e.g., `task_entities`),
	// providing a richer, albeit potentially slightly delayed, view of the tasks.
	queue := c.DefaultQuery("queue", "default")
	status := c.DefaultQuery("status", string(dto.StatusPending))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	var taskInfos []*asynq.TaskInfo
	var err error

	switch status {
	case string(dto.StatusPending):
		taskInfos, err = h.client.ListPendingTasks(queue, pageSize, page)
	case string(dto.StatusScheduled):
		taskInfos, err = h.client.ListScheduledTasks(queue, pageSize, page)
	case string(dto.StatusRetrying):
		taskInfos, err = h.client.ListRetryTasks(queue, pageSize, page)
	case string(dto.StatusArchived):
		taskInfos, err = h.client.ListArchivedTasks(queue, pageSize, page)
	case string(dto.StatusCompleted):
		taskInfos, err = h.client.ListCompletedTasks(queue, pageSize, page)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tasks := make([]dto.TaskResponse, len(taskInfos))
	for i, info := range taskInfos {
		tasks[i] = dto.TaskResponse{
			ID:          info.ID,
			Type:        info.Type,
			Queue:       info.Queue,
			Status:      info.State.String(),
			MaxRetries:  info.MaxRetry,
			Retried:     info.Retried,
			ScheduledAt: info.NextProcessAt,
		}
	}

	c.JSON(http.StatusOK, dto.TaskListResponse{
		Tasks: tasks,
		Page:  page,
		Size:  pageSize,
		Total: len(tasks),
	})
}

// CancelTask cancels a pending or scheduled task
// @Summary Cancel a task
// @Description Cancel a pending or scheduled task
// @Tags tasks
// @Param id path string true "Task ID"
// @Success 200 {object} dto.MessageResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /scheduler/tasks/{id}/cancel [post]
func (h *TaskHandler) CancelTask(c *gin.Context) {
	taskID := c.Param("id")

	// 1. Find the task in the database to get its queue.
	dbTask, err := h.taskRepo.FindTaskByTaskID(c.Request.Context(), taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found in database"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to find task in database", "details": err.Error()})
		return
	}
	queue := h.getQueueNameFromPriority(dbTask.Priority)

	// 2. Update status in the database to 'canceled'.
	// This makes our DB the source of truth.
	if err := h.taskRepo.UpdateTaskStatus(c.Request.Context(), taskID, dto.StatusCanceled); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found in database"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update task status in database", "details": err.Error()})
		return
	}

	// 3. Attempt to delete the task from Redis.
	if err := h.client.CancelTask(queue, taskID); err != nil {
		// Log the error, but don't return a 500 error to the client
		// because the primary operation (updating the DB) was successful.
		// The reconciliation job can handle any inconsistencies later.
		log.Printf("[WARN] Task %s was marked as 'canceled' in DB, but failed to be deleted from Redis queue '%s': %v", taskID, queue, err)
	}

	c.JSON(http.StatusOK, dto.MessageResponse{Message: "task cancelled successfully"})
}

// PauseTask pauses an individual task
// @Summary Pause an individual task
// @Description Removes a task from the queue and marks it as 'paused' in the database.
// @Tags tasks
// @Param id path string true "Task ID"
// @Success 200 {object} dto.MessageResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /scheduler/tasks/{id}/pause [post]
func (h *TaskHandler) PauseTask(c *gin.Context) {
	taskID := c.Param("id")

	// 1. Find the task in the database to get its queue.
	dbTask, err := h.taskRepo.FindTaskByTaskID(c.Request.Context(), taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found in database"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to find task in database", "details": err.Error()})
		return
	}
	queue := h.getQueueNameFromPriority(dbTask.Priority)

	// 2. Update status in the database to 'paused'.
	if err := h.taskRepo.UpdateTaskStatus(c.Request.Context(), taskID, dto.StatusPaused); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found in database"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update task status to paused", "details": err.Error()})
		return
	}

	// 3. Attempt to delete the task from Redis. This is the "pause" action.
	if err := h.client.CancelTask(queue, taskID); err != nil {
		// Log the error, but the main operation (DB update) was successful.
		// The task is effectively paused from our system's perspective.
		log.Printf("[WARN] Task %s was marked as 'paused' in DB, but failed to be deleted from Redis queue '%s': %v", taskID, queue, err)
	}

	c.JSON(http.StatusOK, dto.MessageResponse{Message: "task paused successfully"})
}

// ResumeTask resumes a paused task
// @Summary Resume a paused task
// @Description Re-enqueues a task that was previously marked as 'paused'.
// @Tags tasks
// @Param id path string true "Task ID"
// @Success 200 {object} dto.CreateTaskResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /scheduler/tasks/{id}/resume [post]
func (h *TaskHandler) ResumeTask(c *gin.Context) {
	taskID := c.Param("id")

	// 1. Find the task in the database and get its full details.
	dbTask, err := h.taskRepo.FindTaskWithDetails(c.Request.Context(), taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found in database"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to find task in database", "details": err.Error()})
		return
	}

	// 2. Check if the task is actually paused.
	if dbTask.Status != dto.StatusPaused {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task is not in 'paused' state", "current_status": dbTask.Status})
		return
	}

	// 3. Re-enqueue the task.
	taskType := dbTask.EntityName + ":" + dbTask.ActionName
	queueName := h.getQueueNameFromPriority(dbTask.Priority)

	opts := []asynq.Option{
		asynq.MaxRetry(dbTask.MaxRetries),
		asynq.Queue(queueName),
		asynq.TaskID(dbTask.ID.String()),
	}

	var info *asynq.TaskInfo
	newStatus := dto.StatusPending

	// Decide whether to schedule it or enqueue it immediately.
	if dbTask.ScheduledAt.After(time.Now()) {
		info, err = h.client.ScheduleTask(taskType, dbTask.Payload, dbTask.ScheduledAt, opts...)
		newStatus = dto.StatusScheduled
	} else {
		info, err = h.client.EnqueueTask(taskType, dbTask.Payload, opts...)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to re-enqueue task", "details": err.Error()})
		return
	}

	// 4. Update the task status back in the database.
	if err := h.taskRepo.UpdateTaskStatus(c.Request.Context(), taskID, newStatus); err != nil {
		log.Printf("[ERROR] Failed to update status for resumed task %s: %v", taskID, err)
		// Don't fail the request, but log this critical inconsistency.
	}

	c.JSON(http.StatusOK, dto.CreateTaskResponse{
		ID:          info.ID,
		DatabaseID:  dbTask.ID,
		Type:        info.Type,
		Queue:       info.Queue,
		Status:      info.State.String(),
		StatusInDB:  string(newStatus),
		MaxRetries:  info.MaxRetry,
		ScheduledAt: info.NextProcessAt,
	})
}

// PauseQueue pauses an entire queue
// @Summary Pause an entire queue
// @Description Stops workers from processing new tasks from the specified queue.
// @Tags queues
// @Param name path string true "Queue name"
// @Success 200 {object} dto.MessageResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /scheduler/queues/{name}/pause [post]
func (h *TaskHandler) PauseQueue(c *gin.Context) {
	queueName := c.Param("name")
	if err := h.client.PauseQueue(queueName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to pause queue", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.MessageResponse{Message: "queue '" + queueName + "' paused successfully"})
}

// ResumeQueue resumes a paused queue
// @Summary Resume a paused queue
// @Description Allows workers to start processing tasks from the specified queue again.
// @Tags queues
// @Param name path string true "Queue name"
// @Success 200 {object} dto.MessageResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /scheduler/queues/{name}/resume [post]
func (h *TaskHandler) ResumeQueue(c *gin.Context) {
	queueName := c.Param("name")
	if err := h.client.ResumeQueue(queueName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resume queue", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.MessageResponse{Message: "queue '" + queueName + "' resumed successfully"})
}

// GetQueueStats retrieves statistics for all queues
// @Summary Get queue statistics
// @Description Get statistics for all queues
// @Tags stats
// @Produce json
// @Success 200 {object} map[string]dto.QueueStats
// @Failure 500 {object} dto.ErrorResponse
// @Router /scheduler/stats/queues [get]
func (h *TaskHandler) GetQueueStats(c *gin.Context) {
	stats, err := h.client.GetQueueStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make(map[string]dto.QueueStats)
	for queue, info := range stats {
		result[queue] = dto.QueueStats{
			Size:      info.Size,
			Pending:   info.Pending,
			Active:    info.Active,
			Scheduled: info.Scheduled,
			Retry:     info.Retry,
			Archived:  info.Archived,
			Processed: info.Processed,
			Failed:    info.Failed,
			Paused:    info.Paused,
		}
	}

	c.JSON(http.StatusOK, result)
}

// HealthCheck returns server health status
// @Summary Health check
// @Description Check if the service is healthy
// @Tags health
// @Produce json
// @Success 200 {object} gin.H
// @Router /health [get]
func (h *TaskHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now(),
	})
}

// getQueueNameFromPriority is a helper to find the queue name string from its integer value.
func (h *TaskHandler) getQueueNameFromPriority(priorityLevel int) string {
	for name, level := range h.cfg.Queues {
		if level == priorityLevel {
			return name
		}
	}
	return "default" // Fallback to default
}
