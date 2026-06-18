package plugin

import (
	"fmt"
	"sync"
	"time"

	"task-queue-system/internal/metrics"
)

type BreakerState int

const (
	StateClosed   BreakerState = iota
	StateOpen     BreakerState = iota
	StateHalfOpen BreakerState = iota
)

type pluginBreaker struct {
	failures    int
	lastFailure time.Time
	state       BreakerState
}

// CircuitBreaker tracks consecutive failures per plugin type and
// prevents execution when a threshold is exceeded.
type CircuitBreaker struct {
	mu       sync.RWMutex
	plugins  map[string]*pluginBreaker

	threshold int
	cooldown  time.Duration
}

// NewCircuitBreaker creates a circuit breaker.
// threshold: consecutive failures before opening the circuit.
// cooldown:  time before transitioning from open to half-open.
func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &CircuitBreaker{
		plugins:   make(map[string]*pluginBreaker),
		threshold: threshold,
		cooldown:  cooldown,
	}
}

// IsAllowed checks whether execution is permitted for the given plugin type.
func (cb *CircuitBreaker) IsAllowed(jobType string) bool {
	cb.mu.RLock()
	pb, ok := cb.plugins[jobType]
	cb.mu.RUnlock()

	if !ok {
		return true
	}

	switch pb.state {
	case StateClosed:
		return true
	case StateHalfOpen:
		return true
	case StateOpen:
		if time.Since(pb.lastFailure) > cb.cooldown {
			cb.mu.Lock()
			pb.state = StateHalfOpen
			cb.mu.Unlock()
			return true
		}
		return false
	}
	return true
}

// RecordSuccess resets the failure count for the plugin type.
func (cb *CircuitBreaker) RecordSuccess(jobType string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	pb, ok := cb.plugins[jobType]
	if !ok {
		return
	}
	pb.failures = 0
	pb.state = StateClosed
	metrics.CircuitBreakerOpen.WithLabelValues(jobType).Set(0)
}

// RecordFailure increments the failure count and may open the circuit.
func (cb *CircuitBreaker) RecordFailure(jobType string, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	pb, ok := cb.plugins[jobType]
	if !ok {
		pb = &pluginBreaker{}
		cb.plugins[jobType] = pb
	}

		pb.failures++
	pb.lastFailure = time.Now()

	if pb.failures >= cb.threshold {
		pb.state = StateOpen
		metrics.CircuitBreakerOpen.WithLabelValues(jobType).Set(1)
	}
}

// State returns the current breaker state for a plugin type.
func (cb *CircuitBreaker) State(jobType string) BreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if pb, ok := cb.plugins[jobType]; ok {
		return pb.state
	}
	return StateClosed
}

// Reset forcefully resets the breaker for the given plugin type back to closed.
func (cb *CircuitBreaker) Reset(jobType string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if pb, ok := cb.plugins[jobType]; ok {
		pb.failures = 0
		pb.state = StateClosed
		metrics.CircuitBreakerOpen.WithLabelValues(jobType).Set(0)
	}
}

// Status returns a human-readable description per plugin type.
func (cb *CircuitBreaker) Status() map[string]string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	out := make(map[string]string, len(cb.plugins))
	for t, pb := range cb.plugins {
		var label string
		switch pb.state {
		case StateClosed:
			label = "closed"
		case StateOpen:
			label = fmt.Sprintf("open (%d/%d failures)", pb.failures, cb.threshold)
		case StateHalfOpen:
			label = "half-open"
		}
		out[t] = label
	}
	return out
}
