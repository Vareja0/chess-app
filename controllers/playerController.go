package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/vareja0/go-jwt/initializers"
)

type PlayerState struct {
	Status string `json:"status"`
	RoomID string `json:"room_id"`
}

func userKey(userID uint) string {
	return fmt.Sprintf("player:%d", userID)
}

func SetPlayerState(ctx context.Context, userID uint, state PlayerState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return initializers.RDB.Set(ctx, userKey(userID), data, 0).Err()
}

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

func GetPlayerState(ctx context.Context, userID uint) (*PlayerState, error) {
	return getPlayerStateRaw(ctx, userID)
}

func GetPlayerStatus(ctx context.Context, userID uint) (string, error) {
	state, err := getPlayerStateRaw(ctx, userID)
	if err != nil {
		return "", err
	}
	return state.Status, nil
}

func GetPlayerRoom(ctx context.Context, userID uint) (string, error) {
	state, err := getPlayerStateRaw(ctx, userID)
	if err != nil {
		return "", err
	}
	return state.RoomID, nil
}

func UpdatePlayerStatus(ctx context.Context, userID uint, status string) error {
	state, err := getPlayerStateRaw(ctx, userID)
	if err != nil {
		state = &PlayerState{RoomID: ""}
	}
	state.Status = status
	return SetPlayerState(ctx, userID, *state)
}

func UpdatePlayerRoom(ctx context.Context, userID uint, status string, roomID string) error {
	return SetPlayerState(ctx, userID, PlayerState{Status: status, RoomID: roomID})
}

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

func DeletePlayerState(ctx context.Context, userID uint) error {
	return initializers.RDB.Del(ctx, userKey(userID)).Err()
}
