package session

import (
	"testing"
	"time"
)

func TestStore_CreateGet(t *testing.T) {
	s := NewStore(time.Hour)
	sess, err := s.Create("alice", "admin")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if sess.ID == "" || sess.CSRF == "" {
		t.Fatal("expected non-empty session ID and CSRF token")
	}
	if sess.Role != "admin" {
		t.Fatalf("expected role admin, got %q", sess.Role)
	}

	got, ok := s.Get(sess.ID)
	if !ok {
		t.Fatal("expected session to be retrievable")
	}
	if got.Username != "alice" {
		t.Fatalf("expected username alice, got %q", got.Username)
	}
	if got.CSRF != sess.CSRF {
		t.Fatal("CSRF token must be stable across reads")
	}
}

func TestStore_Expiry(t *testing.T) {
	s := NewStore(20 * time.Millisecond)
	sess, _ := s.Create("bob", "viewer")

	if _, ok := s.Get(sess.ID); !ok {
		t.Fatal("expected session valid immediately after creation")
	}

	time.Sleep(40 * time.Millisecond)
	if _, ok := s.Get(sess.ID); ok {
		t.Fatal("expected session to be expired after TTL")
	}
}

func TestStore_Delete(t *testing.T) {
	s := NewStore(time.Hour)
	sess, _ := s.Create("carol", "admin")

	s.Delete(sess.ID)
	if _, ok := s.Get(sess.ID); ok {
		t.Fatal("expected deleted session to be gone")
	}
}

func TestStore_Cleanup(t *testing.T) {
	s := NewStore(10 * time.Millisecond)
	_, _ = s.Create("dave", "admin")
	_, _ = s.Create("erin", "admin")

	time.Sleep(30 * time.Millisecond)
	if n := s.Cleanup(); n != 2 {
		t.Fatalf("expected 2 cleaned sessions, got %d", n)
	}
	if n := s.Cleanup(); n != 0 {
		t.Fatalf("expected 0 further cleanups, got %d", n)
	}
}

func TestLoginLimiter(t *testing.T) {
	l := NewLoginLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !l.Allow("10.0.0.1") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("attempt 4 should be blocked")
	}
	// Other IPs are unaffected.
	if !l.Allow("10.0.0.2") {
		t.Fatal("different key should be allowed")
	}
}

func TestLoginLimiter_WindowReset(t *testing.T) {
	l := NewLoginLimiter(1, 10*time.Millisecond)

	if !l.Allow("10.0.0.1") {
		t.Fatal("first attempt should be allowed")
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("second attempt should be blocked")
	}

	time.Sleep(15 * time.Millisecond)
	if !l.Allow("10.0.0.1") {
		t.Fatal("attempt after window should be allowed")
	}
}
