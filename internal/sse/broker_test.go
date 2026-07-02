package sse

import (
	"log/slog"
	"testing"
	"time"
)

func TestBrokerSubscribeUnsubscribe(t *testing.T) {
	b := NewBroker(slog.Default())

	ch := b.Subscribe()
	if b.ClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", b.ClientCount())
	}

	b.Unsubscribe(ch)
	if b.ClientCount() != 0 {
		t.Fatalf("expected 0 clients after unsubscribe, got %d", b.ClientCount())
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after unsubscribe")
		}
	default:
		t.Fatal("expected channel to be closed")
	}
}

func TestBrokerPublish(t *testing.T) {
	b := NewBroker(slog.Default())

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	ev := Event{
		JobID:    "job-1",
		Status:   "completed",
		Type:     "email",
		Tenant:   "tenant-a",
		Progress: 1.0,
	}

	b.Publish(ev)

	select {
	case msg := <-ch:
		if msg == "" {
			t.Fatal("expected non-empty message")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published event")
	}
}

func TestBrokerPublishToMultipleClients(t *testing.T) {
	b := NewBroker(slog.Default())

	ch1 := b.Subscribe()
	ch2 := b.Subscribe()
	defer b.Unsubscribe(ch1)
	defer b.Unsubscribe(ch2)

	b.Publish(Event{JobID: "j1", Status: "running", Type: "test"})

	for i, ch := range []chan string{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("client %d did not receive event", i)
		}
	}
}

func TestBrokerPublishToSlowClient(t *testing.T) {
	b := NewBroker(slog.Default())

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	for i := 0; i < 128; i++ {
		b.Publish(Event{JobID: "j1", Status: "running", Type: "test"})
	}

	b.Publish(Event{JobID: "last", Status: "done", Type: "test"})

	drained := 0
	for {
		select {
		case <-ch:
			drained++
		default:
			if drained < 64 {
				t.Fatalf("expected at least 64 buffered messages, got %d", drained)
			}
			return
		}
	}
}

func TestBrokerClientCount(t *testing.T) {
	b := NewBroker(slog.Default())

	if b.ClientCount() != 0 {
		t.Fatalf("expected 0, got %d", b.ClientCount())
	}

	ch1 := b.Subscribe()
	ch2 := b.Subscribe()

	if b.ClientCount() != 2 {
		t.Fatalf("expected 2, got %d", b.ClientCount())
	}

	b.Unsubscribe(ch1)
	if b.ClientCount() != 1 {
		t.Fatalf("expected 1, got %d", b.ClientCount())
	}

	b.Unsubscribe(ch2)
	if b.ClientCount() != 0 {
		t.Fatalf("expected 0, got %d", b.ClientCount())
	}
}
