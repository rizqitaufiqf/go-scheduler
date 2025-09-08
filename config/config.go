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
	DBSource           string
	RedisURL           string
	WorkerConcurrency  int
	WorkerPollInterval time.Duration
}

// LoadConfig loads configuration from .env file
func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		// Don't fail if .env is not present, it might be set in the environment
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=%s",
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_DB"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("TZ"),
	)

	workerConcurrency, err := strconv.Atoi(os.Getenv("WORKER_CONCURRENCY"))
	if err != nil || workerConcurrency <= 0 {
		workerConcurrency = 10 // Default value
	}

	pollIntervalSeconds, err := strconv.Atoi(os.Getenv("WORKER_POLL_INTERVAL_SECONDS"))
	if err != nil || pollIntervalSeconds <= 0 {
		pollIntervalSeconds = 5 // Default value
	}

	return &Config{
		DBSource:           dsn,
		RedisURL:           os.Getenv("REDIS_ADDR"),
		WorkerConcurrency:  workerConcurrency,
		WorkerPollInterval: time.Duration(pollIntervalSeconds) * time.Second,
	}, nil
}
