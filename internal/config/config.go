// Package config provides application-wide configuration loaded from environment variables.
package config

import (
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
}

// Load reads environment variables and returns a populated Config with defaults.
func Load() *Config {
	return &Config{
		ServerPort:    getEnvOrDefault("PORT", "8080"),
		RedisHost:     getEnvOrDefault("REDIS_HOST", "localhost:6379"),
		RedisPassword: getEnvOrDefault("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),
	}
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
