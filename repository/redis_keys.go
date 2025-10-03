package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

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

// IdempotencyKey returns the key for an idempotency check.
// Key format: "scheduler:idempotency:{key}"
func IdempotencyKey(key string) string {
	return fmt.Sprintf("%s:idempotency:%s", redisKeyPrefix, key)
}

// Atomically reserve an idempotency key as P:<token>, with pending TTL.
// Returns (reserved=true) if we successfully reserved.
// If already exists, returns (reserved=false, existingValue="T:<id>" or "P:<token2>")
func ReserveIdem(ctx context.Context, rdb *redis.Client, key, token string, pendingTTL time.Duration) (reserved bool, existing string, err error) {
	const lua = `
		-- KEYS[1]: The idempotency key, e.g., "scheduler:idempotency:<dedup_key>"
		-- ARGV[1]: A unique token (UUID) for this request
		-- ARGV[2]: The pending TTL in milliseconds, e.g., 30000

		local v = redis.call("GET", KEYS[1])
		if not v then
			redis.call("SET", KEYS[1], "P:"..ARGV[1], "PX", ARGV[2], "NX")
			return {1}
		else
			return {0, v}
		end
	`
	res, err := redis.NewScript(lua).Run(ctx, rdb, []string{key}, token, pendingTTL.Milliseconds()).Result()
	if err != nil {
		return false, "", err
	}
	arr := res.([]interface{})
	if len(arr) == 1 { // {1}
		return true, "", nil
	}
	return false, arr[1].(string), nil
}

// Finalize idempotency key: change P:<token> -> T:<taskID> with final TTL.
// Only succeeds if current value still equals P:<token>.
func FinalizeIdem(ctx context.Context, rdb *redis.Client, key, token, taskID string, finalTTL time.Duration) error {
	const lua = `
		-- KEYS[1]: The idempotency key
		-- ARGV[1]: The token we used during the reservation step
		-- ARGV[2]: The final task ID (a UUID string)
		-- ARGV[3]: The final TTL in milliseconds, e.g., 3600000 (1 hour)

		if redis.call("GET", KEYS[1]) == "P:"..ARGV[1] then
			return redis.call("SET", KEYS[1], "T:"..ARGV[2], "PX", ARGV[3])
		else
			return 0
		end
	`
	_, err := redis.NewScript(lua).Run(ctx, rdb, []string{key}, token, taskID, finalTTL.Milliseconds()).Result()
	return err
}
