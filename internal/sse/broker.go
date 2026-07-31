package sse

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// Event represents a server-sent event.
type Event struct {
	// Kind categorizes the event so clients can route updates without
	// guessing from status alone. Values: "job", "rate_limit",
	// "circuit_breaker", "dlq".
	Kind string `json:"kind,omitempty"`
	JobID  string      `json:"job_id"`
	Status string      `json:"status"`
	Type   string      `json:"type"`
	Tenant string      `json:"tenant_id,omitempty"`
	Error  string      `json:"error,omitempty"`
	Progress float64   `json:"progress,omitempty"`
}

// Broker manages SSE client connections and broadcasts.
type Broker struct {
	mu       sync.RWMutex
	clients  map[chan string]struct{}
	logger   *slog.Logger
}

// NewBroker creates an SSE broker.
func NewBroker(logger *slog.Logger) *Broker {
	return &Broker{
		clients: make(map[chan string]struct{}),
		logger:  logger,
	}
}

// Subscribe registers a new client channel for SSE events.
func (b *Broker) Subscribe() chan string {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan string, 64)
	b.clients[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a client channel.
func (b *Broker) Unsubscribe(ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.clients, ch)
	close(ch)
}

// Publish sends a structured event to all connected clients.
func (b *Broker) Publish(ev Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		b.logger.Error("sse broker: failed to marshal event", "error", err)
		return
	}
	msg := fmt.Sprintf("data: %s\n\n", string(data))

	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
			// Client too slow; drop message.
		}
	}
}

// ClientCount returns the number of connected SSE clients.
func (b *Broker) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
