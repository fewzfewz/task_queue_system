package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const webhookStoreKey = "task_queue:webhooks:registered"

type RegisteredWebhook struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"`
	Events    []string  `json:"events"`
	CreatedAt time.Time `json:"created_at"`
}

type WebhookStore struct {
	redis *redis.Client
}

func NewWebhookStore(rdb *redis.Client) *WebhookStore {
	return &WebhookStore{redis: rdb}
}

func (s *WebhookStore) Create(ctx context.Context, tenantID, url, secret string, events []string) (*RegisteredWebhook, error) {
	events = NormalizeEvents(events)
	w := &RegisteredWebhook{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		URL:       url,
		Secret:    secret,
		Events:    events,
		CreatedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("webhook_store: marshal: %w", err)
	}
	if err := s.redis.HSet(ctx, webhookStoreKey, w.ID, data).Err(); err != nil {
		return nil, fmt.Errorf("webhook_store: hset: %w", err)
	}
	return w, nil
}

func (s *WebhookStore) GetByID(ctx context.Context, id string) (*RegisteredWebhook, error) {
	data, err := s.redis.HGet(ctx, webhookStoreKey, id).Bytes()
	if err != nil {
		return nil, fmt.Errorf("webhook_store: hget: %w", err)
	}
	var w RegisteredWebhook
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("webhook_store: unmarshal: %w", err)
	}
	return &w, nil
}

func (s *WebhookStore) List(ctx context.Context, tenantID string) ([]*RegisteredWebhook, error) {
	data, err := s.redis.HGetAll(ctx, webhookStoreKey).Result()
	if err != nil {
		return nil, fmt.Errorf("webhook_store: hgetall: %w", err)
	}
	var result []*RegisteredWebhook
	for _, raw := range data {
		var w RegisteredWebhook
		if err := json.Unmarshal([]byte(raw), &w); err != nil {
			continue
		}
		if tenantID == "" || w.TenantID == tenantID {
			result = append(result, &w)
		}
	}
	return result, nil
}

func (s *WebhookStore) Update(ctx context.Context, id, url, secret string, events []string) (*RegisteredWebhook, error) {
	w, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if url != "" {
		w.URL = url
	}
	if secret != "" {
		w.Secret = secret
	}
	if len(events) > 0 {
		w.Events = NormalizeEvents(events)
	}
	data, err := json.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("webhook_store: marshal: %w", err)
	}
	if err := s.redis.HSet(ctx, webhookStoreKey, w.ID, data).Err(); err != nil {
		return nil, fmt.Errorf("webhook_store: hset: %w", err)
	}
	return w, nil
}

func (s *WebhookStore) Delete(ctx context.Context, id string) error {
	return s.redis.HDel(ctx, webhookStoreKey, id).Err()
}

// Match returns all registered webhooks for a tenant that match the given status event.
func (s *WebhookStore) Match(ctx context.Context, tenantID, status string) ([]*RegisteredWebhook, error) {
	all, err := s.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	var matched []*RegisteredWebhook
	for _, w := range all {
		for _, e := range w.Events {
			if NormalizeEvent(e) == NormalizeEvent(status) || e == "*" {
				matched = append(matched, w)
				break
			}
		}
	}
	return matched, nil
}
