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

// TaskLockKeyPrefix returns the prefix for task locks.
// The full key is constructed by appending the task ID.
func TaskLockKeyPrefix() string {
	return fmt.Sprintf("%s:lock:task:", redisKeyPrefix)
}

// TaskLockKey returns the key for the distributed lock for a specific task.
// Key format: "scheduler:lock:task:{taskID}"
func TaskLockKey(taskID string) string {
	return fmt.Sprintf("%s:lock:task:%s", redisKeyPrefix, taskID)
}
