package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NotificationStatus represents the status of a notification
type NotificationStatus string

const (
	NotificationStatusPending NotificationStatus = "pending"
	NotificationStatusSent    NotificationStatus = "sent"
	NotificationStatusRead    NotificationStatus = "read"
)

// NotificationType represents the type of notification
type NotificationType string

const (
	NotificationTypeTaskCompleted NotificationType = "task_completed"
	NotificationTypeTaskFailed    NotificationType = "task_failed"
)

// Notification represents a notification in the database
type Notification struct {
	ID        uuid.UUID          `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID    uuid.UUID          `gorm:"not null;index" json:"user_id"`
	TaskID    uuid.UUID          `gorm:"not null;index" json:"task_id"`
	Type      NotificationType   `gorm:"type:varchar(50);not null" json:"type"`
	Title     string             `gorm:"type:varchar(255);not null" json:"title"`
	Message   string             `gorm:"type:text;not null" json:"message"`
	Status    NotificationStatus `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Payload   json.RawMessage    `gorm:"type:jsonb" json:"payload,omitempty"`
	SentAt    *time.Time         `json:"sent_at,omitempty"`
	ReadAt    *time.Time         `json:"read_at,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	DeletedAt gorm.DeletedAt     `gorm:"index" json:"-"`
}

// NotificationMessage represents the message structure in Redis Stream
type NotificationMessage struct {
	UserID  uuid.UUID        `json:"user_id"`
	TaskID  uuid.UUID        `json:"task_id"`
	Type    NotificationType `json:"type"`
	Title   string           `json:"title"`
	Message string           `json:"message"`
	Payload json.RawMessage  `json:"payload,omitempty"`
}

// NotificationResponse represents the response sent via SSE to frontend
type NotificationResponse struct {
	ID        uuid.UUID          `json:"id"`
	TaskID    uuid.UUID          `json:"task_id"`
	Type      NotificationType   `json:"type"`
	Title     string             `json:"title"`
	Message   string             `json:"message"`
	Status    NotificationStatus `json:"status"`
	Payload   json.RawMessage    `json:"payload,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

// TableName specifies the table name for GORM
func (Notification) TableName() string {
	return "notifications"
}
