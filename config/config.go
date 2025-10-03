package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	DBSource              string
	RedisURL              string
	WorkerConcurrency     int
	WorkerPollInterval    time.Duration
	LockTTL               time.Duration
	PeriodicReconInterval time.Duration
	MaxFailRefresh        int
	BackoffBaseDelay      time.Duration
	BackoffMaxDelay       time.Duration
}

// LoadConfig loads configuration from .env file
func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		// Don't fail if .env is not present, it might be set in the environment
	}

	// --- Database & Redis Configuration ---
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=%s",
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_DB"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("TZ"),
	)

	// --- Worker Configuration ---
	workerConcurrency, err := strconv.Atoi(os.Getenv("WORKER_CONCURRENCY"))
	if err != nil || workerConcurrency <= 0 {
		workerConcurrency = 10 // Default value
	}

	pollIntervalSeconds, err := strconv.Atoi(os.Getenv("WORKER_POLL_INTERVAL_SECONDS"))
	if err != nil || pollIntervalSeconds <= 0 {
		pollIntervalSeconds = 10 // Default value
	}

	// --- Task Lock & Resilience Configuration ---
	lockTTLSeconds, err := strconv.Atoi(os.Getenv("LOCK_TTL_SECONDS"))
	if err != nil || lockTTLSeconds <= 0 {
		lockTTLSeconds = 300 // Default: 5 minutes
	}

	reconIntervalSeconds, err := strconv.Atoi(os.Getenv("PERIODIC_RECON_INTERVAL_SECONDS"))
	if err != nil || reconIntervalSeconds <= 0 {
		reconIntervalSeconds = 300 // Default: 5 minutes
	}

	maxFailRefresh, err := strconv.Atoi(os.Getenv("MAX_FAIL_REFRESH"))
	if err != nil || maxFailRefresh <= 0 {
		maxFailRefresh = 3 // Default value
	}

	backoffBaseSeconds, err := strconv.Atoi(os.Getenv("BACKOFF_BASE_DELAY_SECONDS"))
	if err != nil || backoffBaseSeconds <= 0 {
		backoffBaseSeconds = 5 // Default value
	}

	backoffMaxSeconds, err := strconv.Atoi(os.Getenv("BACKOFF_MAX_DELAY_SECONDS"))
	if err != nil || backoffMaxSeconds <= 0 {
		backoffMaxSeconds = 300 // Default: 5 minutes
	}

	return &Config{
		DBSource:              dsn,
		RedisURL:              os.Getenv("REDIS_ADDR"),
		WorkerConcurrency:     workerConcurrency,
		WorkerPollInterval:    time.Duration(pollIntervalSeconds) * time.Second,
		LockTTL:               time.Duration(lockTTLSeconds) * time.Second,
		PeriodicReconInterval: time.Duration(reconIntervalSeconds) * time.Second,
		MaxFailRefresh:        maxFailRefresh,
		BackoffBaseDelay:      time.Duration(backoffBaseSeconds) * time.Second,
		BackoffMaxDelay:       time.Duration(backoffMaxSeconds) * time.Second,
	}, nil
}
