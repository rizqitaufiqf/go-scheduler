package repository

import (
	"context"

	dto "github.com/rizqitaufiqf/go-scheduler/dto"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// Scheduler is responsible for adding tasks to the queue.
type Scheduler struct {
	db    *gorm.DB
	redis *redis.Client
	ctx   context.Context
}

func NewScheduler(db *gorm.DB, redis *redis.Client) *Scheduler {
	return &Scheduler{
		db:    db,
		redis: redis,
		ctx:   context.Background(),
	}
}

// ScheduleTask creates a task in PostgreSQL and adds it to the Redis sorted set.
func (s *Scheduler) ScheduleTask(task *dto.ScheduledTask) error {
	// 1. Save the task to PostgreSQL as the source of truth
	if err := s.db.Create(task).Error; err != nil {
		return err
	}

	// 2. Add the task to Redis sorted set, scored by its execution time.
	// This makes polling for due tasks extremely efficient.
	return s.redis.ZAdd(s.ctx, TasksQueueKey(), &redis.Z{
		Score:  float64(task.ScheduledAt.Unix()),
		Member: task.ID.String(),
	}).Err()
}
