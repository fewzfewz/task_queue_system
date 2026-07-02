package limiter

import (
	"context"
	"time"
)

// RateLimiter defines the interface for controlling task execution flow.
type RateLimiter interface {
	// Wait blocks until the next task is allowed to proceed or the context is cancelled.
	Wait(ctx context.Context) error
}

// TokenBucketLimiter implements a simple thread-safe rate limiter using a channel
// and a background ticker to replenish tokens.
type TokenBucketLimiter struct {
	tokens chan struct{}
}

// NewTokenBucketLimiter creates a limiter that allows up to 'rate' operations per second.
// rate must be > 0.
func NewTokenBucketLimiter(rate float64) *TokenBucketLimiter {
	if rate <= 0 {
		return nil
	}

	// We use a buffer size based on the rate to handle minor bursts.
	size := int(rate)
	if size < 1 {
		size = 1
	}

	l := &TokenBucketLimiter{
		tokens: make(chan struct{}, size),
	}

	// Burst: start with a full bucket
	for i := 0; i < size; i++ {
		l.tokens <- struct{}{}
	}

	// Background replenishment loop
	interval := time.Duration(float64(time.Second) / rate)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			select {
			case l.tokens <- struct{}{}:
				// token added
			default:
				// bucket full
			}
		}
	}()

	return l
}

// Wait blocks until a token is available or context is cancelled.
// Context cancellation takes priority over available tokens.
func (l *TokenBucketLimiter) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.tokens:
		return nil
	}
}
