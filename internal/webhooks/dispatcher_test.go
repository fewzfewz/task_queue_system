package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSign(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	secret := "test-secret"

	got := sign(body, secret)
	want := "84cc33df716ed0b0598f07437c94069ace3730358778a592bd6bbd1423d111f3"

	if got != want {
		t.Fatalf("unexpected signature: got %s want %s", got, want)
	}
}

func TestDispatcherSendSetsWebhookSignature(t *testing.T) {
	var capturedBody []byte
	var capturedHeader string

	dispatcher := &Dispatcher{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		client: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				capturedHeader = r.Header.Get("X-Webhook-Signature")
				var err error
				capturedBody, err = io.ReadAll(r.Body)
				if err != nil {
					return nil, err
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(nil)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	ev := Event{
		JobID:     "job-123",
		TenantID:  "tenant-456",
		Status:    "completed",
		Result:    map[string]string{"foo": "bar"},
		Timestamp: time.Unix(1700000000, 0).UTC(),
		URL:       "http://example.test/webhook",
		Secret:    "test-secret",
	}

	status, err := dispatcher.send(context.Background(), ev)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	expectedBody, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("failed to marshal expected body: %v", err)
	}

	if string(capturedBody) != string(expectedBody) {
		t.Fatalf("unexpected webhook body: got %s want %s", string(capturedBody), string(expectedBody))
	}

	expectedSig := "sha256=" + sign(expectedBody, ev.Secret)
	if capturedHeader != expectedSig {
		t.Fatalf("unexpected signature header: got %s want %s", capturedHeader, expectedSig)
	}
}
