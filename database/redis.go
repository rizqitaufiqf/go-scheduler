package database

import (
	"github.com/go-redis/redis/v8"
)

// InitRedis initializes the Redis client.
func InitRedis(url string) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: url,
	})

	return rdb
}
