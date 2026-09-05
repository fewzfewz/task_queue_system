package jobtypes

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const storeKey = "task_queue:job_types:registered"

// Built-in job types always available without registration.
var BuiltIn = []JobType{
	{Name: "email", Description: "Simulated email delivery (logs only — configure SMTP for production)", Handler: "email", BuiltIn: true},
	{Name: "image", Description: "Image processing pipeline (resize, transform)", Handler: "image", BuiltIn: true},
	{Name: "http", Description: "HTTP callback — POST/GET/PUT to any URL from job payload", Handler: "http", BuiltIn: true},
}

// JobType describes a supported job type and the worker plugin that executes it.
type JobType struct {
	ID          string    `json:"id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Handler     string    `json:"handler"`
	PayloadHint      string    `json:"payload_hint,omitempty"`
	BuiltIn          bool      `json:"built_in,omitempty"`
	ConcurrencyLimit int       `json:"concurrency_limit,omitempty"`
	Paused           bool      `json:"paused,omitempty"`
	CreatedAt        time.Time `json:"created_at,omitempty"`
}

// Store persists admin-registered custom job types in Redis.
type Store struct {
	redis *redis.Client
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{redis: rdb}
}

func (s *Store) Create(ctx context.Context, name, description, handler, payloadHint string) (*JobType, error) {
	if name == "" {
		return nil, fmt.Errorf("job type name is required")
	}
	if handler == "" {
		handler = "http"
	}
	for _, b := range BuiltIn {
		if b.Name == name {
			return nil, fmt.Errorf("job type %q is built-in and cannot be re-registered", name)
		}
	}
	if existing, _ := s.GetByName(ctx, name); existing != nil {
		return nil, fmt.Errorf("job type %q already exists", name)
	}

	jt := &JobType{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Handler:     handler,
		PayloadHint: payloadHint,
		CreatedAt:   time.Now().UTC(),
	}
	data, err := json.Marshal(jt)
	if err != nil {
		return nil, err
	}
	if err := s.redis.HSet(ctx, storeKey, jt.Name, data).Err(); err != nil {
		return nil, err
	}
	return jt, nil
}

func (s *Store) GetByName(ctx context.Context, name string) (*JobType, error) {
	data, err := s.redis.HGet(ctx, storeKey, name).Bytes()
	if err != nil {
		return nil, err
	}
	var jt JobType
	if err := json.Unmarshal(data, &jt); err != nil {
		return nil, err
	}
	return &jt, nil
}

func (s *Store) List(ctx context.Context) ([]JobType, error) {
	all, err := s.redis.HGetAll(ctx, storeKey).Result()
	if err != nil {
		return nil, err
	}
	custom := make([]JobType, 0, len(all))
	for _, raw := range all {
		var jt JobType
		if err := json.Unmarshal([]byte(raw), &jt); err != nil {
			continue
		}
		custom = append(custom, jt)
	}
	out := make([]JobType, 0, len(BuiltIn)+len(custom))
	out = append(out, BuiltIn...)
	out = append(out, custom...)
	return out, nil
}

func (s *Store) Delete(ctx context.Context, name string) error {
	for _, b := range BuiltIn {
		if b.Name == name {
			return fmt.Errorf("cannot delete built-in job type %q", name)
		}
	}
	return s.redis.HDel(ctx, storeKey, name).Err()
}

// HandlerFor returns the worker plugin name for a job type, or empty if unknown.
func (s *Store) HandlerFor(ctx context.Context, jobType string) string {
	for _, b := range BuiltIn {
		if b.Name == jobType {
			return b.Handler
		}
	}
	jt, err := s.GetByName(ctx, jobType)
	if err != nil || jt == nil {
		return ""
	}
	return jt.Handler
}

// IsAllowed reports whether jobs of the given type may be submitted.
func (s *Store) IsAllowed(ctx context.Context, jobType string) bool {
	for _, b := range BuiltIn {
		if b.Name == jobType {
			return true
		}
	}
	_, err := s.GetByName(ctx, jobType)
	return err == nil
}
