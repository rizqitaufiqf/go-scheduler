package dto

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TaskStatus string

// Defines the possible statuses for a task, aligned with Asynq's lifecycle.
const (
	// Asynq-aligned statuses
	StatusScheduled  TaskStatus = "scheduled"  // Task is scheduled for future execution.
	StatusPending    TaskStatus = "pending"    // Task is in a queue, waiting to be processed.
	StatusProcessing TaskStatus = "processing" // Task is being processed by a worker (maps to Asynq's 'active').
	StatusRetrying   TaskStatus = "retrying"   // Task has failed and is waiting for a retry.
	StatusArchived   TaskStatus = "archived"   // Task has failed all retries and is moved to the archive.
	StatusCompleted  TaskStatus = "completed"  // Task has been processed successfully.

	// Custom application-specific statuses
	StatusCanceled TaskStatus = "canceled" // Task was canceled by a user.
	StatusPaused   TaskStatus = "paused"   // Task is intentionally paused and not in the queue.
)

const (
	TaskEntityProduct = "PRODUCT"
)

// Defines known task action names.
const (
	TaskActionCreate = "CREATE"
	TaskActionUpdate = "UPDATE"
	TaskActionDelete = "DELETE"
)

// JSONB is a custom type for handling JSONB data with GORM
type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONB) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, &j)
}

// TaskScheduler represents the `task_schedulers` table in the database.
type TaskScheduler struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key;" json:"id"`
	TaskEntityID uuid.UUID      `gorm:"type:uuid;not null" json:"task_entity_id"`
	TaskActionID uuid.UUID      `gorm:"type:uuid;not null" json:"task_action_id"`
	Payload      JSONB          `gorm:"type:jsonb;not null" json:"payload"`
	ScheduledAt  time.Time      `gorm:"not null" json:"scheduled_at"`
	Priority     int            `gorm:"not null;default:0" json:"priority"`
	RetryCount   int            `gorm:"not null;default:0" json:"retry_count"`
	MaxRetries   int            `gorm:"not null;default:3" json:"max_retries"`
	Status       TaskStatus     `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Result       string         `gorm:"type:text" json:"result,omitempty"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	LastErrorAt  *time.Time     `json:"last_error_at,omitempty"`
	CreatedAt    time.Time      `gorm:"default:now()" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"default:now()" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (TaskScheduler) TableName() string {
	return "public.task_schedulers"
}

// TaskWithDetails is a helper struct to hold task data along with entity/action names.
type TaskWithDetails struct {
	TaskScheduler
	EntityName string `json:"entity_name"`
	ActionName string `json:"action_name"`
}

// ScheduleTaskRequest is the request body for scheduling a task
type ScheduleTaskRequest struct {
	Entity      string                 `json:"entity" binding:"required" example:"PRODUCT"`
	Action      string                 `json:"action" binding:"required" example:"CREATE"`
	ScheduledAt string                 `json:"scheduled_at" binding:"required" example:"2025-10-20T10:00:00+07:00"`
	Priority    string                 `json:"priority" example:"default"`
	MaxRetries  int                    `json:"max_retries" example:"5"`
	Payload     map[string]interface{} `json:"payload" binding:"required"`
}

// EnqueueTaskRequest is the request body for enqueuing a task immediately
type EnqueueTaskRequest struct {
	Entity     string                 `json:"entity" binding:"required" example:"PRODUCT"`
	Action     string                 `json:"action" binding:"required" example:"CREATE"`
	Priority   string                 `json:"priority" example:"default"`
	MaxRetries int                    `json:"max_retries" example:"5"`
	Payload    map[string]interface{} `json:"payload" binding:"required"`
}

// CreateTaskResponse is the unified response for creating/scheduling a task.
type CreateTaskResponse struct {
	ID          string    `json:"id" example:"01HQX7Z8K9M3N4P5Q6R7S8T9UV"`
	DatabaseID  uuid.UUID `json:"database_id" example:"a1b2c3d4-e5f6-g7h8-i9j0-k1l2m3n4o5p6"`
	Type        string    `json:"type" example:"PRODUCT:CREATE"`
	Queue       string    `json:"queue" example:"default"`
	Status      string    `json:"status" example:"pending"`
	StatusInDB  string    `json:"status_in_db" example:"pending"`
	MaxRetries  int       `json:"max_retries" example:"5"`
	Retried     int       `json:"retried" example:"0"`
	ScheduledAt time.Time `json:"scheduled_at" example:"2025-10-20T10:00:00Z"`
}

// TaskResponse is the response for task operations
type TaskResponse struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Queue       string    `json:"queue"`
	Status      string    `json:"status"`
	MaxRetries  int       `json:"max_retries"`
	Retried     int       `json:"retried"`
	ScheduledAt time.Time `json:"scheduled_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// TaskListResponse is the response for listing tasks
type TaskListResponse struct {
	Tasks []TaskResponse `json:"tasks"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
	Total int            `json:"total"`
}

// QueueStats represents statistics for a single queue.
type QueueStats struct {
	Size      int  `json:"size"`
	Pending   int  `json:"pending"`
	Active    int  `json:"active"`
	Scheduled int  `json:"scheduled"`
	Retry     int  `json:"retry"`
	Archived  int  `json:"archived"`
	Processed int  `json:"processed"`
	Failed    int  `json:"failed"`
	Paused    bool `json:"paused"`
}

// TaskEntity represents the master table for task entities (e.g., PRODUCT, ORDER).
type TaskEntity struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key;" json:"id"`
	Name        string         `gorm:"type:varchar(50);unique;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	CreatedAt   time.Time      `gorm:"default:now()" json:"created_at"`
	CreatedBy   uuid.NullUUID  `json:"created_by,omitempty"`
	UpdatedAt   time.Time      `gorm:"default:now()" json:"updated_at"`
	UpdatedBy   uuid.NullUUID  `json:"updated_by,omitempty"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	DeletedBy   uuid.NullUUID  `json:"deleted_by,omitempty"`
}

func (TaskEntity) TableName() string {
	return "public.task_entities"
}

// TaskAction represents the master table for task actions (e.g., CREATE, UPDATE).
type TaskAction struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key;" json:"id"`
	Name        string         `gorm:"type:varchar(50);unique;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	CreatedAt   time.Time      `gorm:"default:now()" json:"created_at"`
	CreatedBy   uuid.NullUUID  `json:"created_by,omitempty"`
	UpdatedAt   time.Time      `gorm:"default:now()" json:"updated_at"`
	UpdatedBy   uuid.NullUUID  `json:"updated_by,omitempty"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	DeletedBy   uuid.NullUUID  `json:"deleted_by,omitempty"`
}

func (TaskAction) TableName() string {
	return "public.task_actions"
}
