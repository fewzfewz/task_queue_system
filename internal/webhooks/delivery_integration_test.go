package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type deliveryReceiver struct {
	status int
	secret string

	mu          sync.Mutex
	attempts    int
	times       []time.Time
	bodies      [][]byte
	methods     []string
	contentType []string
	sigValid    []bool
}

func (r *deliveryReceiver) handler(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.attempts++
	r.mu.Unlock()

	body, _ := io.ReadAll(req.Body)
	sig := req.Header.Get("X-Webhook-Signature")

	valid := false
	if r.secret != "" {
		valid = hmac.Equal([]byte("sha256="+sign(body, r.secret)), []byte(sig))
	}

	if r.secret != "" && !valid {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	r.mu.Lock()
	r.times = append(r.times, time.Now())
	r.bodies = append(r.bodies, body)
	r.methods = append(r.methods, req.Method)
	r.contentType = append(r.contentType, req.Header.Get("Content-Type"))
	r.sigValid = append(r.sigValid, valid)
	r.mu.Unlock()

	w.WriteHeader(r.status)
}

func (r *deliveryReceiver) attemptsCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts
}

func (r *deliveryReceiver) allBodies() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]byte(nil), r.bodies...)
}

func (r *deliveryReceiver) hits() []time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Time(nil), r.times...)
}

func (r *deliveryReceiver) lastBody() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bodies) == 0 {
		return nil
	}
	return r.bodies[len(r.bodies)-1]
}

func (r *deliveryReceiver) lastSigValid() (bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sigValid) == 0 {
		return false, false
	}
	return r.sigValid[len(r.sigValid)-1], true
}

func newTestDispatcher(rdb *redis.Client, timeout time.Duration, cfg DispatcherConfig) *Dispatcher {
	return &Dispatcher{
		redis:  rdb,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		client: &http.Client{Timeout: timeout},
		config: cfg,
	}
}

func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}

func baseEvent(url, secret, status string) Event {
	return Event{
		JobID:     "job-1",
		TenantID:  "tenant-1",
		Status:    status,
		Result:    map[string]interface{}{"output": "hello world"},
		Timestamp: time.Now().UTC().Truncate(time.Millisecond),
		URL:       url,
		Secret:    secret,
	}
}

func TestDeliveryHMACSignatureVerification(t *testing.T) {
	rdb := newTestRedis(t)
	rcv := &deliveryReceiver{status: http.StatusOK, secret: "receiver-secret"}
	srv := httptest.NewServer(http.HandlerFunc(rcv.handler))
	defer srv.Close()

	d := newTestDispatcher(rdb, 5*time.Second, DispatcherConfig{MaxRetries: 3, BaseDelay: 20 * time.Millisecond, MaxDelay: time.Second})
	ev := baseEvent(srv.URL, "receiver-secret", "completed")

	d.dispatch(context.Background(), ev, "msg-hmac-ok")

	if got := rcv.attemptsCount(); got != 1 {
		t.Fatalf("expected exactly 1 delivery attempt, got %d", got)
	}
	if valid, ok := rcv.lastSigValid(); !ok || !valid {
		t.Fatalf("expected receiver to validate signature, ok=%v valid=%v", ok, valid)
	}
}

func TestDeliveryRejectsWrongSecretWithoutRetry(t *testing.T) {
	rdb := newTestRedis(t)
	rcv := &deliveryReceiver{status: http.StatusOK, secret: "receiver-secret"}
	srv := httptest.NewServer(http.HandlerFunc(rcv.handler))
	defer srv.Close()

	d := newTestDispatcher(rdb, 5*time.Second, DispatcherConfig{MaxRetries: 5, BaseDelay: 10 * time.Millisecond, MaxDelay: time.Second})
	ev := baseEvent(srv.URL, "attacker-secret", "completed")

	d.dispatch(context.Background(), ev, "msg-hmac-bad")

	if got := rcv.attemptsCount(); got != 1 {
		t.Fatalf("expected client-error to be non-retried, got %d attempts", got)
	}
	if len(rcv.allBodies()) != 0 {
		t.Fatal("receiver must not accept deliveries with an invalid signature")
	}
}

