package dto

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// TaskStatus defines the status of a scheduled task.
type TaskStatus string

// Defines the possible statuses for a task.
const (
	StatusPending    TaskStatus = "pending"
	StatusPaused     TaskStatus = "paused"
	StatusProcessing TaskStatus = "processing"
	StatusRetrying   TaskStatus = "retrying"
	StatusCompleted  TaskStatus = "completed"
	StatusFailed     TaskStatus = "failed"
	StatusCanceled   TaskStatus = "canceled"
)

// Defines known task entity names.
const (
	TaskEntityProduct = "PRODUCT"
)

// Defines known task action names.
const (
	TaskActionCreate = "CREATE"
	TaskActionUpdate = "UPDATE"
	TaskActionDelete = "DELETE"
)

// TaskScheduler represents a task that is scheduled for future execution.
type TaskScheduler struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	TaskEntityID uuid.UUID      `gorm:"not null" json:"task_entity_id"`
	TaskActionID uuid.UUID      `gorm:"not null" json:"task_action_id"`
	Payload      datatypes.JSON `gorm:"not null" json:"payload" swaggertype:"object"`
	ScheduledAt  time.Time      `gorm:"not null" json:"scheduled_at"`
	Priority     int            `gorm:"not null;default:0" json:"priority"`
	RetryCount   int            `gorm:"not null;default:0" json:"retry_count"`
	MaxRetries   int            `gorm:"not null;default:3" json:"max_retries"`
	Status       TaskStatus     `gorm:"type:varchar(20);not null;default:pending" json:"status" example:"pending"`
	Result       string         `gorm:"type:text" json:"result,omitempty"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	LastErrorAt  *time.Time     `json:"last_error_at,omitempty"`
	CreatedAt    time.Time      `gorm:"default:now()" json:"created_at"`
	CreatedBy    uuid.NullUUID  `json:"created_by,omitempty"`
	UpdatedAt    time.Time      `gorm:"default:now()" json:"updated_at"`
	UpdatedBy    uuid.NullUUID  `json:"updated_by,omitempty"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	DeletedBy    uuid.NullUUID  `json:"deleted_by,omitempty"`

	// Eager loading associations (optional but useful)
	TaskEntity TaskEntity `gorm:"foreignKey:TaskEntityID" json:"task_entity,omitempty"`
	TaskAction TaskAction `gorm:"foreignKey:TaskActionID" json:"task_action,omitempty"`
}

func (TaskScheduler) TableName() string {
	return "public.task_schedulers"
}

// BeforeCreate will set a UUID rather than letting the database generate one.
func (task *TaskScheduler) BeforeCreate(tx *gorm.DB) (err error) {
	if task.ID == uuid.Nil {
		task.ID = uuid.New()
	}
	return
}

// TaskEntity represents the master table for task entities (e.g., PRODUCT, ORDER).
type TaskEntity struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
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
	ID          uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
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

type TaskResponse struct {
	ID             uuid.UUID      `json:"id"`
	TaskEntityName string         `json:"task_entity_name"`
	TaskActionName string         `json:"task_action_name"`
	Payload        datatypes.JSON `json:"payload"`
	ScheduledAt    time.Time      `json:"scheduled_at"`
	Priority       int            `json:"priority"`
	RetryCount     int            `json:"retry_count"`
	MaxRetries     int            `json:"max_retries"`
	Status         TaskStatus     `json:"status"`
	Result         string         `gorm:"type:text" json:"result,omitempty" example:"Success"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	LastErrorAt    *time.Time     `json:"last_error_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	CreatedBy      uuid.NullUUID  `json:"created_by,omitempty"`
	UpdatedAt      time.Time      `json:"updated_at"`
	UpdatedBy      uuid.NullUUID  `json:"updated_by,omitempty"`
	DeletedAt      gorm.DeletedAt `json:"deleted_at,omitempty"`
	DeletedBy      uuid.NullUUID  `json:"deleted_by,omitempty"`
}
