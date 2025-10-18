package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"github.com/rizqitaufiqf/go-scheduler/handler"
	"github.com/rizqitaufiqf/go-scheduler/repository"
)

type NotificationConsumer struct {
	redis         *redis.Client
	repository    repository.NotificationRepository
	sseManager    *handler.SSEManager
	streamName    string
	consumerGroup string
	consumerName  string
}

func NewNotificationConsumer(
	redisClient *redis.Client,
	repository repository.NotificationRepository,
	sseManager *handler.SSEManager,
) *NotificationConsumer {
	return &NotificationConsumer{
		redis:         redisClient,
		repository:    repository,
		sseManager:    sseManager,
		streamName:    "notifications:stream",
		consumerGroup: "notification-processors", // Consumer group name
		consumerName:  "consumer-1",              // Consumer instance name
	}
}

// Start starts consuming from Redis Stream
func (c *NotificationConsumer) Start(ctx context.Context) error {
	// Create consumer group if not exists
	// XGROUP CREATE notifications:stream notification-processors 0 MKSTREAM
	err := c.redis.XGroupCreateMkStream(ctx, c.streamName, c.consumerGroup, "0").Err()
	if err != nil {
		// Ignore error if group already exists
		if err.Error() != "BUSYGROUP Consumer Group name already exists" {
			return err
		}
	}

	log.Printf("🚀 Notification Consumer started")
	log.Printf("   Stream: %s", c.streamName)
	log.Printf("   Consumer Group: %s", c.consumerGroup)
	log.Printf("   Consumer Name: %s", c.consumerName)

	// Start consuming in background
	go c.consumeLoop(ctx)

	return nil
}

// consumeLoop continuously reads and processes messages
func (c *NotificationConsumer) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("⏹️  Notification Consumer stopped")
			return
		default:
			c.processMessages(ctx)
		}
	}
}

// processMessages reads messages from Redis Stream
func (c *NotificationConsumer) processMessages(ctx context.Context) {
	// XREADGROUP GROUP notification-processors consumer-1 BLOCK 5000 COUNT 10 STREAMS notifications:stream >
	// ">" means read only new messages that haven't been delivered to any consumer in the group
	streams, err := c.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.consumerGroup,
		Consumer: c.consumerName,
		Streams:  []string{c.streamName, ">"},
		Count:    10,              // Process up to 10 messages at once
		Block:    5 * time.Second, // Block for 5 seconds waiting for new messages
		NoAck:    false,           // Require explicit acknowledgment
	}).Result()

	if err != nil {
		if err == redis.Nil {
			// No new messages, this is normal
			return
		}
		log.Printf("❌ Failed to read from stream: %v", err)
		time.Sleep(time.Second) // Backoff on error
		return
	}

	// Process each stream (usually just one)
	for _, stream := range streams {
		for _, message := range stream.Messages {
			c.processMessage(ctx, message)
		}
	}
}

// processMessage processes a single notification message
func (c *NotificationConsumer) processMessage(ctx context.Context, msg redis.XMessage) {
	// Extract data from message
	dataStr, ok := msg.Values["data"].(string)
	if !ok {
		log.Printf("❌ Invalid message format: %v", msg.Values)
		c.ackMessage(ctx, msg.ID)
		return
	}

	// Parse notification message
	var notifMsg dto.NotificationMessage
	if err := json.Unmarshal([]byte(dataStr), &notifMsg); err != nil {
		log.Printf("❌ Failed to unmarshal message: %v", err)
		c.ackMessage(ctx, msg.ID)
		return
	}

	log.Printf("📥 Processing notification: MessageID=%s, TaskID=%s, UserID=%s, Type=%s",
		msg.ID, notifMsg.TaskID, notifMsg.UserID, notifMsg.Type)

	// Create notification in database for persistence
	notification := &dto.Notification{
		UserID:  notifMsg.UserID,
		TaskID:  notifMsg.TaskID,
		Type:    notifMsg.Type,
		Title:   notifMsg.Title,
		Message: notifMsg.Message,
		Payload: notifMsg.Payload,
	}

	if err := c.repository.Create(ctx, notification); err != nil {
		log.Printf("❌ Failed to save notification to DB: %v", err)
		// Don't ACK, will retry later
		return
	}

	// Check if user is online (has active SSE connection)
	if c.sseManager.IsUserOnline(notifMsg.UserID) {
		// User is online, send notification immediately via SSE
		response := &dto.NotificationResponse{
			ID:        notification.ID,
			TaskID:    notification.TaskID,
			Type:      notification.Type,
			Title:     notification.Title,
			Message:   notification.Message,
			Payload:   notification.Payload,
			CreatedAt: notification.CreatedAt,
		}

		if err := c.sseManager.SendToUser(notifMsg.UserID, response); err != nil {
			log.Printf("⚠️  Failed to send SSE to user %s: %v", notifMsg.UserID, err)
			// Don't fail, notification is saved in DB
		} else {
			// Mark as sent in database
			if err := c.repository.MarkAsSent(ctx, notification.ID); err != nil {
				log.Printf("⚠️  Failed to mark notification as sent: %v", err)
			}
			log.Printf("📨 Sent notification to online user: UserID=%s, NotificationID=%s", notifMsg.UserID, notification.ID)
		}
	} else {
		// User is offline, notification will be sent when they connect
		log.Printf("💾 User %s is offline, notification saved to DB (ID: %s)",
			notifMsg.UserID, notification.ID)
	}

	// Acknowledge the message (remove from pending list)
	c.ackMessage(ctx, msg.ID)
}

// ackMessage acknowledges a processed message
func (c *NotificationConsumer) ackMessage(ctx context.Context, messageID string) {
	// XACK notifications:stream notification-processors <message-id>
	err := c.redis.XAck(ctx, c.streamName, c.consumerGroup, messageID).Err()
	if err != nil {
		log.Printf("⚠️  Failed to ACK message %s: %v", messageID, err)
	}
}

// CleanupOldMessages removes old messages from stream (run periodically)
func (c *NotificationConsumer) CleanupOldMessages(ctx context.Context) error {
	// XTRIM notifications:stream MAXLEN ~ 10000
	// Keep approximately last 10000 messages
	trimmed, err := c.redis.XTrimMaxLenApprox(ctx, c.streamName, 10000, 0).Result()
	if err != nil {
		return err
	}

	if trimmed > 0 {
		log.Printf("🧹 Cleaned up %d old messages from stream", trimmed)
	}

	return nil
}
