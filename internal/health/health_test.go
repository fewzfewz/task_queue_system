package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestCheckerLive(t *testing.T) {
	c := NewChecker("api", nil)
	rr := httptest.NewRecorder()
	c.Live(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestCheckerReady(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		c := NewChecker("api", fakePinger{})
		rr := httptest.NewRecorder()
		c.Ready(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("not ready", func(t *testing.T) {
		c := NewChecker("api", fakePinger{err: errors.New("down")})
		rr := httptest.NewRecorder()
		c.Ready(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rr.Code)
		}
	})
}
