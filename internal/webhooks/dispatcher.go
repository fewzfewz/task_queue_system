package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"task-queue-system/internal/metrics"
)

const (
	StreamKey        = "task_queue:webhooks:stream"
	Group            = "dispatcher-group"
	DefaultMaxRetries = 5
	DefaultBaseDelay  = 1 * time.Second
	DefaultMaxDelay   = 5 * time.Minute
)

// DispatcherConfig configures webhook delivery retry behaviour.
type DispatcherConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

func (c DispatcherConfig) defaults() DispatcherConfig {
	if c.MaxRetries <= 0 {
		c.MaxRetries = DefaultMaxRetries
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = DefaultBaseDelay
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = DefaultMaxDelay
	}
	return c
}

type Event struct {
	JobID     string      `json:"job_id"`
	TenantID  string      `json:"tenant_id"`
	Status    string      `json:"status"`
	Result    interface{} `json:"result"`
	Error     string      `json:"error"`
	Timestamp time.Time   `json:"timestamp"`
	URL       string      `json:"url"`
	Secret    string      `json:"secret"`
}

type Dispatcher struct {
	redis  *redis.Client
	logger *slog.Logger
	client *http.Client
	config DispatcherConfig
}

func NewDispatcher(rdb *redis.Client, logger *slog.Logger, cfg ...DispatcherConfig) *Dispatcher {
	c := DispatcherConfig{}.defaults()
	if len(cfg) > 0 {
		c = cfg[0].defaults()
	}
	return &Dispatcher{
		redis:  rdb,
		logger: logger,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		config: c,
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	// Create consumer group
	_ = d.redis.XGroupCreateMkStream(ctx, StreamKey, Group, "$").Err()

	d.logger.Info("webhook dispatcher started")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msgs, err := d.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    Group,
				Consumer: "dispatcher-1",
				Streams:  []string{StreamKey, ">"},
				Count:    10,
				Block:    5 * time.Second,
			}).Result()

			if err != nil && err != redis.Nil {
				d.logger.Error("failed to read from webhook stream", "error", err)
				continue
			}

			for _, stream := range msgs {
				for _, msg := range stream.Messages {
					var ev Event
					if data, ok := msg.Values["data"].(string); ok {
						_ = json.Unmarshal([]byte(data), &ev)
					}

					go d.dispatch(ctx, ev, msg.ID)
				}
			}
		}
	}
}

func (d *Dispatcher) dispatch(ctx context.Context, ev Event, msgID string) {
	for attempt := 0; attempt < d.config.MaxRetries; attempt++ {
		var backoffMs int64
		if attempt > 0 {
			delay := d.backoff(attempt)
			backoffMs = delay.Milliseconds()
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}

		status, err := d.send(ctx, ev)
		rec := DeliveryRecord{
			Timestamp:  time.Now().UTC(),
			JobID:      ev.JobID,
			TenantID:   ev.TenantID,
			URL:        ev.URL,
			StatusCode: status,
			Attempt:    attempt + 1,
			Success:    err == nil,
			BackoffMs:  backoffMs,
		}
		if err != nil {
			rec.Error = err.Error()
		}
		if rerr := RecordDelivery(ctx, d.redis, rec); rerr != nil {
			d.logger.Warn("failed to record webhook delivery", "error", rerr)
		}

		if err == nil {
			d.redis.XAck(ctx, StreamKey, Group, msgID)
			metrics.WebhookDeliveryTotal.WithLabelValues(ev.TenantID, "delivered").Inc()
			return
		}

		// Don't retry client errors (4xx).
		if isClientError(err) {
			d.logger.Warn("webhook rejected with client error, not retrying",
				"job_id", ev.JobID, "error", err)
			d.redis.XAck(ctx, StreamKey, Group, msgID)
			metrics.WebhookDeliveryFailuresTotal.WithLabelValues(ev.TenantID, "client_error").Inc()
			return
		}

		d.logger.Warn("webhook delivery failed",
			"job_id", ev.JobID,
			"attempt", attempt+1,
			"max_retries", d.config.MaxRetries,
			"error", err)
		metrics.WebhookDeliveryFailuresTotal.WithLabelValues(ev.TenantID, "retry").Inc()
	}

	d.logger.Error("webhook marked as dead after all attempts", "job_id", ev.JobID)
	metrics.WebhookDeliveryFailuresTotal.WithLabelValues(ev.TenantID, "exhausted_retries").Inc()
	d.redis.XAck(ctx, StreamKey, Group, msgID)
}

// backoff computes an exponential backoff with jitter for the given attempt.
func (d *Dispatcher) backoff(attempt int) time.Duration {
	delay := d.config.BaseDelay * (1 << min(attempt, d.config.MaxRetries))
	if delay > d.config.MaxDelay {
		delay = d.config.MaxDelay
	}
	// Add ±25% jitter.
	jitter := time.Duration(rand.Int63n(int64(delay / 2))) - time.Duration(int64(delay)/4)
	return delay + jitter
}

type webhookHTTPError struct {
	status int
}

func (e *webhookHTTPError) Error() string {
	return fmt.Sprintf("server returned status: %d", e.status)
}

func (e *webhookHTTPError) StatusCode() int { return e.status }

func isClientError(err error) bool {
	var se interface{ StatusCode() int }
	if errors.As(err, &se) {
		return se.StatusCode() >= 400 && se.StatusCode() < 500
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// send delivers the event, returning the HTTP status code (0 on transport
// errors) and any error.
func (d *Dispatcher) send(ctx context.Context, ev Event) (int, error) {
	body, _ := json.Marshal(ev)
	req, err := http.NewRequestWithContext(ctx, "POST", ev.URL, bytes.NewBuffer(body))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	
	if ev.Secret != "" {
		sig := sign(body, ev.Secret)
		req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return resp.StatusCode, &webhookHTTPError{status: resp.StatusCode}
	}

	return resp.StatusCode, nil
}

func sign(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
