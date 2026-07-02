package webhooks

import (
	"context"
	"net"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("skipping: local TCP listeners not permitted")
	}
	_ = ln.Close()

	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping: miniredis could not start")
	}
	t.Cleanup(mr.Close)

	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestWebhookStoreCreateAndGetByID(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewWebhookStore(rdb)
	ctx := context.Background()

	w, err := s.Create(ctx, "tenant-a", "https://example.com/hook", "mysecret", []string{"completed"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if w.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if w.TenantID != "tenant-a" {
		t.Fatalf("expected tenant-a, got %s", w.TenantID)
	}
	if w.URL != "https://example.com/hook" {
		t.Fatalf("expected https://example.com/hook, got %s", w.URL)
	}

	got, err := s.GetByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.ID != w.ID {
		t.Fatalf("expected %s, got %s", w.ID, got.ID)
	}
}

func TestWebhookStoreCreateDefaultEvents(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewWebhookStore(rdb)
	ctx := context.Background()

	w, err := s.Create(ctx, "tenant-b", "https://example.com/hook", "", nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if len(w.Events) != 2 {
		t.Fatalf("expected 2 default events, got %d", len(w.Events))
	}
}

func TestWebhookStoreListFilterByTenant(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewWebhookStore(rdb)
	ctx := context.Background()

	_, _ = s.Create(ctx, "tenant-a", "https://a.com/hook", "", nil)
	_, _ = s.Create(ctx, "tenant-b", "https://b.com/hook", "", nil)

	all, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 webhooks, got %d", len(all))
	}

	tenantA, err := s.List(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("List tenant-a failed: %v", err)
	}
	if len(tenantA) != 1 {
		t.Fatalf("expected 1 webhook for tenant-a, got %d", len(tenantA))
	}
}

func TestWebhookStoreUpdate(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewWebhookStore(rdb)
	ctx := context.Background()

	w, _ := s.Create(ctx, "tenant-a", "https://old.com", "", nil)

	updated, err := s.Update(ctx, w.ID, "https://new.com", "newsecret", []string{"failed"})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.URL != "https://new.com" {
		t.Fatalf("expected https://new.com, got %s", updated.URL)
	}

	got, _ := s.GetByID(ctx, w.ID)
	if got.URL != "https://new.com" {
		t.Fatalf("expected https://new.com from GetByID, got %s", got.URL)
	}
}

func TestWebhookStoreDelete(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewWebhookStore(rdb)
	ctx := context.Background()

	w, _ := s.Create(ctx, "tenant-a", "https://example.com", "", nil)

	if err := s.Delete(ctx, w.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := s.GetByID(ctx, w.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestWebhookStoreMatch(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewWebhookStore(rdb)
	ctx := context.Background()

	_, _ = s.Create(ctx, "tenant-a", "https://a.com/c1", "", []string{"completed"})
	_, _ = s.Create(ctx, "tenant-a", "https://a.com/c2", "", []string{"completed", "failed"})
	_, _ = s.Create(ctx, "tenant-a", "https://a.com/c3", "", []string{"*"})
	_, _ = s.Create(ctx, "tenant-b", "https://b.com/hook", "", []string{"completed"})

	matched, err := s.Match(ctx, "tenant-a", "completed")
	if err != nil {
		t.Fatalf("Match failed: %v", err)
	}
	if len(matched) != 3 {
		t.Fatalf("expected 3 matched webhooks for tenant-a/completed, got %d", len(matched))
	}

	matchedFailed, err := s.Match(ctx, "tenant-a", "failed")
	if err != nil {
		t.Fatalf("Match failed: %v", err)
	}
	if len(matchedFailed) != 2 {
		t.Fatalf("expected 2 matched webhooks for tenant-a/failed, got %d", len(matchedFailed))
	}

	matchedB, err := s.Match(ctx, "tenant-b", "completed")
	if err != nil {
		t.Fatalf("Match failed: %v", err)
	}
	if len(matchedB) != 1 {
		t.Fatalf("expected 1 matched webhook for tenant-b, got %d", len(matchedB))
	}
}

func TestWebhookStoreGetByIDNotFound(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewWebhookStore(rdb)
	ctx := context.Background()

	_, err := s.GetByID(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent webhook")
	}
}

func TestWebhookStoreUpdateNotFound(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewWebhookStore(rdb)
	ctx := context.Background()

	_, err := s.Update(ctx, "nonexistent", "https://example.com", "", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent webhook")
	}
}
