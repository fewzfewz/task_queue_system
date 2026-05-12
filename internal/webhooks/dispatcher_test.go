package webhooks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"log/slog"
)

func TestDispatcher_Integration(t *testing.T) {
	// ── 1. Setup Mock Redis ───────────────────────────────────────────────────
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// ── 2. Setup Mock Webhook Target ──────────────────────────────────────────
	var mu sync.Mutex
	receivedBody := []byte{}
	receivedHeader := ""
	receivedCount := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		receivedCount++
		receivedHeader = r.Header.Get("X-Webhook-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// ── 3. Initialise Dispatcher ─────────────────────────────────────────────
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dispatcher := NewDispatcher(rdb, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go dispatcher.Start(ctx)

	// ── 4. Publish Event ──────────────────────────────────────────────────────
	secret := "test-secret"
	event := Event{
		JobID:     "job-123",
		TenantID:  "tenant-456",
		Status:    "completed",
		Result:    map[string]string{"foo": "bar"},
		Timestamp: time.Now().UTC(),
		URL:       ts.URL,
		Secret:    secret,
	}

	data, _ := json.Marshal(event)
	err = rdb.XAdd(context.Background(), &redis.XAddArgs{
		Stream: StreamKey,
		Values: map[string]interface{}{"data": string(data)},
	}).Err()
	if err != nil {
		t.Fatalf("failed to add to stream: %v", err)
	}

	// ── 5. Assert Delivery ────────────────────────────────────────────────────
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := receivedCount
		mu.Unlock()
		if count > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedCount != 1 {
		t.Errorf("expected exactly 1 webhook delivery, got %d", receivedCount)
	}

	// Validate Signature
	expectedSig := sign(receivedBody, secret)
	if receivedHeader != "sha256="+expectedSig {
		t.Errorf("invalid signature header: got %s, expected sha256=%s", receivedHeader, expectedSig)
	}

	// Validate Body JSON
	var receivedEvent map[string]interface{}
	if err := json.Unmarshal(receivedBody, &receivedEvent); err != nil {
		t.Fatalf("failed to parse received body: %v", err)
	}
	if receivedEvent["job_id"] != "job-123" {
		t.Errorf("expected job_id job-123, got %v", receivedEvent["job_id"])
	}
}
