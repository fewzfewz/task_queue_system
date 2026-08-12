package routes

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"task-queue-system/internal/config"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/storage/models"
)

func TestNewRouter_RegistersRoutes(t *testing.T) {
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	cfg := config.Load()
	handler := NewRouter(mq, store, slog.Default(), cfg, nil, nil)
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
		{"GET", "/login", 200},
		{"GET", "/admin/dlq", 200},
		{"GET", "/metrics", 200},
		{"GET", "/api/v1/session", 200},
		{"GET", "/api/v1/stats", 401},
		{"GET", "/workers", 401},
		{"GET", "/jobs/nonexistent", 401},
		{"GET", "/events", 401},
		{"GET", "/api/v1/circuit-breakers", 401},
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

func TestNewRouter_DataEndpointsRequireAuth(t *testing.T) {
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	cfg := config.Load()
	handler := NewRouter(mq, store, slog.Default(), cfg, nil, nil)

	dlqTests := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/dlq"},
		{"DELETE", "/api/v1/dlq"},
		{"GET", "/api/v1/dlq/job-1"},
		{"POST", "/api/v1/dlq/job-1/replay"},
		{"DELETE", "/api/v1/dlq/job-1"},
		{"GET", "/jobs"},
		{"POST", "/jobs"},
		{"GET", "/jobs/job-1"},
		{"PATCH", "/jobs/job-1/progress"},
		{"POST", "/jobs/job-1/cancel"},
		{"POST", "/api/v1/circuit-breakers/reset/email"},
		{"GET", "/api/v1/circuit-breakers"},
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

func TestNewRouter_APIKeyAuth(t *testing.T) {
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	cfg := config.Load()
	handler := NewRouter(mq, store, slog.Default(), cfg, nil, nil)

	req := httptest.NewRequest("GET", "/jobs/nonexistent", nil)
	req.Header.Set("X-API-Key", cfg.ApiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Authenticated: the job simply does not exist.
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /jobs/nonexistent with valid key: expected 404, got %d", rec.Code)
	}
}

func TestNewRouter_LoginSessionFlow(t *testing.T) {
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	cfg := config.Load()
	handler := NewRouter(mq, store, slog.Default(), cfg, nil, nil)

	// Login as admin.
	login := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"username":"admin","password":"admin123"}`))
	login.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, login)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "tq_session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("login did not set the tq_session cookie")
	}
	if !sessionCookie.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if !sessionCookie.Secure {
		t.Error("session cookie must be Secure")
	}

	// Session cookie must not contain the API key.
	if strings.Contains(loginRec.Body.String(), cfg.ApiKey) {
		t.Fatal("login response leaked the API key")
	}

	// Cookie-authenticated read.
	read := httptest.NewRequest("GET", "/jobs/nonexistent", nil)
	read.AddCookie(sessionCookie)
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, read)
	if readRec.Code != http.StatusNotFound {
		t.Fatalf("cookie-authenticated read: expected 404, got %d", readRec.Code)
	}

	// State-changing action without CSRF token must be rejected.
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest("DELETE", "/api/v1/dlq", nil)
	csrfReq.AddCookie(sessionCookie)
	handler.ServeHTTP(csrfRec, csrfReq)
	if csrfRec.Code != http.StatusForbidden {
		t.Fatalf("mutation without CSRF: expected 403, got %d", csrfRec.Code)
	}
}

func TestNewRouter_LoginBadCredentials(t *testing.T) {
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	cfg := config.Load()
	handler := NewRouter(mq, store, slog.Default(), cfg, nil, nil)

	login := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	login.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, login)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestNewRouter_SwaggerUI(t *testing.T) {
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	cfg := config.Load()
	handler := NewRouter(mq, store, slog.Default(), cfg, nil, nil)

	req := httptest.NewRequest("GET", "/swagger/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 && rec.Code != 301 {
		t.Errorf("expected 200 or 301, got %d", rec.Code)
	}
}
