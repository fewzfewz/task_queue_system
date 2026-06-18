package webhooks

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
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

func TestDispatcherStartIntegration(t *testing.T) {
	if os.Getenv("RUN_QUEUE_INTEGRATION") != "1" {
		t.Skip("RUN_QUEUE_INTEGRATION=1 is required for this integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available on localhost:6379, skipping")
	}
	defer rdb.FlushAll(ctx)

	var delivered atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher(rdb, slog.New(slog.NewTextHandler(io.Discard, nil)))

	go d.Start(ctx)

	// Give the dispatcher time to create the consumer group
	time.Sleep(500 * time.Millisecond)

	ev := Event{
		JobID:     "job-start-1",
		TenantID:  "tenant-1",
		Status:    "completed",
		Result:    map[string]string{"ok": "yes"},
		Timestamp: time.Now().UTC(),
		URL:       srv.URL,
		Secret:    "",
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	if err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamKey,
		Values: map[string]interface{}{"data": string(data)},
	}).Err(); err != nil {
		t.Fatalf("xadd failed: %v", err)
	}

	// Wait for delivery
	deadline := time.After(5 * time.Second)
	for {
		if delivered.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for webhook delivery")
		case <-time.After(100 * time.Millisecond):
		}
	}

	if n := delivered.Load(); n != 1 {
		t.Fatalf("expected 1 delivery, got %d", n)
	}
}
