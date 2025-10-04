package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// Database
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresHost     string
	PostgresPort     string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Server
	Port string
	TZ   string

	// Asynq Worker Configuration
	WorkerConcurrency int

	// Queue Priorities (high to low)
	Queues map[string]int

	// Retry Configuration
	MaxRetry       int
	RetryDelayFunc string // "exponential" or "constant"

	// Task Timeout
	TaskTimeout int // seconds

	// Task Retention
	TaskRetention int // hours, how long to keep completed tasks
}

func LoadConfig() *Config {
	// Load .env file if exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	return &Config{
		// Database
		PostgresUser:     getEnv("POSTGRES_USER", "user"),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", "password"),
		PostgresDB:       getEnv("POSTGRES_DB", "scheduler_db"),
		PostgresHost:     getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:     getEnv("POSTGRES_PORT", "5432"),

		// Redis
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		// Server
		Port: getEnv("PORT", "8080"),
		TZ:   getEnv("TZ", "Asia/Jakarta"),

		// Asynq Worker
		WorkerConcurrency: getEnvAsInt("WORKER_CONCURRENCY", 10),

		// Queue Priorities (priority level as value)
		Queues: map[string]int{
			"critical": 6, // Highest priority
			"high":     5,
			"default":  4,
			"medium":   3,
			"low":      2,
			"batch":    1, // Lowest priority
		},

		// Retry
		MaxRetry:       getEnvAsInt("MAX_RETRY", 5),
		RetryDelayFunc: getEnv("RETRY_DELAY_FUNC", "exponential"),

		// Timeout
		TaskTimeout: getEnvAsInt("TASK_TIMEOUT_SECONDS", 300),

		// Retention
		TaskRetention: getEnvAsInt("TASK_RETENTION_HOURS", 24),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func (c *Config) GetDSN() string {
	return "host=" + c.PostgresHost +
		" user=" + c.PostgresUser +
		" password=" + c.PostgresPassword +
		" dbname=" + c.PostgresDB +
		" port=" + c.PostgresPort +
		" sslmode=disable TimeZone=" + c.TZ
}
