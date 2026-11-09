package session

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

func TestRedisStore_MultiInstance(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping: miniredis could not start")
	}
	defer mr.Close()

	rdbPodA := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdbPodA.Close()
	storePodA := NewRedisStore(rdbPodA, time.Hour)

	rdbPodB := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdbPodB.Close()
	storePodB := NewRedisStore(rdbPodB, time.Hour)

	// Pod A creates a session for user alice
	sess, err := storePodA.Create("alice", "admin")
	if err != nil {
		t.Fatalf("failed to create session on Pod A: %v", err)
	}

	// Pod B retrieves the session created by Pod A
	retrieved, ok := storePodB.Get(sess.ID)
	if !ok {
		t.Fatal("expected Pod B to retrieve session created on Pod A")
	}
	if retrieved.Username != "alice" || retrieved.Role != "admin" {
		t.Fatalf("expected alice/admin on Pod B, got %s/%s", retrieved.Username, retrieved.Role)
	}
	if retrieved.CSRF != sess.CSRF {
		t.Fatalf("expected matching CSRF token across pods")
	}

	// Pod B touches the session
	if !storePodB.Touch(sess.ID) {
		t.Fatal("expected Pod B to successfully touch session")
	}

	// Pod B deletes the session
	storePodB.Delete(sess.ID)

	// Pod A verifies the session is immediately gone
	if _, ok := storePodA.Get(sess.ID); ok {
		t.Fatal("expected session deleted by Pod B to be gone on Pod A")
	}
}

func TestRedisStore_Expiry(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping: miniredis could not start")
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	s := NewRedisStore(rdb, 50*time.Millisecond)

	sess, err := s.Create("bob", "viewer")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if _, ok := s.Get(sess.ID); !ok {
		t.Fatal("expected session to exist immediately")
	}

	mr.FastForward(100 * time.Millisecond)

	if _, ok := s.Get(sess.ID); ok {
		t.Fatal("expected session to expire after TTL")
	}
}
