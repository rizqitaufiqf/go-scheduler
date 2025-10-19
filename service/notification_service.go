package service

import (
	"context"
	"log"
	"time"

	"github.com/rizqitaufiqf/go-scheduler/config"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"github.com/rizqitaufiqf/go-scheduler/repository"
)

type NotificationService struct {
	repo   *repository.NotificationRepository
	pubsub *PubSubService
	wsHub  *WebSocketHub
	cfg    *config.Config
}

func NewNotificationService(
	repo *repository.NotificationRepository,
	pubsub *PubSubService,
	wsHub *WebSocketHub,
	cfg *config.Config,
) *NotificationService {
	return &NotificationService{
		repo:   repo,
		pubsub: pubsub,
		wsHub:  wsHub,
		cfg:    cfg,
	}
}

// Create and send notification
func (s *NotificationService) CreateAndSend(ctx context.Context, notification *dto.Notification) error {
	// Save to database
	if err := s.repo.Create(ctx, notification); err != nil {
		log.Printf("[Notification] Error saving to DB: %v", err)
		return err
	}

	log.Printf("[Notification] Created: ID=%s, UserID=%s, TaskID=%s", notification.ID, notification.UserID, notification.TaskID)

	// Publish to Redis Pub/Sub
	if err := s.pubsub.Publish(ctx, notification); err != nil {
		log.Printf("[Notification] Error publishing to Redis: %v", err)
		return err
	}

	log.Printf("[Notification] Published to Redis channel: %s", s.cfg.NotificationRedisChannel)

	return nil
}

// Handle incoming notification from Pub/Sub
func (s *NotificationService) HandleNotification(ctx context.Context, notification *dto.Notification) {
	// Check if user is online
	if !s.wsHub.IsUserOnline(notification.UserID) {
		log.Printf("[Notification] User %s is offline, notification stored for later", notification.UserID)
		return
	}

	// Send via WebSocket to all user's sessions
	wsMsg := dto.WSMessage{
		Type: dto.WSMessageTypeNotification,
		Data: dto.WSNotificationMessage{
			Notification: dto.NotificationResponse{
				Notification: *notification,
				IsRead:       false,
			},
		},
		Timestamp: time.Now().Unix(),
	}

	s.wsHub.SendToUser(notification.UserID, &wsMsg)
	log.Printf("[Notification] Sent to user %s via WebSocket", notification.UserID)
}

// Get user notifications with read status
func (s *NotificationService) GetUserNotifications(ctx context.Context, userID, sessionID string, limit, offset int, unreadOnly bool) (*dto.NotificationListResponse, error) {
	notifications, total, err := s.repo.GetByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}

	var response []dto.NotificationResponse
	var unreadCount int64

	for _, notif := range notifications {
		// With user-centric read status, the logic is simpler.
		// We just check if the user has read the notification.
		isRead, _ := s.repo.IsReadByUser(ctx, notif.ID, userID)

		// If the caller only wants unread notifications, and we've determined this one is read, skip it.
		if unreadOnly && isRead {
			continue
		}

		// Count unread items for the final response.
		if !isRead {
			unreadCount++
		}

		response = append(response, dto.NotificationResponse{
			Notification: notif,
			IsRead:       isRead,
		})
	}

	return &dto.NotificationListResponse{
		Notifications: response,
		Total:         total,
		Unread:        unreadCount,
	}, nil
}

// Mark notifications as read
func (s *NotificationService) MarkAsRead(ctx context.Context, notificationIDs []string, sessionID, userID string) error {
	for _, id := range notificationIDs {
		if err := s.repo.MarkAsRead(ctx, id, sessionID, userID); err != nil {
			log.Printf("[Notification] Error marking as read: %v", err)
			continue
		} else {
			// On success, broadcast the read status update to all of the user's sessions.
			s.broadcastReadUpdate(userID, id)
		}
	}
	return nil
}

// broadcastReadUpdate sends a WebSocket message to all sessions of a user
// to inform them that a notification has been marked as read.
func (s *NotificationService) broadcastReadUpdate(userID, notificationID string) {
	wsMsg := &dto.WSMessage{
		Type:      dto.WSMessageTypeNotificationRead,
		Timestamp: time.Now().Unix(),
		Data:      map[string]interface{}{"notification_id": notificationID},
	}
	s.wsHub.SendToUser(userID, wsMsg)
	log.Printf("[Notification] Broadcasted read update for notif %s to user %s", notificationID, userID)
}

// Get unread count
func (s *NotificationService) GetUnreadCount(ctx context.Context, userID, sessionID string) (int64, error) {
	return s.repo.GetUnreadCount(ctx, userID, sessionID)
}

// Save session
func (s *NotificationService) SaveSession(ctx context.Context, sessionID, userID, deviceInfo, ipAddress string) error {
	ttl := time.Hour * time.Duration(s.cfg.JWTExpiryHours)

	if s.cfg.NotificationUsePostgresSession {
		expiresAt := time.Now().Add(ttl)
		session := &dto.Session{
			ID:         sessionID,
			UserID:     userID,
			DeviceInfo: deviceInfo,
			IPAddress:  ipAddress,
			ExpiresAt:  &expiresAt,
		}
		return s.repo.SaveSessionPostgres(ctx, session)
	}

	return s.repo.SaveSessionRedis(ctx, sessionID, userID, ttl)
}

// Delete session
func (s *NotificationService) DeleteSession(ctx context.Context, sessionID string) error {
	if s.cfg.NotificationUsePostgresSession {
		return s.repo.DeleteSessionPostgres(ctx, sessionID)
	}
	return s.repo.DeleteSessionRedis(ctx, sessionID)
}

// Start cleanup job
func (s *NotificationService) StartCleanupJob(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	log.Println("[Notification] Cleanup job started")

	for {
		select {
		case <-ctx.Done():
			log.Println("[Notification] Cleanup job stopped")
			return
		case <-ticker.C:
			if err := s.repo.CleanupExpired(ctx); err != nil {
				log.Printf("[Notification] Cleanup error: %v", err)
			} else {
				log.Println("[Notification] Expired notifications cleaned up")
			}
		}
	}
}
