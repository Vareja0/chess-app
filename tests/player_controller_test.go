package tests

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/vareja0/go-jwt/controllers"
	"github.com/vareja0/go-jwt/initializers"
)

func setupRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)
	initializers.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr
}

func TestSetAndGetPlayerState(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	want := controllers.PlayerState{Status: "idle", RoomID: ""}
	if err := controllers.SetPlayerState(ctx, 1, want); err != nil {
		t.Fatalf("SetPlayerState: %v", err)
	}

	got, err := controllers.GetPlayerState(ctx, 1)
	if err != nil {
		t.Fatalf("GetPlayerState: %v", err)
	}
	if got.Status != want.Status || got.RoomID != want.RoomID {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetPlayerStatus(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	controllers.SetPlayerState(ctx, 2, controllers.PlayerState{Status: "in_queue", RoomID: ""})

	status, err := controllers.GetPlayerStatus(ctx, 2)
	if err != nil {
		t.Fatalf("GetPlayerStatus: %v", err)
	}
	if status != "in_queue" {
		t.Errorf("got %q, want %q", status, "in_queue")
	}
}

func TestGetPlayerRoom(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	controllers.SetPlayerState(ctx, 3, controllers.PlayerState{Status: "in_game", RoomID: "room-abc"})

	room, err := controllers.GetPlayerRoom(ctx, 3)
	if err != nil {
		t.Fatalf("GetPlayerRoom: %v", err)
	}
	if room != "room-abc" {
		t.Errorf("got %q, want %q", room, "room-abc")
	}
}

func TestUpdatePlayerStatus_PreservesRoom(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	controllers.SetPlayerState(ctx, 4, controllers.PlayerState{Status: "idle", RoomID: "room-xyz"})

	if err := controllers.UpdatePlayerStatus(ctx, 4, "in_queue"); err != nil {
		t.Fatalf("UpdatePlayerStatus: %v", err)
	}

	state, _ := controllers.GetPlayerState(ctx, 4)
	if state.Status != "in_queue" {
		t.Errorf("status: got %q, want %q", state.Status, "in_queue")
	}
	if state.RoomID != "room-xyz" {
		t.Errorf("room should be preserved: got %q, want %q", state.RoomID, "room-xyz")
	}
}

func TestUpdatePlayerStatus_CreatesIfMissing(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	if err := controllers.UpdatePlayerStatus(ctx, 99, "idle"); err != nil {
		t.Fatalf("UpdatePlayerStatus on missing key: %v", err)
	}

	status, err := controllers.GetPlayerStatus(ctx, 99)
	if err != nil {
		t.Fatalf("GetPlayerStatus: %v", err)
	}
	if status != "idle" {
		t.Errorf("got %q, want %q", status, "idle")
	}
}

func TestUpdatePlayerRoom(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	if err := controllers.UpdatePlayerRoom(ctx, 5, "in_game", "room-42"); err != nil {
		t.Fatalf("UpdatePlayerRoom: %v", err)
	}

	state, _ := controllers.GetPlayerState(ctx, 5)
	if state.Status != "in_game" || state.RoomID != "room-42" {
		t.Errorf("got %+v, want status=in_game room=room-42", state)
	}
}

func TestAddIfNotExists_SetsIdleForNewPlayer(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	if err := controllers.AddIfNotExists(ctx, 10); err != nil {
		t.Fatalf("AddIfNotExists: %v", err)
	}

	state, err := controllers.GetPlayerState(ctx, 10)
	if err != nil {
		t.Fatalf("GetPlayerState: %v", err)
	}
	if state.Status != "idle" {
		t.Errorf("got %q, want %q", state.Status, "idle")
	}
}

func TestAddIfNotExists_DoesNotOverwriteExisting(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	controllers.SetPlayerState(ctx, 11, controllers.PlayerState{Status: "in_game", RoomID: "room-1"})

	if err := controllers.AddIfNotExists(ctx, 11); err != nil {
		t.Fatalf("AddIfNotExists: %v", err)
	}

	state, _ := controllers.GetPlayerState(ctx, 11)
	if state.Status != "in_game" {
		t.Errorf("existing state was overwritten: got %q, want %q", state.Status, "in_game")
	}
}

func TestDeletePlayerState(t *testing.T) {
	setupRedis(t)
	ctx := context.Background()

	controllers.SetPlayerState(ctx, 12, controllers.PlayerState{Status: "idle", RoomID: ""})

	if err := controllers.DeletePlayerState(ctx, 12); err != nil {
		t.Fatalf("DeletePlayerState: %v", err)
	}

	_, err := controllers.GetPlayerState(ctx, 12)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}
