package service

import (
	"context"
	"encoding/json"
	"log"

	"github.com/go-redis/redis/v8"
	"github.com/rizqitaufiqf/go-scheduler/config"
	"github.com/rizqitaufiqf/go-scheduler/dto"
)

type PubSubService struct {
	redis   *redis.Client
	cfg     *config.Config
	pubsub  *redis.PubSub
	channel string
}

func NewPubSubService(redisClient *redis.Client, cfg *config.Config) *PubSubService {
	return &PubSubService{
		redis:   redisClient,
		cfg:     cfg,
		channel: cfg.NotificationRedisChannel,
	}
}

// Publish notification
func (s *PubSubService) Publish(ctx context.Context, notification *dto.Notification) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	return s.redis.Publish(ctx, s.channel, data).Err()
}

// Subscribe to notifications
func (s *PubSubService) Subscribe(ctx context.Context) <-chan *redis.Message {
	s.pubsub = s.redis.Subscribe(ctx, s.channel)
	return s.pubsub.Channel()
}

// Unsubscribe
func (s *PubSubService) Unsubscribe(ctx context.Context) error {
	if s.pubsub != nil {
		return s.pubsub.Close()
	}
	return nil
}

// Start listening for notifications
func (s *PubSubService) Listen(ctx context.Context, handler func(*dto.Notification)) {
	ch := s.Subscribe(ctx)

	log.Printf("[PubSub] Listening on channel: %s", s.channel)

	for {
		select {
		case <-ctx.Done(): // Stop listening when the context is cancelled.
			log.Println("[PubSub] Context cancelled, stopping listener")
			s.Unsubscribe(ctx)
			return
		case msg := <-ch:
			var notification dto.Notification
			if err := json.Unmarshal([]byte(msg.Payload), &notification); err != nil {
				log.Printf("[PubSub] Error unmarshaling notification: %v", err)
				continue
			}

			handler(&notification)
		}
	}
}
