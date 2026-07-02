package config

import "testing"

func TestConfigValidate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &Config{
			ServerPort:   "8080",
			RedisHost:    "localhost:6379",
			ApiKey:       "secret",
			StoreBackend: "redis",
			LogLevel:     "info",
		}

		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected valid config, got error: %v", err)
		}
	})

	t.Run("missing redis host", func(t *testing.T) {
		cfg := &Config{
			ServerPort: "8080",
			ApiKey:     "secret",
		}

		if err := cfg.Validate(); err == nil {
			t.Fatal("expected validation error for missing redis host")
		}
	})

	t.Run("missing port", func(t *testing.T) {
		cfg := &Config{
			RedisHost: "localhost:6379",
			ApiKey:    "secret",
		}

		if err := cfg.Validate(); err == nil {
			t.Fatal("expected validation error for missing port")
		}
	})

	t.Run("missing api key", func(t *testing.T) {
		cfg := &Config{
			ServerPort: "8080",
			RedisHost:  "localhost:6379",
		}

		if err := cfg.Validate(); err == nil {
			t.Fatal("expected validation error for missing api key")
		}
	})
}
