package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
	"github.com/vareja0/go-jwt/initializers"
)

func EnqueuePlayer(ctx context.Context, playerID uint) error {
	return initializers.RDB.RPush(ctx, "matchmaking:queue", playerID).Err()
}

func DequeuePlayer(ctx context.Context) (uint, error) {
	result, err := initializers.RDB.LPop(ctx, "matchmaking:queue").Result()
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseUint(result, 10, 32)
	return uint(id), err
}

func RemoveFromQueue(ctx context.Context, playerID uint) error {
	return initializers.RDB.LRem(ctx, "matchmaking:queue", 1, playerID).Err()
}

func PublishMatch(ctx context.Context, playerID uint, result MatchmakingResult) error {
	data, _ := json.Marshal(result)
	return initializers.RDB.Publish(ctx, fmt.Sprintf("match:%d", playerID), data).Err()
}

func SubscribeMatch(ctx context.Context, playerID uint) *redis.PubSub {
	return initializers.RDB.Subscribe(ctx, fmt.Sprintf("match:%d", playerID))
}

func gameKey(id string) string {
	return fmt.Sprintf("game:%s", id)
}

func UpdateGameFEN(ctx context.Context, id string, fen string) error {
	return initializers.RDB.JSONSet(ctx, gameKey(id), "$.fen", fmt.Sprintf(`"%s"`, fen)).Err()
}

func UpdateGameTime(ctx context.Context, id string, whiteTime, blackTime int) error {
	pipe := initializers.RDB.Pipeline()
	pipe.JSONSet(ctx, gameKey(id), "$.time_left[0]", whiteTime)
	pipe.JSONSet(ctx, gameKey(id), "$.time_left[1]", blackTime)
	_, err := pipe.Exec(ctx)
	return err
}

func UpdateFirstMove(ctx context.Context, id string) error {
	return initializers.RDB.JSONSet(ctx, gameKey(id), "$.first_move", false).Err()
}

func DeleteGame(ctx context.Context, id string) error {
	return initializers.RDB.Del(ctx, gameKey(id)).Err()
}

func SaveGameResult(ctx context.Context, result GameResult) error {
	return initializers.RDB.JSONSet(ctx, fmt.Sprintf("game_result:%s", result.ID), "$", result).Err()
}
