package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Port             string
	MigrationWorkers int
	LogLevel         string
}

// Load reads configuration from .env file (if present) and environment variables.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	workers, err := strconv.Atoi(getEnv("MIGRATION_WORKERS", "5"))
	if err != nil {
		workers = 5
	}

	return &Config{
		Port:             getEnv("PORT", "8080"),
		MigrationWorkers: workers,
		LogLevel:         getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
