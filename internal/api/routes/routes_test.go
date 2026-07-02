package routes

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"task-queue-system/internal/config"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/storage/models"
)

func TestNewRouter_RegistersRoutes(t *testing.T) {
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	cfg := config.Load()
	handler := NewRouter(mq, store, slog.Default(), cfg, nil)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	tests := []struct {
		method string
		path   string
		code   int
	}{
		{"GET", "/", 200},
		{"GET", "/ui", 200},
		{"GET", "/jobs/nonexistent", 404},
		{"GET", "/metrics", 200},
		{"GET", "/workers", 200},
		{"GET", "/admin/dlq", 200},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.code {
				t.Errorf("%s %s: expected status %d, got %d", tt.method, tt.path, tt.code, rec.Code)
			}
		})
	}
}

func TestNewRouter_DLQEndpointsRequireAuth(t *testing.T) {
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	cfg := config.Load()
	handler := NewRouter(mq, store, slog.Default(), cfg, nil)

	dlqTests := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/dlq"},
		{"DELETE", "/api/v1/dlq"},
		{"GET", "/api/v1/dlq/job-1"},
		{"POST", "/api/v1/dlq/job-1/replay"},
		{"DELETE", "/api/v1/dlq/job-1"},
	}

	for _, tt := range dlqTests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401, got %d", tt.method, tt.path, rec.Code)
			}
		})
	}
}

func TestNewRouter_SwaggerUI(t *testing.T) {
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	cfg := config.Load()
	handler := NewRouter(mq, store, slog.Default(), cfg, nil)

	req := httptest.NewRequest("GET", "/swagger/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 && rec.Code != 301 {
		t.Errorf("expected 200 or 301, got %d", rec.Code)
	}
}
