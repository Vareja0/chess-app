package controllers

import (
	"context"
	"encoding/json"
	"log"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vareja0/go-jwt/utils"
)

// wsBaseURL derives the WebSocket base URL from APP_BASE_URL, swapping http(s) scheme for ws(s).
func wsBaseURL() string {
	base := strings.TrimRight(os.Getenv("APP_BASE_URL"), "/")
	base = strings.Replace(base, "https://", "wss://", 1)
	base = strings.Replace(base, "http://", "ws://", 1)
	return base
}

type MatchmakingResult struct {
	Room     string `json:"room"`
	URL      string `json:"url"`
	TimeMode string `json:"time_mode"`
}

var timeControls = map[string]time.Duration{
	"bullet":    1 * time.Minute,
	"blitz":     3 * time.Minute,
	"rapid":     5 * time.Minute,
	"classical": 10 * time.Minute,
}

// HandleCancelMatchmaking removes the authenticated player from the matchmaking queue and sets their status back to "idle".
func HandleCancelMatchmaking(c *gin.Context) {
	ctx := context.Background()
	user := utils.GetUserId(c)

	state, err := GetPlayerState(ctx, user.ID)
	if err == nil && state.TimeMode != "" {
		if err := RemoveFromQueue(ctx, user.ID, state.TimeMode); err != nil {
			log.Print(err)
		}
	}

	UpdatePlayerStatus(ctx, user.ID, "idle")

	c.JSON(200, gin.H{"message": "cancelled"})
}

// HandleMatchmaking pairs the authenticated player with an opponent in the same time-control queue.
// If already in a game, returns the existing room. If the queue is empty, enqueues and long-polls (30 s timeout).
// If an opponent is available, opens a Room with random colours and notifies both players via Pub/Sub.
func HandleMatchmaking(c *gin.Context) {
	ctx := context.Background()
	user := utils.GetUserId(c)

	var body struct {
		TimeMode string `json:"time_mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.TimeMode == "" {
		c.JSON(400, gin.H{"error": "time_mode required"})
		return
	}

	duration, ok := timeControls[body.TimeMode]
	if !ok {
		c.JSON(400, gin.H{"error": "invalid time_mode"})
		return
	}

	playerStatus, err := GetPlayerStatus(ctx, user.ID)
	if err != nil {
		c.JSON(401, gin.H{"error": "erro pegando player status"})
		return
	}

	if playerStatus == "in_game" {
		room, err := GetPlayerRoom(ctx, user.ID)
		if err != nil {
			c.JSON(401, gin.H{"error": "erro pegando player room"})
			return
		}
		c.JSON(200, gin.H{
			"room": room,
			"url":  wsBaseURL() + "/ws/" + room,
		})
		return
	}

	opponentID, err := DequeuePlayer(ctx, body.TimeMode)

	if err == redis.Nil || opponentID == 0 {
		currentState, _ := GetPlayerState(ctx, user.ID)
		roomID := ""
		if currentState != nil {
			roomID = currentState.RoomID
		}
		if err := SetPlayerState(ctx, user.ID, PlayerState{
			Status:   "in_queue",
			RoomID:   roomID,
			TimeMode: body.TimeMode,
		}); err != nil {
			log.Print("erro ao update player state: ", err)
		}

		EnqueuePlayer(ctx, user.ID, body.TimeMode)

		sub := SubscribeMatch(ctx, user.ID)
		defer sub.Close()

		ch := sub.Channel()

		select {
		case msg := <-ch:
			var result MatchmakingResult
			json.Unmarshal([]byte(msg.Payload), &result)

			UpdatePlayerRoom(ctx, user.ID, "in_game", result.Room)

			c.JSON(200, result)

		case <-time.After(30 * time.Second):
			RemoveFromQueue(ctx, user.ID, body.TimeMode)
			UpdatePlayerStatus(ctx, user.ID, "idle")
			c.JSON(408, gin.H{"error": "timeout"})
		}

	} else if err != nil {
		c.JSON(500, gin.H{"error": "redis error"})
		return
	} else {
		white, black := user.ID, opponentID
		if rand.IntN(2) == 0 {
			white, black = black, white
		}
		id := GameRooms.Create(white, black, duration)

		result := MatchmakingResult{
			Room:     id,
			URL:      wsBaseURL() + "/ws/" + id,
			TimeMode: body.TimeMode,
		}

		PublishMatch(ctx, opponentID, result)

		UpdatePlayerRoom(ctx, user.ID, "in_game", id)

		c.JSON(200, result)
	}
}