func TestDeliveryNoSignatureHeaderWithoutSecret(t *testing.T) {
	rdb := newTestRedis(t)
	rcv := &deliveryReceiver{status: http.StatusOK}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rcv.mu.Lock()
		rcv.attempts++
		rcv.bodies = append(rcv.bodies, body)
		rcv.mu.Unlock()
		if sig := r.Header.Get("X-Webhook-Signature"); sig != "" {
			t.Errorf("expected no signature header, got %q", sig)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := newTestDispatcher(rdb, 5*time.Second, DispatcherConfig{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: time.Second})
	ev := baseEvent(srv.URL, "", "completed")

	d.dispatch(context.Background(), ev, "msg-nosig")

	if got := rcv.attemptsCount(); got != 1 {
		t.Fatalf("expected 1 attempt, got %d", got)
	}
}

func TestDeliveryPayloadIntegrity(t *testing.T) {
	cases := []struct {
		name   string
		status string
		result interface{}
		err    string
	}{
		{name: "completed", status: "completed", result: map[string]interface{}{"output": "hello world", "latency_ms": 12.5}},
		{name: "failed", status: "failed", result: "worker error: boom", err: "worker error: boom"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rdb := newTestRedis(t)
			rcv := &deliveryReceiver{status: http.StatusOK, secret: "payload-secret"}
			srv := httptest.NewServer(http.HandlerFunc(rcv.handler))
			defer srv.Close()

			ev := baseEvent(srv.URL, "payload-secret", tc.status)
			ev.JobID = "job-payload-1"
			ev.TenantID = "tenant-payload-1"
			ev.Result = tc.result
			ev.Error = tc.err
			ev.Timestamp = time.Unix(1700000000, 123456000).UTC()

			d := newTestDispatcher(rdb, 5*time.Second, DispatcherConfig{MaxRetries: 3, BaseDelay: 20 * time.Millisecond, MaxDelay: time.Second})
			d.dispatch(context.Background(), ev, "msg-payload-"+tc.status)

			wantPayload := WebhookPayload{
				JobID:     ev.JobID,
				TenantID:  ev.TenantID,
				Status:    ev.Status,
				Result:    ev.Result,
				Error:     ev.Error,
				Timestamp: ev.Timestamp,
			}
			want, err := json.Marshal(wantPayload)
			if err != nil {
				t.Fatalf("marshal expected body: %v", err)
			}
			if got := rcv.lastBody(); !bytes.Equal(got, want) {
				t.Fatalf("delivered body mismatch:\ngot  %s\nwant %s", got, want)
			}

			var decoded WebhookPayload
			if err := json.Unmarshal(rcv.lastBody(), &decoded); err != nil {
				t.Fatalf("delivered body is not valid JSON: %v", err)
			}
			if decoded.JobID != ev.JobID || decoded.TenantID != ev.TenantID || decoded.Status != ev.Status {
				t.Fatalf("identity fields mismatch: %+v", decoded)
			}
			if !decoded.Timestamp.Equal(ev.Timestamp) {
				t.Fatalf("timestamp mismatch: got %v want %v", decoded.Timestamp, ev.Timestamp)
			}

			gotResult, _ := json.Marshal(decoded.Result)
			wantResult, _ := json.Marshal(tc.result)
			if !bytes.Equal(gotResult, wantResult) {
				t.Fatalf("result mismatch: got %s want %s", gotResult, wantResult)
			}
			if decoded.Error != tc.err {
				t.Fatalf("error mismatch: got %q want %q", decoded.Error, tc.err)
			}

			rcv.mu.Lock()
			method, ct := rcv.methods[len(rcv.methods)-1], rcv.contentType[len(rcv.contentType)-1]
			rcv.mu.Unlock()
			if method != http.MethodPost {
				t.Fatalf("expected POST, got %s", method)
			}
			if ct != "application/json" {
				t.Fatalf("expected application/json content-type, got %s", ct)
			}
		})
	}
}

func TestDeliveryBackoffSchedule(t *testing.T) {
	rdb := newTestRedis(t)
	rcv := &deliveryReceiver{status: http.StatusInternalServerError, secret: "sched-secret"}
	srv := httptest.NewServer(http.HandlerFunc(rcv.handler))
	defer srv.Close()

	base := 25 * time.Millisecond
	d := newTestDispatcher(rdb, 5*time.Second, DispatcherConfig{MaxRetries: 3, BaseDelay: base, MaxDelay: time.Second})
	ev := baseEvent(srv.URL, "sched-secret", "completed")

	d.dispatch(context.Background(), ev, "msg-backoff")

	if got := rcv.attemptsCount(); got != 3 {
		t.Fatalf("expected 3 attempts with MaxRetries=3, got %d", got)
	}

	times := rcv.hits()
	for i, expected := range []time.Duration{2 * base, 4 * base} {
		interval := times[i+1].Sub(times[i])
		if interval < expected*6/10 || interval > expected*14/10 {
			t.Fatalf("attempt %d backoff interval %v outside expected window around %v", i+1, interval, expected)
		}
	}
}

func TestDeliveryGivesUpAfterMaxRetries(t *testing.T) {
	rdb := newTestRedis(t)
	rcv := &deliveryReceiver{status: http.StatusInternalServerError, secret: "giveup-secret"}
	srv := httptest.NewServer(http.HandlerFunc(rcv.handler))
	defer srv.Close()

	d := newTestDispatcher(rdb, 5*time.Second, DispatcherConfig{MaxRetries: 5, BaseDelay: 10 * time.Millisecond, MaxDelay: time.Second})
	ev := baseEvent(srv.URL, "giveup-secret", "completed")

	d.dispatch(context.Background(), ev, "msg-giveup")

	if got := rcv.attemptsCount(); got != 5 {
		t.Fatalf("expected exactly 5 attempts before giving up, got %d", got)
	}
}

