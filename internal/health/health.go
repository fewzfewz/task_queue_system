package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type Pinger interface {
	Ping(context.Context) error
}

type RedisClient interface {
	Ping(context.Context) *redis.StatusCmd
}

type redisAdapter struct {
	client RedisClient
}

func (r redisAdapter) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func AdaptRedis(client RedisClient) Pinger {
	return redisAdapter{client: client}
}

type Checker struct {
	name   string
	pinger Pinger
}

func NewChecker(name string, pinger Pinger) *Checker {
	return &Checker{name: name, pinger: pinger}
}

func (c *Checker) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"service": c.name,
	})
}

func (c *Checker) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if c.pinger != nil {
		if err := c.pinger.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status":  "not_ready",
				"service": c.name,
				"error":   err.Error(),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
		"service": c.name,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
