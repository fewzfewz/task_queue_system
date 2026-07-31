package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"task-queue-system/internal/api/session"
	"task-queue-system/internal/config"
)

func TestRequireAuth_APIKey(t *testing.T) {
	cfg := &config.Config{ApiKey: "key"}
	sessions := session.NewStore(time.Hour)
	h := RequireAuth(cfg, sessions)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RoleFromContext(r.Context()); got != RoleAdmin {
			t.Errorf("expected role admin, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/jobs", nil)
	req.Header.Set("X-API-Key", "key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireAuth_SessionCookie(t *testing.T) {
	cfg := &config.Config{ApiKey: "key"}
	sessions := session.NewStore(time.Hour)
	sess, _ := sessions.Create("alice", RoleViewer)

	h := RequireAuth(cfg, sessions)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := SessionFromContext(r.Context())
		if got == nil || got.ID != sess.ID {
			t.Error("expected session in context")
		}
		if RoleFromContext(r.Context()) != RoleViewer {
			t.Errorf("expected viewer role, got %q", RoleFromContext(r.Context()))
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/jobs", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireAuth_RejectsAnonymous(t *testing.T) {
	cfg := &config.Config{ApiKey: "key"}
	sessions := session.NewStore(time.Hour)
	h := RequireAuth(cfg, sessions)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not run")
	}))

	req := httptest.NewRequest("GET", "/jobs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuth_RejectsExpiredSession(t *testing.T) {
	cfg := &config.Config{ApiKey: "key"}
	sessions := session.NewStore(10 * time.Millisecond)
	sess, _ := sessions.Create("alice", RoleAdmin)

	h := RequireAuth(cfg, sessions)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not run")
	}))

	time.Sleep(20 * time.Millisecond)
	req := httptest.NewRequest("GET", "/jobs", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireRole(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})
	h := RequireRole(RoleAdmin)(inner)

	ctx := context.WithValue(context.Background(), ContextKeyRole, RoleViewer)
	req := httptest.NewRequest("POST", "/jobs", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("viewer on admin route: expected 403, got %d", rec.Code)
	}
}

func TestCSRFProtect(t *testing.T) {
	sessions := session.NewStore(time.Hour)
	sess, _ := sessions.Create("alice", RoleAdmin)

	withSession := func(method, csrfHeader string) *http.Request {
		ctx := context.WithValue(context.Background(), ContextKeySession, sess)
		ctx = context.WithValue(ctx, ContextKeyAuthType, AuthTypeSession)
		req := httptest.NewRequest(method, "/jobs", nil).WithContext(ctx)
		if csrfHeader != "" {
			req.Header.Set("X-CSRF-Token", csrfHeader)
		}
		return req
	}

	run := func(req *http.Request) int {
		rec := httptest.NewRecorder()
		CSRFProtect()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, req)
		return rec.Code
	}

	if code := run(withSession(http.MethodPost, sess.CSRF)); code != http.StatusOK {
		t.Errorf("valid CSRF token: expected 200, got %d", code)
	}
	if code := run(withSession(http.MethodPost, "wrong")); code != http.StatusForbidden {
		t.Errorf("invalid CSRF token: expected 403, got %d", code)
	}
	if code := run(withSession(http.MethodPost, "")); code != http.StatusForbidden {
		t.Errorf("missing CSRF token: expected 403, got %d", code)
	}
	// GET is never CSRF-checked.
	if code := run(withSession(http.MethodGet, "")); code != http.StatusOK {
		t.Errorf("GET without CSRF: expected 200, got %d", code)
	}
}

func TestCSRFProtect_SkipsAPIKey(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyAuthType, AuthTypeAPIKey)
	req := httptest.NewRequest("POST", "/jobs", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	CSRFProtect()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("API-key mutation without CSRF: expected 200, got %d", rec.Code)
	}
}
