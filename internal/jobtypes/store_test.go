package jobtypes

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestStore_CreateAndList(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewStore(rdb)
	ctx := context.Background()

	jt, err := store.Create(ctx, "slack-notify", "Slack webhook", "http", `{"url":"..."}`)
	if err != nil {
		t.Fatal(err)
	}
	if jt.Name != "slack-notify" || jt.Handler != "http" {
		t.Fatalf("unexpected job type: %+v", jt)
	}

	if !store.IsAllowed(ctx, "slack-notify") {
		t.Fatal("custom type should be allowed")
	}
	if store.HandlerFor(ctx, "slack-notify") != "http" {
		t.Fatal("expected http handler")
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 4 { // 3 built-in + 1 custom
		t.Fatalf("expected at least 4 types, got %d", len(list))
	}

	if err := store.Delete(ctx, "slack-notify"); err != nil {
		t.Fatal(err)
	}
	if store.IsAllowed(ctx, "slack-notify") {
		t.Fatal("deleted type should not be allowed")
	}
}

func TestStore_CannotDeleteBuiltIn(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	store := NewStore(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	if err := store.Delete(context.Background(), "email"); err == nil {
		t.Fatal("expected error deleting built-in type")
	}
}
