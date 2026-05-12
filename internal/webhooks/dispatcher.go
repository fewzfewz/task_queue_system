package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"task-queue-system/internal/metrics"
)

const (
	StreamKey = "task_queue:webhooks:stream"
	Group     = "dispatcher-group"
)

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
}

func NewDispatcher(rdb *redis.Client, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		redis:  rdb,
		logger: logger,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
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
	backoffs := []time.Duration{0, 5 * time.Second, 30 * time.Second, 5 * time.Minute}
	
	for attempt := 0; attempt < len(backoffs); attempt++ {
		if attempt > 0 {
			time.Sleep(backoffs[attempt])
		}

		err := d.send(ctx, ev)
		if err == nil {
			d.redis.XAck(ctx, StreamKey, Group, msgID)
			return
		}

		d.logger.Warn("webhook delivery failed", 
			"job_id", ev.JobID, 
			"attempt", attempt+1, 
			"error", err)
	}

	d.logger.Error("webhook marked as dead after all attempts", "job_id", ev.JobID)
	metrics.WebhookDeliveryFailuresTotal.WithLabelValues(ev.TenantID, "exhausted_retries").Inc()
	d.redis.XAck(ctx, StreamKey, Group, msgID) // Dead letter
}

func (d *Dispatcher) send(ctx context.Context, ev Event) error {
	body, _ := json.Marshal(ev)
	req, err := http.NewRequestWithContext(ctx, "POST", ev.URL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	
	if ev.Secret != "" {
		sig := sign(body, ev.Secret)
		req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	return nil
}

func sign(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