func TestDeliverySucceedsAfterRetries(t *testing.T) {
	rdb := newTestRedis(t)
	rcv := &deliveryReceiver{status: http.StatusOK, secret: "retry-secret"}
	var serverHits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serverHits.Add(1) <= 2 {
			http.Error(w, "transient failure", http.StatusInternalServerError)
			return
		}
		rcv.handler(w, r)
	}))
	defer srv.Close()

	d := newTestDispatcher(rdb, 5*time.Second, DispatcherConfig{MaxRetries: 5, BaseDelay: 20 * time.Millisecond, MaxDelay: time.Second})
	ev := baseEvent(srv.URL, "retry-secret", "completed")

	d.dispatch(context.Background(), ev, "msg-succeed-after-retries")

	if got := serverHits.Load(); got != 3 {
		t.Fatalf("expected 3 server hits (2 failures then success), got %d", got)
	}
	if len(rcv.allBodies()) != 1 {
		t.Fatalf("expected exactly 1 accepted delivery, got %d", len(rcv.allBodies()))
	}
}

func TestDeliveryTimeoutRetriesAndGivesUp(t *testing.T) {
	rdb := newTestRedis(t)
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := newTestDispatcher(rdb, 100*time.Millisecond, DispatcherConfig{MaxRetries: 3, BaseDelay: 20 * time.Millisecond, MaxDelay: time.Second})
	ev := baseEvent(srv.URL, "timeout-secret", "completed")

	d.dispatch(context.Background(), ev, "msg-timeout")

	waitFor(t, 3*time.Second, "all timed-out attempts to complete", func() bool {
		return hits.Load() == 3
	})
}

func TestDeliveryClientErrorDoesNotRetry(t *testing.T) {
	rdb := newTestRedis(t)
	rcv := &deliveryReceiver{status: http.StatusBadRequest, secret: "clienterr-secret"}
	srv := httptest.NewServer(http.HandlerFunc(rcv.handler))
	defer srv.Close()

	d := newTestDispatcher(rdb, 5*time.Second, DispatcherConfig{MaxRetries: 5, BaseDelay: 10 * time.Millisecond, MaxDelay: time.Second})
	ev := baseEvent(srv.URL, "clienterr-secret", "completed")

	d.dispatch(context.Background(), ev, "msg-4xx")

	if got := rcv.attemptsCount(); got != 1 {
		t.Fatalf("expected client error to be non-retried, got %d attempts", got)
	}
}

func TestConcurrentDeliveriesNoLeaks(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping: miniredis could not start")
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), PoolSize: 4})
	t.Cleanup(func() { _ = rdb.Close() })

	var mu sync.Mutex
	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var ev Event
		_ = json.Unmarshal(body, &ev)
		mu.Lock()
		received = append(received, ev.JobID)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := &http.Transport{MaxIdleConns: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 5 * time.Second}
	d := &Dispatcher{
		redis:  rdb,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		client: &http.Client{Transport: transport, Timeout: 5 * time.Second},
		config: DispatcherConfig{MaxRetries: 5, BaseDelay: 5 * time.Millisecond, MaxDelay: time.Second},
	}

	var warmWg sync.WaitGroup
	for i := 0; i < 4; i++ {
		warmWg.Add(1)
		go func(i int) {
			defer warmWg.Done()
			ev := baseEvent(srv.URL, "conc-secret", "completed")
			ev.JobID = fmt.Sprintf("warm-%d", i)
			d.dispatch(context.Background(), ev, fmt.Sprintf("warm-msg-%d", i))
		}(i)
	}
	warmWg.Wait()

	const n = 50
	baseline := runtime.NumGoroutine()
	mu.Lock()
	received = nil
	mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ev := baseEvent(srv.URL, "conc-secret", "completed")
			ev.JobID = fmt.Sprintf("job-%d", i)
			d.dispatch(context.Background(), ev, fmt.Sprintf("msg-%d", i))
		}(i)
	}
	wg.Wait()

	if len(received) != n {
		t.Fatalf("expected %d deliveries, got %d", n, len(received))
	}
	seen := make(map[string]bool, len(received))
	for _, id := range received {
		seen[id] = true
	}
	for i := 0; i < n; i++ {
		if !seen[fmt.Sprintf("job-%d", i)] {
			t.Fatalf("delivery for job-%d missing", i)
		}
	}

	waitFor(t, 5*time.Second, "goroutine count to return to baseline", func() bool {
		return runtime.NumGoroutine() <= baseline+3
	})
}
