package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	deliveriesKeyPrefix = "task_queue:webhooks:deliveries:"
	// MaxDeliveriesPerURL bounds how many delivery attempts are retained per
	// webhook URL. Older attempts are trimmed away.
	MaxDeliveriesPerURL = 50
)

// DeliveryRecord captures a single webhook delivery attempt (including
// retries). It is what powers the per-endpoint delivery history surfaced in
// the operator UI.
type DeliveryRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	JobID      string    `json:"job_id"`
	TenantID   string    `json:"tenant_id"`
	URL        string    `json:"url"`
	StatusCode int       `json:"status_code"` // 0 = transport error (no HTTP response)
	Attempt    int       `json:"attempt"`     // 1-based
	Success    bool      `json:"success"`
	Error      string    `json:"error,omitempty"`
	BackoffMs  int64     `json:"backoff_ms,omitempty"` // wait before this attempt (0 on first try)
}

// RecordDelivery persists a delivery attempt to a per-URL bounded history list.
func RecordDelivery(ctx context.Context, rdb *redis.Client, rec DeliveryRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("webhook: marshal delivery record: %w", err)
	}
	key := deliveriesKeyPrefix + rec.URL
	pipe := rdb.TxPipeline()
	pipe.LPush(ctx, key, data)
	pipe.LTrim(ctx, key, 0, MaxDeliveriesPerURL-1)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("webhook: record delivery: %w", err)
	}
	return nil
}

// ListDeliveries returns the most recent delivery records for a URL, newest first.
func ListDeliveries(ctx context.Context, rdb *redis.Client, url string, limit int) ([]DeliveryRecord, error) {
	if limit <= 0 || limit > MaxDeliveriesPerURL {
		limit = MaxDeliveriesPerURL
	}
	key := deliveriesKeyPrefix + url
	items, err := rdb.LRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("webhook: list deliveries: %w", err)
	}
	records := make([]DeliveryRecord, 0, len(items))
	for _, raw := range items {
		var rec DeliveryRecord
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// RecordDelivery persists a delivery attempt for a registered webhook endpoint.
func (s *WebhookStore) RecordDelivery(ctx context.Context, rec DeliveryRecord) error {
	if s == nil || s.redis == nil {
		return nil
	}
	return RecordDelivery(ctx, s.redis, rec)
}

// ListDeliveries returns the delivery history for a webhook URL.
func (s *WebhookStore) ListDeliveries(ctx context.Context, url string, limit int) ([]DeliveryRecord, error) {
	if s == nil || s.redis == nil {
		return nil, nil
	}
	return ListDeliveries(ctx, s.redis, url, limit)
}
