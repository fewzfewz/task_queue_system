package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"task-queue-system/internal/api/middleware"
	"task-queue-system/internal/api/session"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
)

func newSessionHandler(rateLimit int) (*JobHandler, *session.Store) {
	store := models.NewInMemoryStore()
	q := &mockQueue{}
	svc := service.New(q, store, slog.Default(), 0)
	sessions := session.NewStore(time.Hour)
	h := New(svc, slog.Default(), "test-api-key", "admin", "admin123", sessions, "localhost:8081", rateLimit, 10)
	return h, sessions
}

func TestLogin_Success(t *testing.T) {
	h, _ := newSessionHandler(5)

	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "test-api-key") {
		t.Fatal("login response must not leak the API key")
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["role"] != "admin" {
		t.Errorf("expected role admin, got %v", body["role"])
	}
	if csrf, _ := body["csrf_token"].(string); csrf == "" {
		t.Error("expected a csrf_token in the response")
	}

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("expected tq_session cookie")
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie flags not hardened: %+v", cookie)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	h, _ := newSessionHandler(5)

	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"username":"admin","password":"nope"}`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLogin_RateLimited(t *testing.T) {
	h, _ := newSessionHandler(2)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"username":"admin","password":"nope"}`))
		rec := httptest.NewRecorder()
		h.Login(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"username":"admin","password":"nope"}`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 3: expected 429, got %d", rec.Code)
	}
}

func TestLogin_ViewerRole(t *testing.T) {
	h, _ := newSessionHandler(5)
	h.SetReadonlyCredentials("monitor", "monitor-pass")

	req := httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{"username":"monitor","password":"monitor-pass"}`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["role"] != "viewer" {
		t.Errorf("expected role viewer, got %v", body["role"])
	}
}

func TestLogout_RevokesSession(t *testing.T) {
	h, sessions := newSessionHandler(5)
	sess, _ := sessions.Create("alice", "admin")

	ctx := context.WithValue(context.Background(), middleware.ContextKeySession, sess)
	req := httptest.NewRequest("POST", "/api/v1/logout", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.Logout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if _, ok := sessions.Get(sess.ID); ok {
		t.Fatal("session should be revoked after logout")
	}
}

func TestGetSession(t *testing.T) {
	h, sessions := newSessionHandler(5)

	// No cookie.
	req := httptest.NewRequest("GET", "/api/v1/session", nil)
	rec := httptest.NewRecorder()
	h.GetSession(rec, req)
	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["authenticated"] != false {
		t.Fatalf("expected unauthenticated, got %v", body)
	}

	// Valid cookie.
	sess, _ := sessions.Create("alice", "viewer")
	req = httptest.NewRequest("GET", "/api/v1/session", nil)
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: sess.ID})
	rec = httptest.NewRecorder()
	h.GetSession(rec, req)
	body = nil
	json.NewDecoder(rec.Body).Decode(&body)
	if body["authenticated"] != true {
		t.Fatalf("expected authenticated, got %v", body)
	}
	if body["csrf_token"] != sess.CSRF {
		t.Errorf("expected CSRF token to be returned, got %v", body["csrf_token"])
	}
}

func TestJobEventsSSE_ClosesOnSessionExpiry(t *testing.T) {
	h, _ := newSessionHandler(5)
	h.sessions = session.NewStore(60 * time.Millisecond)
	h.sseCheckInterval = 20 * time.Millisecond
	sess, _ := h.sessions.Create("alice", "admin")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), middleware.ContextKeySession, sess)
		ctx = context.WithValue(ctx, middleware.ContextKeyAuthType, middleware.AuthTypeSession)
		h.JobEventsSSE(w, r.WithContext(ctx))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("SSE must not allow cross-origin access, got ACAO %q", got)
	}

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := resp.Body.Read(buf); err != nil {
				done <- err
				return
			}
		}
	}()

	select {
	case err := <-done:
		if err != io.EOF {
			t.Fatalf("expected EOF after session expiry, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not close after session expiry")
	}
}

func TestCircuitBreakerProxy_ForwardsKey(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"email":"closed"}`)
	}))
	defer worker.Close()

	h, _ := newSessionHandler(5)
	h.workerAddr = strings.TrimPrefix(worker.URL, "http://")

	req := httptest.NewRequest("GET", "/api/v1/circuit-breakers", nil)
	rec := httptest.NewRecorder()
	h.GetCircuitBreakers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("expected proxied body, got %q", rec.Body.String())
	}
}
