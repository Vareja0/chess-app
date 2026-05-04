package tests

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/vareja0/go-jwt/controllers"
)

func TestEnqueueDequeue_PerMode(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	if err := controllers.EnqueuePlayer(ctx, 1, "blitz"); err != nil {
		t.Fatalf("EnqueuePlayer: %v", err)
	}

	got, err := controllers.DequeuePlayer(ctx, "blitz")
	if err != nil {
		t.Fatalf("DequeuePlayer blitz: %v", err)
	}
	if got != 1 {
		t.Errorf("got %d, want 1", got)
	}

	_, err = controllers.DequeuePlayer(ctx, "rapid")
	if err != redis.Nil {
		t.Errorf("expected redis.Nil for empty queue, got %v", err)
	}
}

func TestRemoveFromQueue_PerMode(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	controllers.EnqueuePlayer(ctx, 2, "bullet")

	if err := controllers.RemoveFromQueue(ctx, 2, "bullet"); err != nil {
		t.Fatalf("RemoveFromQueue: %v", err)
	}

	_, err := controllers.DequeuePlayer(ctx, "bullet")
	if err != redis.Nil {
		t.Errorf("expected redis.Nil after remove, got %v", err)
	}
}

func TestEnqueue_IsolatedPerMode(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	controllers.EnqueuePlayer(ctx, 3, "classical")
	controllers.EnqueuePlayer(ctx, 4, "blitz")

	got, err := controllers.DequeuePlayer(ctx, "classical")
	if err != nil {
		t.Fatalf("DequeuePlayer classical: %v", err)
	}
	if got != 3 {
		t.Errorf("got %d, want 3", got)
	}

	got, err = controllers.DequeuePlayer(ctx, "blitz")
	if err != nil {
		t.Fatalf("DequeuePlayer blitz: %v", err)
	}
	if got != 4 {
		t.Errorf("got %d, want 4", got)
	}
}
