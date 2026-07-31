// Package session provides server-side sessions and a login rate limiter
// used to authenticate the operator UI without exposing the shared API key.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Session is a server-side operator session. The browser only ever holds the
// opaque session ID in an httpOnly cookie; the CSRF token is returned to the
// page so it can be echoed back on state-changing requests.
type Session struct {
	ID        string
	Username  string
	Role      string
	CSRF      string
	ExpiresAt time.Time
}

// Store is an in-memory session store with fixed TTL expiry.
type Store struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]*Session
}

// NewStore creates a session store whose sessions expire after ttl.
func NewStore(ttl time.Duration) *Store {
	return &Store{
		ttl:   ttl,
		items: make(map[string]*Session),
	}
}

// TTL returns the configured session lifetime.
func (s *Store) TTL() time.Duration { return s.ttl }

// Create issues a new session for the given username and role.
func (s *Store) Create(username, role string) (*Session, error) {
	id, err := newToken(32)
	if err != nil {
		return nil, err
	}
	csrf, err := newToken(32)
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:        id,
		Username:  username,
		Role:      role,
		CSRF:      csrf,
		ExpiresAt: time.Now().Add(s.ttl),
	}

	s.mu.Lock()
	s.items[id] = sess
	s.mu.Unlock()
	return sess, nil
}

// Get returns the session if it exists and has not expired.
func (s *Store) Get(id string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.items[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		delete(s.items, id)
		return nil, false
	}
	return sess, true
}

// Touch extends the session lifetime by the configured TTL. Returns false if
// the session does not exist or has already expired.
func (s *Store) Touch(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.items[id]
	if !ok {
		return false
	}
	if time.Now().After(sess.ExpiresAt) {
		delete(s.items, id)
		return false
	}
	sess.ExpiresAt = time.Now().Add(s.ttl)
	return true
}

// Delete removes a session, invalidating it immediately.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.items, id)
	s.mu.Unlock()
}

// Cleanup removes all expired sessions. Call periodically.
func (s *Store) Cleanup() int {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for id, sess := range s.items {
		if now.After(sess.ExpiresAt) {
			delete(s.items, id)
			n++
		}
	}
	return n
}

func newToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// LoginLimiter is a simple per-key (client IP) rate limiter used to slow down
// online brute-force attempts against the login endpoint.
type LoginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

// NewLoginLimiter allows at most max attempts per window per key.
func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	if max <= 0 {
		max = 5
	}
	if window <= 0 {
		window = time.Minute
	}
	return &LoginLimiter{
		max:    max,
		window: window,
		hits:   make(map[string][]time.Time),
	}
}

// Allow records an attempt for the key and reports whether it is permitted.
func (l *LoginLimiter) Allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	l.hits[key] = recent

	if len(recent) >= l.max {
		return false
	}
	l.hits[key] = append(l.hits[key], now)
	return true
}
