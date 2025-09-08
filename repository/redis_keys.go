package repository

import "fmt"

const (
	// redisKeyPrefix is the global prefix for all keys used by this application.
	// Using a prefix creates a namespace, making it easy to manage and monitor keys.
	redisKeyPrefix = "scheduler"
)

// TasksQueueKey returns the key for the Redis sorted set that holds scheduled tasks.
// Key format: "scheduler:tasks"
func TasksQueueKey() string {
	return fmt.Sprintf("%s:tasks", redisKeyPrefix)
}

// TaskLockKey returns the key for the distributed lock for a specific task.
// Key format: "scheduler:lock:task:{taskID}"
func TaskLockKey(taskID string) string {
	return fmt.Sprintf("%s:lock:task:%s", redisKeyPrefix, taskID)
}
