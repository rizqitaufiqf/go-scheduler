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
	StatusProcessing TaskStatus = "processing"
	StatusCompleted  TaskStatus = "completed"
	StatusFailed     TaskStatus = "failed"

	TaskTypeProductCreate = "PRODUCT_CREATE"
	TaskTypeProductUpdate = "PRODUCT_UPDATE"
	TaskTypeProductDelete = "PRODUCT_DELETE"
)

// ScheduledTask represents a task that is scheduled for future execution.
type ScheduledTask struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key;" json:"id"`
	TaskType    string         `json:"task_type" example:"PRODUCT_CREATE"`
	Payload     datatypes.JSON `json:"payload" swaggertype:"object"`
	Status      TaskStatus     `json:"status" example:"pending"`
	ScheduledAt time.Time      `json:"scheduled_at"`
	Result      string         `gorm:"type:text" json:"result,omitempty" example:"Success"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// BeforeCreate will set a UUID rather than letting the database generate one.
func (task *ScheduledTask) BeforeCreate(tx *gorm.DB) (err error) {
	if task.ID == uuid.Nil {
		task.ID = uuid.New()
	}
	return
}
