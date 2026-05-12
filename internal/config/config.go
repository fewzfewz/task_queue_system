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

	// MaxQueueSize is the maximum number of pending jobs allowed in the queue.
	// Default: 10000 (0 = unlimited)
	MaxQueueSize int64

	// StoreBackend determines where job states are persisted (redis, postgres, dual).
	// Default: redis
	StoreBackend string

	// PostgresConnStr is the DSN for the Postgres database.
	PostgresConnStr string

	// JwtPublicKey is the RSA public key in PEM format for token validation.
	JwtPublicKey string

	// JwtPublicKeyPath is the path to the RSA public key file.
	JwtPublicKeyPath string
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
		MaxQueueSize:  getEnvAsInt64("MAX_QUEUE_SIZE", 10000),
		StoreBackend:  getEnvOrDefault("STORE_BACKEND", "redis"),
		PostgresConnStr: getEnvOrDefault("POSTGRES_CONN_STR", ""),
		JwtPublicKey:  getEnvOrDefault("JWT_PUBLIC_KEY", ""),
		JwtPublicKeyPath: getEnvOrDefault("JWT_PUBLIC_KEY_PATH", ""),
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
func getEnvAsInt64(key string, defaultVal int64) int64 {
	str := os.Getenv(key)
	if str == "" {
		return defaultVal
	}
	if v, err := strconv.ParseInt(str, 10, 64); err == nil {
		return v
	}
	return defaultVal
}
