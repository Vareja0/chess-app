package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/vareja0/go-jwt/initializers"
)

type PlayerState struct {
	Status   string `json:"status"`
	RoomID   string `json:"room_id"`
	TimeMode string `json:"time_mode"`
}

// userKey returns the Redis key used to store a player's state.
func userKey(userID uint) string {
	return fmt.Sprintf("player:%d", userID)
}

// SetPlayerState serialises and writes the full PlayerState to Redis with no expiry.
func SetPlayerState(ctx context.Context, userID uint, state PlayerState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return initializers.RDB.Set(ctx, userKey(userID), data, 0).Err()
}

// getPlayerStateRaw fetches and deserialises a player's state from Redis; shared by all read helpers.
func getPlayerStateRaw(ctx context.Context, userID uint) (*PlayerState, error) {
	val, err := initializers.RDB.Get(ctx, userKey(userID)).Bytes()
	if err != nil {
		return nil, err
	}
	var state PlayerState
	if err := json.Unmarshal(val, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// GetPlayerState returns the full PlayerState for a user.
func GetPlayerState(ctx context.Context, userID uint) (*PlayerState, error) {
	return getPlayerStateRaw(ctx, userID)
}

// GetPlayerStatus returns only the status field (e.g. "idle", "in_queue", "in_game").
func GetPlayerStatus(ctx context.Context, userID uint) (string, error) {
	state, err := getPlayerStateRaw(ctx, userID)
	if err != nil {
		return "", err
	}
	return state.Status, nil
}

// GetPlayerRoom returns only the room_id field for the player's current game.
func GetPlayerRoom(ctx context.Context, userID uint) (string, error) {
	state, err := getPlayerStateRaw(ctx, userID)
	if err != nil {
		return "", err
	}
	return state.RoomID, nil
}

// UpdatePlayerStatus changes the status field while preserving the existing room_id.
func UpdatePlayerStatus(ctx context.Context, userID uint, status string) error {
	state, err := getPlayerStateRaw(ctx, userID)
	if err != nil {
		state = &PlayerState{RoomID: ""}
	}
	state.Status = status
	return SetPlayerState(ctx, userID, *state)
}

// UpdatePlayerRoom overwrites both status and room_id atomically.
func UpdatePlayerRoom(ctx context.Context, userID uint, status string, roomID string) error {
	return SetPlayerState(ctx, userID, PlayerState{Status: status, RoomID: roomID})
}

// AddIfNotExists initialises a player's Redis state as "idle" only if no entry exists yet.
func AddIfNotExists(ctx context.Context, userID uint) error {
	key := userKey(userID)
	exists, _ := initializers.RDB.Exists(ctx, key).Result()
	log.Printf("AddIfNotExists: key=%s exists=%d", key, exists)
	if exists == 0 {
		err := SetPlayerState(ctx, userID, PlayerState{Status: "idle", RoomID: ""})
		log.Printf("AddIfNotExists: set result err=%v", err)
		return err
	}
	return nil
}

// DeletePlayerState removes all Redis state for a player.
func DeletePlayerState(ctx context.Context, userID uint) error {
	return initializers.RDB.Del(ctx, userKey(userID)).Err()
}
