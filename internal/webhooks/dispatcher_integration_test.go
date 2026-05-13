package webhooks

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDispatcherSendIntegration(t *testing.T) {
	var gotSig string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Webhook-Signature")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := &Dispatcher{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		client: srv.Client(),
	}

	ev := Event{
		JobID:     "job-1",
		TenantID:  "tenant-1",
		Status:    "completed",
		Result:    map[string]string{"ok": "yes"},
		Timestamp: time.Unix(1700000000, 0).UTC(),
		URL:       srv.URL,
		Secret:    "secret",
	}

	if err := d.send(context.Background(), ev); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if gotSig == "" {
		t.Fatal("expected webhook signature header")
	}
	if len(gotBody) == 0 {
		t.Fatal("expected body to be delivered")
	}
}
