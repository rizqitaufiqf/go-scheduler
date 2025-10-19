package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rizqitaufiqf/go-scheduler/config"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationRepository struct {
	db    *gorm.DB
	redis *redis.Client
	cfg   *config.Config
}

func NewNotificationRepository(db *gorm.DB, redisClient *redis.Client, cfg *config.Config) *NotificationRepository {
	return &NotificationRepository{
		db:    db,
		redis: redisClient,
		cfg:   cfg,
	}
}

// Create notification
func (r *NotificationRepository) Create(ctx context.Context, notification *dto.Notification) error {
	if notification.ExpiresAt == nil {
		expiresAt := time.Now().AddDate(0, 0, r.cfg.NotificationRetentionDays)
		notification.ExpiresAt = &expiresAt
	}

	return r.db.WithContext(ctx).Create(notification).Error
}

// Get notifications for user
func (r *NotificationRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]dto.Notification, int64, error) {
	var notifications []dto.Notification
	var total int64

	query := r.db.WithContext(ctx).Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now())

	if err := query.Model(&dto.Notification{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&notifications).Error
	return notifications, total, err
}

// Get unread count for user and session
func (r *NotificationRepository) GetUnreadCount(ctx context.Context, userID, sessionID string) (int64, error) {
	var count int64

	subQuery := r.db.WithContext(ctx).
		Table("notification_read_statuses").
		Select("notification_id").
		Where("user_id = ?", userID)

	err := r.db.WithContext(ctx).
		Model(&dto.Notification{}).
		Preload(clause.Associations).
		Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, time.Now()).
		Where("id NOT IN (?)", subQuery).
		Count(&count).Error

	return count, err
}

// Mark as read
func (r *NotificationRepository) MarkAsRead(ctx context.Context, notificationID, sessionID, userID string) error {
	readStatus := &dto.NotificationReadStatus{
		NotificationID: notificationID,
		UserID:         userID,
	}

	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "notification_id"}, {Name: "user_id"}},
			DoNothing: true,
		}).
		Create(readStatus).Error
}

// IsRead checks if a notification has been read by the user associated with the session.
func (r *NotificationRepository) IsRead(ctx context.Context, notificationID, sessionID string) (bool, error) {
	var count int64
	// The logic is now user-centric. We find the user_id from the session_id and check against that.
	err := r.db.WithContext(ctx).
		Model(&dto.NotificationReadStatus{}).
		Where("notification_id = ? AND user_id = (SELECT user_id FROM sessions WHERE id = ? LIMIT 1)", notificationID, sessionID).
		Count(&count).Error

	return count > 0, err
}

// IsReadByUser checks if a notification has been read by a user in ANY session.
// This is used for the hybrid logic to avoid showing old, already-seen notifications as unread on new sessions.
func (r *NotificationRepository) IsReadByUser(ctx context.Context, notificationID, userID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&dto.NotificationReadStatus{}).
		Where("notification_id = ? AND user_id = ?", notificationID, userID).
		Count(&count).Error

	return count > 0, err
}

// Session Management (Redis)
func (r *NotificationRepository) SaveSessionRedis(ctx context.Context, sessionID, userID string, ttl time.Duration) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return r.redis.Set(ctx, key, userID, ttl).Err()
}

func (r *NotificationRepository) GetSessionRedis(ctx context.Context, sessionID string) (string, error) {
	key := fmt.Sprintf("session:%s", sessionID)
	return r.redis.Get(ctx, key).Result()
}

func (r *NotificationRepository) DeleteSessionRedis(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return r.redis.Del(ctx, key).Err()
}

// Session Management (PostgreSQL)
func (r *NotificationRepository) SaveSessionPostgres(ctx context.Context, session *dto.Session) error {
	return r.db.WithContext(ctx).Save(session).Error
}

func (r *NotificationRepository) GetSessionPostgres(ctx context.Context, sessionID string) (*dto.Session, error) {
	var session dto.Session
	err := r.db.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *NotificationRepository) DeleteSessionPostgres(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).Delete(&dto.Session{}, "id = ?", sessionID).Error
}

// Cleanup expired notifications
func (r *NotificationRepository) CleanupExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&dto.Notification{}).Error
}
