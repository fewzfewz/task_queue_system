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

	// AdminUsername is the login username for the web UI.
	AdminUsername string

	// AdminPassword is the login password for the web UI.
	AdminPassword string

	// ReadonlyUsername/ReadonlyPassword optionally define a second UI account
	// with viewer-only (no state-changing) permissions. Empty disables it.
	ReadonlyUsername string
	ReadonlyPassword string

	// SessionTTLSeconds is how long an operator UI session lasts before the
	// session cookie expires. Default: 28800 (8 hours).
	SessionTTLSeconds int

	// LoginRateLimit is the maximum number of login attempts per IP per minute.
	// Default: 5.
	LoginRateLimit int

	// WorkerAddr is the base address of the worker's HTTP server, used to proxy
	// circuit-breaker access through the API. Default: "localhost:8081".
	WorkerAddr string

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

	// DrainTimeoutSeconds is the maximum time to wait for a worker to finish
	// in-flight jobs before force exiting during a graceful shutdown.
	// Default: 60 seconds.
	DrainTimeoutSeconds int

	// WorkerPoolSize is the number of goroutines the worker process starts.
	// Default: 50.
	WorkerPoolSize int

	// WorkerPort is the HTTP port the worker's metrics/health server listens on.
	// Default: "8081"
	WorkerPort string

	// SchedulerPort is the HTTP port the scheduler's health server listens on.
	// Default: "8082"
	SchedulerPort string

	// TenantRateLimit is the max requests per second per tenant. Default: 0 (unlimited).
	TenantRateLimit int64

	// SLATargetSeconds is the target execution time in seconds for SLA compliance.
	// Default: 5.
	SLATargetSeconds int

	// OTELExporterOTLPEndpoint is the OpenTelemetry OTLP/HTTP endpoint.
	// When set, traces are exported via OTLP. When empty, no tracing SDK is initialized.
	// Example: "localhost:4318"
	OTELExporterOTLPEndpoint string
}


// Load reads environment variables and returns a populated Config with defaults.
func Load() *Config {
	return &Config{
		ServerPort:    getEnvOrDefault("PORT", "8080"),
		RedisHost:     getEnvOrDefault("REDIS_HOST", "localhost:6379"),
		RedisPassword: getEnvOrDefault("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),
		ApiKey:          getEnvOrDefault("API_KEY", "secret-api-key"),
		AdminUsername:   getEnvOrDefault("ADMIN_USERNAME", "admin"),
		AdminPassword:   getEnvOrDefault("ADMIN_PASSWORD", "admin123"),
		ReadonlyUsername:   getEnvOrDefault("READONLY_USERNAME", ""),
		ReadonlyPassword:   getEnvOrDefault("READONLY_PASSWORD", ""),
		SessionTTLSeconds:  getEnvAsInt("SESSION_TTL_SECONDS", 28800),
		LoginRateLimit:     getEnvAsInt("LOGIN_RATE_LIMIT", 5),
		WorkerAddr:         getEnvOrDefault("WORKER_ADDR", "localhost:8081"),
		JobRateLimit:  getEnvAsFloat("JOB_RATE_LIMIT", 0.0),
		LogLevel:      getEnvOrDefault("LOG_LEVEL", "info"),
		MaxQueueSize:  getEnvAsInt64("MAX_QUEUE_SIZE", 10000),
		StoreBackend:  getEnvOrDefault("STORE_BACKEND", "redis"),
		PostgresConnStr: getEnvOrDefault("POSTGRES_CONN_STR", ""),
		DrainTimeoutSeconds: getEnvAsInt("DRAIN_TIMEOUT", 60),
		WorkerPoolSize:      getEnvAsInt("WORKER_POOL_SIZE", 50),
		WorkerPort:          getEnvOrDefault("WORKER_PORT", "8081"),
		SchedulerPort:       getEnvOrDefault("SCHEDULER_PORT", "8082"),
		TenantRateLimit:     getEnvAsInt64("TENANT_RATE_LIMIT", 0),
		SLATargetSeconds:           getEnvAsInt("SLA_TARGET_SECONDS", 5),
		OTELExporterOTLPEndpoint:   getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
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
	switch c.StoreBackend {
	case "redis", "postgres", "dual":
	default:
		return fmt.Errorf("STORE_BACKEND must be one of: redis, postgres, dual (got %q)", c.StoreBackend)
	}
	if (c.StoreBackend == "postgres" || c.StoreBackend == "dual") && c.PostgresConnStr == "" {
		return fmt.Errorf("POSTGRES_CONN_STR is required when STORE_BACKEND is %s", c.StoreBackend)
	}
	switch c.LogLevel {
	case "info", "error", "debug":
	default:
		return fmt.Errorf("LOG_LEVEL must be one of: info, error, debug (got %q)", c.LogLevel)
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
