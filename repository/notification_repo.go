package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"gorm.io/gorm"
)

type NotificationRepository interface {
	Create(ctx context.Context, notification *dto.Notification) error
	FindPendingByUserID(ctx context.Context, userID uuid.UUID) ([]*dto.Notification, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status dto.NotificationStatus) error
	MarkAsSent(ctx context.Context, id uuid.UUID) error
	MarkAsRead(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*dto.Notification, error)
	FindByUserID(ctx context.Context, userID uuid.UUID, limit int) ([]*dto.Notification, error)
}

type notificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) NotificationRepository {
	return &notificationRepository{db: db}
}

// Create creates a new notification
func (r *notificationRepository) Create(ctx context.Context, notification *dto.Notification) error {
	return r.db.WithContext(ctx).Create(notification).Error
}

// FindPendingByUserID finds all pending notifications for a user
func (r *notificationRepository) FindPendingByUserID(ctx context.Context, userID uuid.UUID) ([]*dto.Notification, error) {
	var notifications []*dto.Notification
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, dto.NotificationStatusPending).
		Order("created_at ASC").
		Find(&notifications).Error
	return notifications, err
}

// FindByUserID finds all notifications for a user (with limit)
func (r *notificationRepository) FindByUserID(ctx context.Context, userID uuid.UUID, limit int) ([]*dto.Notification, error) {
	var notifications []*dto.Notification
	query := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&notifications).Error
	return notifications, err
}

// UpdateStatus updates notification status
func (r *notificationRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status dto.NotificationStatus) error {
	return r.db.WithContext(ctx).
		Model(&dto.Notification{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// MarkAsSent marks notification as sent
func (r *notificationRepository) MarkAsSent(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&dto.Notification{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":  dto.NotificationStatusSent,
			"sent_at": now,
		}).Error
}

// MarkAsRead marks notification as read
func (r *notificationRepository) MarkAsRead(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&dto.Notification{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":  dto.NotificationStatusRead,
			"read_at": now,
		}).Error
}

// FindByID finds a notification by ID
func (r *notificationRepository) FindByID(ctx context.Context, id uuid.UUID) (*dto.Notification, error) {
	var notification dto.Notification
	err := r.db.WithContext(ctx).First(&notification, id).Error
	return &notification, err
}
