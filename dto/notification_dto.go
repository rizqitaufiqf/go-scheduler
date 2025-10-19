package dto

import (
	"time"

	"gorm.io/datatypes"
)

// Notification represents a notification record in the database.
type Notification struct {
	ID        string         `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID    string         `json:"user_id" gorm:"index;not null"`
	TaskID    string         `json:"task_id" gorm:"index;not null"`
	Title     string         `json:"title" gorm:"not null"`
	Message   string         `json:"message" gorm:"not null"`
	Type      string         `json:"type" gorm:"default:'task_update'"`
	Metadata  datatypes.JSON `json:"metadata" gorm:"type:jsonb"` // Use datatypes.JSON for proper scanning
	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty" gorm:"index"`
	IsRead    bool           `json:"is_read" gorm:"-"` // Computed field
}

// NotificationReadStatus tracks the read status of a notification per session.
type NotificationReadStatus struct {
	ID             string    `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	NotificationID string    `json:"notification_id" gorm:"index;not null"`
	UserID         string    `json:"user_id" gorm:"index;not null"`
	ReadAt         time.Time `json:"read_at" gorm:"autoCreateTime"`
}

// Session represents a user session for tracking notifications.
type Session struct {
	ID           string     `json:"id" gorm:"primaryKey"`
	UserID       string     `json:"user_id" gorm:"index;not null"`
	DeviceInfo   string     `json:"device_info,omitempty"`
	IPAddress    string     `json:"ip_address,omitempty"`
	CreatedAt    time.Time  `json:"created_at" gorm:"autoCreateTime"`
	LastActiveAt time.Time  `json:"last_active_at" gorm:"autoUpdateTime"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty" gorm:"index"`
}

// Request DTOs
type MarkReadRequest struct {
	NotificationIDs []string `json:"notification_ids" binding:"required"`
}

type GetNotificationsQuery struct {
	Limit  int  `form:"limit" binding:"min=1,max=100"`
	Offset int  `form:"offset" binding:"min=0"`
	Unread bool `form:"unread"`
}

// Response DTOs
type NotificationResponse struct {
	Notification Notification `json:"notification"`
	IsRead       bool         `json:"is_read"`
}

type NotificationListResponse struct {
	Notifications []NotificationResponse `json:"notifications"`
	Total         int64                  `json:"total"`
	Unread        int64                  `json:"unread"`
}
