package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/rizqitaufiqf/go-scheduler/dto"
)

type NotificationPublisher struct {
	redis      *redis.Client
	streamName string
}

func NewNotificationPublisher(redisClient *redis.Client) *NotificationPublisher {
	return &NotificationPublisher{
		redis:      redisClient,
		streamName: "notifications:stream",
	}
}

// PublishTaskNotification publishes notification to Redis Stream
func (p *NotificationPublisher) PublishTaskNotification(
	ctx context.Context,
	userID uuid.UUID,
	taskID uuid.UUID,
	notifType dto.NotificationType,
	title string,
	message string,
	payload json.RawMessage,
) error {
	// Create notification message
	notification := dto.NotificationMessage{
		UserID:  userID,
		TaskID:  taskID,
		Type:    notifType,
		Title:   title,
		Message: message,
		Payload: payload,
	}

	// Marshal to JSON
	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}
	// Publish to Redis Stream using XADD
	// XADD notifications:stream MAXLEN ~ 10000 * data <json> user_id <id> task_id <id> type <type>
	result, err := p.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: p.streamName,
		MaxLen: 10,   // Keep only last 10k messages (prevents memory issues)
		Approx: true, // Use approximate trimming (~) for better performance
		Values: map[string]interface{}{
			"data":    string(data),
			"user_id": userID.String(),
			"task_id": taskID.String(),
			"type":    string(notifType),
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to publish to Redis Stream: %w", err)
	}

	log.Printf("📤 Published notification to Redis Stream: ID=%s, TaskID=%s, UserID=%s, Type=%s",
		result, taskID, userID, notifType)

	return nil
}

// PublishTaskCompletedNotification is a helper for task completion
func (p *NotificationPublisher) PublishTaskCompletedNotification(
	ctx context.Context,
	userID uuid.UUID,
	taskID uuid.UUID,
	entity string,
	action string,
) error {
	payloadMap := map[string]interface{}{
		"entity": entity,
		"action": action,
	}
	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return fmt.Errorf("failed to marshal completion payload: %w", err)
	}

	return p.PublishTaskNotification(
		ctx,
		userID,
		taskID,
		dto.NotificationTypeTaskCompleted,
		"Task Completed Successfully",
		fmt.Sprintf("Your task %s %s has been completed successfully", entity, action),
		payloadBytes,
	)
}

// PublishTaskFailedNotification is a helper for task failure
func (p *NotificationPublisher) PublishTaskFailedNotification(
	ctx context.Context,
	userID uuid.UUID,
	taskID uuid.UUID,
	entity string,
	action string,
	errorMessage string,
) error {
	payloadMap := map[string]interface{}{
		"entity": entity,
		"action": action,
		"error":  errorMessage,
	}
	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return fmt.Errorf("failed to marshal failure payload: %w", err)
	}

	return p.PublishTaskNotification(
		ctx,
		userID,
		taskID,
		dto.NotificationTypeTaskFailed,
		"Task Failed",
		fmt.Sprintf("Your task %s %s has failed: %s", entity, action, errorMessage),
		payloadBytes,
	)
}
