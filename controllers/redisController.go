package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
	"github.com/vareja0/go-jwt/initializers"
)

// queueKey returns the Redis key for a time-mode-specific matchmaking queue.
func queueKey(timeMode string) string { return "matchmaking:queue:" + timeMode }

// EnqueuePlayer appends a player to the tail of the mode-specific matchmaking queue.
func EnqueuePlayer(ctx context.Context, playerID uint, timeMode string) error {
	return initializers.RDB.RPush(ctx, queueKey(timeMode), playerID).Err()
}

// DequeuePlayer pops a player from the head of the mode-specific matchmaking queue; returns redis.Nil when queue is empty.
func DequeuePlayer(ctx context.Context, timeMode string) (uint, error) {
	result, err := initializers.RDB.LPop(ctx, queueKey(timeMode)).Result()
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseUint(result, 10, 32)
	return uint(id), err
}

// RemoveFromQueue removes the first occurrence of a player from the mode-specific matchmaking queue (used on cancel).
func RemoveFromQueue(ctx context.Context, playerID uint, timeMode string) error {
	return initializers.RDB.LRem(ctx, queueKey(timeMode), 1, playerID).Err()
}

// PublishMatch sends the match result to the player's personal Pub/Sub channel so their long-poll returns.
func PublishMatch(ctx context.Context, playerID uint, result MatchmakingResult) error {
	data, _ := json.Marshal(result)
	return initializers.RDB.Publish(ctx, fmt.Sprintf("match:%d", playerID), data).Err()
}

// SubscribeMatch opens a Redis Pub/Sub subscription on the player's personal match channel.
func SubscribeMatch(ctx context.Context, playerID uint) *redis.PubSub {
	return initializers.RDB.Subscribe(ctx, fmt.Sprintf("match:%d", playerID))
}

// SaveGameResult stores the Result of a finished Game as JSON for history.
func SaveGameResult(ctx context.Context, result GameResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return initializers.RDB.Set(ctx, "game_result:"+result.ID, data, 0).Err()
}
