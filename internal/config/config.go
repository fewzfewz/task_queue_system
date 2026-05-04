// Package config provides application-wide configuration loaded from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds the configuration for both API and Worker.
type Config struct {
	// ServerPort is the HTTP port the API will listen on. Default: 8080
	ServerPort string

	// RedisHost is the Redis server address, e.g. "localhost:6379".
	RedisHost string

	// RedisPassword is the password to authenticate with Redis, if any.
	RedisPassword string

	// RedisDB is the database number to use. Default: 0
	RedisDB int

	// ApiKey is the required secret for protected endpoints.
	ApiKey string

	// JobRateLimit is the global throughput limit for worker tasks (JPS).
	// Default: 0 (unlimited)
	JobRateLimit float64

	// LogLevel is the minimum level to log (info, error, debug).
	// Default: info
	LogLevel string
}

// Load reads environment variables and returns a populated Config with defaults.
func Load() *Config {
	return &Config{
		ServerPort:    getEnvOrDefault("PORT", "8080"),
		RedisHost:     getEnvOrDefault("REDIS_HOST", "localhost:6379"),
		RedisPassword: getEnvOrDefault("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),
		ApiKey:        getEnvOrDefault("API_KEY", "secret-api-key"),
		JobRateLimit:  getEnvAsFloat("JOB_RATE_LIMIT", 0.0),
		LogLevel:      getEnvOrDefault("LOG_LEVEL", "info"),
	}
}

// Validate checks for critical configuration errors.
func (c *Config) Validate() error {
	if c.RedisHost == "" {
		return fmt.Errorf("REDIS_HOST is required")
	}
	if c.ServerPort == "" {
		return fmt.Errorf("PORT is required")
	}
	if c.ApiKey == "" {
		return fmt.Errorf("API_KEY is required")
	}
	return nil
}

// ── private helpers ───────────────────────────────────────────────────────────

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	str := os.Getenv(key)
	if str == "" {
		return defaultVal
	}
	if v, err := strconv.Atoi(str); err == nil {
		return v
	}
	return defaultVal
}
func getEnvAsFloat(key string, defaultVal float64) float64 {
	str := os.Getenv(key)
	if str == "" {
		return defaultVal
	}
	if v, err := strconv.ParseFloat(str, 64); err == nil {
		return v
	}
	return defaultVal
}
