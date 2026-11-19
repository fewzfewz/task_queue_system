package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestLogger_Unwrap(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mw := RequestLogger(logger)

	var unwrappedOk bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		// SetWriteDeadline requires accessing the underlying response writer or returning ErrNotSupported.
		// If Unwrap() is implemented, ResponseController inspects the underlying writer.
		err := rc.SetWriteDeadline(time.Time{})
		// On httptest.ResponseRecorder it returns http.ErrNotSupported, but Unwrap works.
		if u, ok := w.(interface{ Unwrap() http.ResponseWriter }); ok {
			unwrappedOk = (u.Unwrap() != nil)
		}
		_ = err
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test-unwrap", nil)
	handler.ServeHTTP(rec, req)

	if !unwrappedOk {
		t.Errorf("expected responseWriter to implement Unwrap()")
	}
	if rec.Code != http.StatusTeapot {
		t.Errorf("expected 418, got %d", rec.Code)
	}
}
