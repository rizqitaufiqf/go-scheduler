package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application.
type Config struct {
	// Raw values / primitives
	Dsn              string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresHost     string
	PostgresPort     string
	RedisAddr        string
	TZ               string

	// Worker numeric configs (seconds / counts)
	MaxFailRefresh               int
	WorkerConcurrency            int
	WorkerPollIntervalSeconds    int
	LockTTLSeconds               int
	PeriodicReconIntervalSeconds int
	BackoffBaseDelaySeconds      int
	BackoffMaxDelaySeconds       int

	// Notification configs
	NotificationEnabled            bool
	NotificationRedisChannel       string
	NotificationRetentionDays      int
	NotificationUsePostgresSession bool

	// JWT configs
	JWTSecret      string
	JWTExpiryHours int

	// WebSocket configs
	WSReadBufferSize      int
	WSWriteBufferSize     int
	WSPingIntervalSeconds int
	WSPongTimeoutSeconds  int
	WSMaxMessageSize      int
}

const (
	// defaults
	defaultWorkerConcurrency         = 10
	defaultWorkerPollIntervalSeconds = 10
	defaultLockTTLSeconds            = 300
	defaultPeriodicReconSeconds      = 300
	defaultMaxFailRefresh            = 3
	defaultBackoffBaseSeconds        = 5
	defaultBackoffMaxSeconds         = 300

	defaultPostgresUser = "user"
	defaultPostgresPass = "password"
	defaultPostgresDB   = "scheduler_db"
	defaultPostgresHost = "postgresql"
	defaultPostgresPort = "5432"
	defaultRedisAddr    = "redis:6379"
	defaultTZ           = "Asia/Jakarta"
)

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		// Raw database / redis / tz
		PostgresUser:     getEnv("POSTGRES_USER", defaultPostgresUser),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", defaultPostgresPass),
		PostgresDB:       getEnv("POSTGRES_DB", defaultPostgresDB),
		PostgresHost:     getEnv("POSTGRES_HOST", defaultPostgresHost),
		PostgresPort:     getEnv("POSTGRES_PORT", defaultPostgresPort),
		RedisAddr:        getEnv("REDIS_ADDR", defaultRedisAddr),
		TZ:               getEnv("TZ", defaultTZ),
		// Worker counts / timeouts (seconds)
		WorkerConcurrency:            getEnvAsInt("WORKER_CONCURRENCY", defaultWorkerConcurrency),
		WorkerPollIntervalSeconds:    getEnvAsInt("WORKER_POLL_INTERVAL_SECONDS", defaultWorkerPollIntervalSeconds),
		LockTTLSeconds:               getEnvAsInt("LOCK_TTL_SECONDS", defaultLockTTLSeconds),
		PeriodicReconIntervalSeconds: getEnvAsInt("PERIODIC_RECON_INTERVAL_SECONDS", defaultPeriodicReconSeconds),
		MaxFailRefresh:               getEnvAsInt("MAX_FAIL_REFRESH", defaultMaxFailRefresh),
		BackoffBaseDelaySeconds:      getEnvAsInt("BACKOFF_BASE_DELAY_SECONDS", defaultBackoffBaseSeconds),
		BackoffMaxDelaySeconds:       getEnvAsInt("BACKOFF_MAX_DELAY_SECONDS", defaultBackoffMaxSeconds),

		// Notification
		NotificationEnabled:            getEnvAsBool("NOTIFICATION_ENABLED", true),
		NotificationRedisChannel:       getEnv("NOTIFICATION_REDIS_CHANNEL", "notification-dev"),
		NotificationRetentionDays:      getEnvAsInt("NOTIFICATION_RETENTION_DAYS", 30),
		NotificationUsePostgresSession: getEnvAsBool("NOTIFICATION_USE_POSTGRES_SESSION", false),

		// JWT
		JWTSecret:      getEnv("JWT_SECRET", "change-this-secret-key"),
		JWTExpiryHours: getEnvAsInt("JWT_EXPIRY_HOURS", 24),

		// WebSocket
		WSReadBufferSize:      getEnvAsInt("WS_READ_BUFFER_SIZE", 1024),
		WSWriteBufferSize:     getEnvAsInt("WS_WRITE_BUFFER_SIZE", 1024),
		WSPingIntervalSeconds: getEnvAsInt("WS_PING_INTERVAL_SECONDS", 30),
		WSPongTimeoutSeconds:  getEnvAsInt("WS_PONG_TIMEOUT_SECONDS", 60),
		WSMaxMessageSize:      getEnvAsInt("WS_MAX_MESSAGE_SIZE", 512),
	}

	// Construct DSN after initializing other fields
	cfg.Dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=%s",
		cfg.PostgresHost,
		cfg.PostgresUser,
		cfg.PostgresPassword,
		cfg.PostgresDB,
		cfg.PostgresPort,
		cfg.TZ)

	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvAsInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvAsBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
