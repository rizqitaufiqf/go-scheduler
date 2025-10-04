package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// Client wraps asynq.Client with additional functionality
type Client struct {
	asynqClient *asynq.Client
	redisOpt    asynq.RedisConnOpt
	db          *gorm.DB
}

// NewClient creates a new task client
func NewClient(asynqClient *asynq.Client, redisOpt asynq.RedisConnOpt, db *gorm.DB) *Client {
	return &Client{
		asynqClient: asynqClient,
		redisOpt:    redisOpt,
		db:          db,
	}
}

// EnqueueTask enqueues a task with options
func (c *Client) EnqueueTask(
	taskType string,
	payload interface{},
	opts ...asynq.Option,
) (*asynq.TaskInfo, error) {
	// Marshal payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create asynq task
	task := asynq.NewTask(taskType, payloadBytes)

	// Enqueue task
	info, err := c.asynqClient.Enqueue(task, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}

	return info, nil
}

// ScheduleTask schedules a task for future execution
func (c *Client) ScheduleTask(taskType string, payload interface{}, scheduledAt time.Time, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	// Add ProcessAt option
	opts = append(opts, asynq.ProcessAt(scheduledAt))
	return c.EnqueueTask(taskType, payload, opts...)
}

// EnqueueTaskWithPriority enqueues a task with specific priority
func (c *Client) EnqueueTaskWithPriority(
	taskType string,
	payload interface{},
	priority string, // "critical", "high", "default", "medium", "low", "batch"
	opts ...asynq.Option,
) (*asynq.TaskInfo, error) {
	// Add Queue option
	opts = append(opts, asynq.Queue(priority))
	return c.EnqueueTask(taskType, payload, opts...)
}

// EnqueueUniqueTask enqueues a unique task (prevents duplicates)
func (c *Client) EnqueueUniqueTask(
	taskType string,
	payload interface{},
	uniqueKey string,
	ttl time.Duration,
	opts ...asynq.Option,
) (*asynq.TaskInfo, error) {
	// Add unique options
	opts = append(opts,
		asynq.TaskID(uniqueKey),
		asynq.Unique(ttl),
	)
	return c.EnqueueTask(taskType, payload, opts...)
}

// EnqueueTaskWithRetry enqueues a task with custom retry settings
func (c *Client) EnqueueTaskWithRetry(
	taskType string,
	payload interface{},
	maxRetry int,
	opts ...asynq.Option,
) (*asynq.TaskInfo, error) {
	// Add retry option
	opts = append(opts, asynq.MaxRetry(maxRetry))
	return c.EnqueueTask(taskType, payload, opts...)
}

// EnqueueGroupTask enqueues a task as part of a group
func (c *Client) EnqueueGroupTask(
	taskType string,
	payload interface{},
	groupKey string,
	opts ...asynq.Option,
) (*asynq.TaskInfo, error) {
	// Add group option
	opts = append(opts, asynq.Group(groupKey))
	return c.EnqueueTask(taskType, payload, opts...)
}

// withInspector is a helper function to create, use, and close an inspector.
func (c *Client) withInspector(fn func(inspector *asynq.Inspector) error) error {
	inspector := asynq.NewInspector(c.redisOpt)
	defer inspector.Close()
	return fn(inspector)
}

// GetTaskInfo retrieves task information by ID
func (c *Client) GetTaskInfo(queue, taskID string) (*asynq.TaskInfo, error) {
	var info *asynq.TaskInfo
	err := c.withInspector(func(inspector *asynq.Inspector) (err error) {
		info, err = inspector.GetTaskInfo(queue, taskID)
		return err
	})
	return info, err
}

// CancelTask cancels a pending or scheduled task
func (c *Client) CancelTask(queue, taskID string) error {
	return c.withInspector(func(inspector *asynq.Inspector) error {
		return inspector.DeleteTask(queue, taskID)
	})
}

// PauseQueue pauses task processing on the given queue.
func (c *Client) PauseQueue(queue string) error {
	return c.withInspector(func(inspector *asynq.Inspector) error {
		return inspector.PauseQueue(queue)
	})
}

// ResumeQueue resumes task processing on the given queue.
func (c *Client) ResumeQueue(queue string) error {
	return c.withInspector(func(inspector *asynq.Inspector) error {
		return inspector.UnpauseQueue(queue)
	})
}

// ArchiveTask archives a task
func (c *Client) ArchiveTask(queue, taskID string) error {
	return c.withInspector(func(inspector *asynq.Inspector) error {
		return inspector.ArchiveTask(queue, taskID)
	})
}

// ListPendingTasks lists all pending tasks in a queue
func (c *Client) ListPendingTasks(queue string, pageSize, pageNum int) ([]*asynq.TaskInfo, error) {
	var tasks []*asynq.TaskInfo
	err := c.withInspector(func(inspector *asynq.Inspector) (err error) {
		tasks, err = inspector.ListPendingTasks(queue, asynq.PageSize(pageSize), asynq.Page(pageNum))
		return err
	})
	return tasks, err
}

// ListScheduledTasks lists all scheduled tasks
func (c *Client) ListScheduledTasks(queue string, pageSize, pageNum int) ([]*asynq.TaskInfo, error) {
	var tasks []*asynq.TaskInfo
	err := c.withInspector(func(inspector *asynq.Inspector) (err error) {
		tasks, err = inspector.ListScheduledTasks(queue, asynq.PageSize(pageSize), asynq.Page(pageNum))
		return err
	})
	return tasks, err
}

// ListRetryTasks lists all retry tasks
func (c *Client) ListRetryTasks(queue string, pageSize, pageNum int) ([]*asynq.TaskInfo, error) {
	var tasks []*asynq.TaskInfo
	err := c.withInspector(func(inspector *asynq.Inspector) (err error) {
		tasks, err = inspector.ListRetryTasks(queue, asynq.PageSize(pageSize), asynq.Page(pageNum))
		return err
	})
	return tasks, err
}

// ListArchivedTasks lists all archived (completed/failed) tasks
func (c *Client) ListArchivedTasks(queue string, pageSize, pageNum int) ([]*asynq.TaskInfo, error) {
	var tasks []*asynq.TaskInfo
	err := c.withInspector(func(inspector *asynq.Inspector) (err error) {
		tasks, err = inspector.ListArchivedTasks(queue, asynq.PageSize(pageSize), asynq.Page(pageNum))
		return err
	})
	return tasks, err
}

// ListArchivedTasks lists all archived (completed) tasks
func (c *Client) ListCompletedTasks(queue string, pageSize, pageNum int) ([]*asynq.TaskInfo, error) {
	var tasks []*asynq.TaskInfo
	err := c.withInspector(func(inspector *asynq.Inspector) (err error) {
		tasks, err = inspector.ListCompletedTasks(queue, asynq.PageSize(pageSize), asynq.Page(pageNum))
		return err
	})
	return tasks, err
}

// GetQueueStats gets statistics for all queues
func (c *Client) GetQueueStats() (map[string]*asynq.QueueInfo, error) {
	stats := make(map[string]*asynq.QueueInfo)
	err := c.withInspector(func(inspector *asynq.Inspector) error {
		queues, err := inspector.Queues()
		if err != nil {
			return err
		}
		for _, q := range queues {
			info, err := inspector.GetQueueInfo(q)
			if err != nil {
				// Log or skip queue on error
				continue
			}
			stats[q] = info
		}
		return nil
	})
	return stats, err
}

// Close closes the client
func (c *Client) Close() error {
	return c.asynqClient.Close()
}
