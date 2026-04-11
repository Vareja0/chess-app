package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/notnil/chess"
	"github.com/redis/go-redis/v9"
	"github.com/vareja0/go-jwt/utils"
)

type Message struct {
	Type      string `json:"type"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Promotion string `json:"promotion,omitempty"`
}

type Player struct {
	ID    uint
	Conn  *websocket.Conn
	Color chess.Color
}

type MatchmakingEntry struct {
	PlayerID uint
	ResChan  chan MatchmakingResult
}

type MatchmakingResult struct {
	Room string `json:"room"`
	URL  string `json:"url"`
}

type Game struct {
	ID        string
	Players   [2]*Player
	chess     *chess.Game
	mutex     sync.Mutex
	TimeLeft  [2]time.Duration
	LastTick  time.Time
	TimerStop chan struct{}
	FirstMove bool
}

type GameResult struct {
	ID       string        `json:"id"`
	Players  [2]PlayerInfo `json:"players"`
	FEN      string        `json:"fen"`
	Outcome  string        `json:"outcome"`
	Method   string        `json:"method"`
	TimeLeft [2]int        `json:"time_left"`
}

type PlayerInfo struct {
	ID    uint   `json:"id"`
	Color string `json:"color"`
}

var games = make(map[string]*Game)
var gamesMu sync.RWMutex

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (g *Game) runTimer() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.TimerStop:
			return
		case <-ticker.C:
			g.mutex.Lock()
			turn := g.chess.Position().Turn() 
			idx := 0
			if turn == chess.Black {
				idx = 1
			}

			g.TimeLeft[idx] -= time.Second

			g.sendAll(map[string]interface{}{
				"type":       "timer",
				"white_time": int(g.TimeLeft[0].Seconds()),
				"black_time": int(g.TimeLeft[1].Seconds()),
			})

			if g.TimeLeft[idx] <= 0 {
				winner := "black"
				if idx == 1 {
					winner = "white"
				}
				g.sendAll(map[string]interface{}{
					"type":    "game_over",
					"outcome": "timeout",
					"winner":  winner,
				})
				g.removePlayer(g.Players[0].Conn)
				g.removePlayer(g.Players[1].Conn)
				g.mutex.Unlock()
				return
			}

			g.mutex.Unlock()
		}
	}
}

func HandleCancelMatchmaking(c *gin.Context) {
	ctx := context.Background()
	user := utils.GetUserId(c)

	err := RemoveFromQueue(ctx, user.ID)

	if err != nil {
		log.Print(err)
	}

	UpdatePlayerStatus(ctx, user.ID, "idle")

	c.JSON(200, gin.H{"message": "cancelled"})
}

func HandleMatchmaking(c *gin.Context) {
	ctx := context.Background()
	user := utils.GetUserId(c)

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
			"url":  "ws://localhost:3000/ws/" + room,
		})
		return
	}

	opponentID, err := DequeuePlayer(ctx)

	if err == redis.Nil || opponentID == 0 {
		EnqueuePlayer(ctx, user.ID)

		err := UpdatePlayerStatus(ctx, user.ID, "in_queue")

		if err != nil {
			log.Print("erro ao update player status")
		}

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
			RemoveFromQueue(ctx, user.ID)
			UpdatePlayerStatus(ctx, user.ID, "idle")
			c.JSON(408, gin.H{"error": "timeout"})
		}

	} else if err != nil {
		c.JSON(500, gin.H{"error": "redis error"})
		return
	} else {
		colors := []chess.Color{chess.White, chess.Black}
		rand.Shuffle(len(colors), func(i, j int) {
			colors[i], colors[j] = colors[j], colors[i]
		})

		id := uuid.New().String()[:8]
		gamesMu.Lock()
		games[id] = &Game{
			ID:        id,
			chess:     chess.NewGame(),
			TimeLeft:  [2]time.Duration{5 * time.Minute, 5 * time.Minute},
			LastTick:  time.Now(),
			TimerStop: make(chan struct{}),
			Players: [2]*Player{
				{ID: user.ID, Color: colors[0]},
				{ID: opponentID, Color: colors[1]},
			},
			FirstMove: true,
		}
		gamesMu.Unlock()

		result := MatchmakingResult{
			Room: id,
			URL:  "ws://localhost:3000/ws/" + id,
		}

		PublishMatch(ctx, opponentID, result)

		UpdatePlayerRoom(ctx, user.ID, "in_game", id)

		c.JSON(200, result)
	}
}

func HandleWebSocket(c *gin.Context) {
	roomID := c.Param("room")
	ctx := context.Background()

	gamesMu.RLock()
	game, exists := games[roomID]
	gamesMu.RUnlock()
	if !exists {
		c.String(404, "Sala não encontrada")
		return
	}

	user := utils.GetUserId(c)

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer ws.Close()

	game.mutex.Lock()

	var player *Player
	for _, p := range game.Players {
		if p.ID == user.ID {
			player = p
			break
		}
	}

	if player == nil {

		game.mutex.Unlock()
		ws.WriteJSON(map[string]string{"type": "error", "message": "Não autorizado"})
		ws.Close()
		return
	}

	player.Conn = ws

	fen := game.chess.FEN()

	game.mutex.Unlock()

	ws.WriteJSON(map[string]interface{}{
		"type":    "joined",
		"color":   player.Color.String(),
		"fen":     fen,
		"message": "Conectado! Aguarde o adversário...",
	})

	game.broadcastExcept(ws, map[string]interface{}{
		"type":    "opponent_joined",
		"message": "Adversário conectado! Você pode iniciar.",
	})

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			break
		}

		log.Printf("Received message: %s", message)

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			ws.WriteJSON(map[string]string{"type": "error", "message": "Formato inválido"})
			continue
		}

		game.mutex.Lock()
		switch msg.Type {
		case "move":
			if game.FirstMove == true {
				go game.runTimer()
				game.FirstMove = false
			}
			if game.chess.Position().Turn() != player.Color {
				ws.WriteJSON(map[string]string{"type": "error", "message": "Não é sua vez"})
				break
			}

			move, err := chess.UCINotation{}.Decode(game.chess.Position(), msg.From+msg.To+msg.Promotion)
			if err != nil {
				ws.WriteJSON(map[string]string{"type": "error", "message": "Movimento inválido: " + err.Error()})
				break
			}

			if err := game.chess.Move(move); err != nil {
				ws.WriteJSON(map[string]string{"type": "error", "message": "Movimento ilegal: " + err.Error()})
				break
			}

			game.sendAll(map[string]interface{}{
				"type":      "move",
				"from":      msg.From,
				"to":        msg.To,
				"promotion": msg.Promotion,
				"fen":       game.chess.FEN(),
				"turn":      game.chess.Position().Turn().String(),
			})

			if game.chess.Outcome() != chess.NoOutcome {
				game.sendAll(map[string]interface{}{
					"type":    "game_over",
					"outcome": game.chess.Outcome().String(),
					"method":  game.chess.Method().String(),
				})
				cleanup(ctx, game, game.chess.Outcome().String(), game.chess.Method().String())
			}
			game.LastTick = time.Now()

		case "start":
			if game.Players[0] != nil && game.Players[1] != nil {
				game.sendAll(map[string]interface{}{
					"type": "start",
					"fen":  game.chess.FEN(),
				})
			}

		case "resign":
			game.sendAll(map[string]interface{}{
				"type":    "game_over",
				"outcome": "resign",
				"winner":  (chess.Black + player.Color).String(),
			})
			cleanup(ctx, game, game.chess.Outcome().String(), game.chess.Method().String())

		default:
			ws.WriteJSON(map[string]string{"type": "error", "message": "Tipo desconhecido"})
		}
		game.mutex.Unlock()

	}
	game.mutex.Lock()
	if game.chess.Outcome() == chess.NoOutcome {
		winner := "black"
		if player.Color == chess.White {
			winner = "black"
		} else {
			winner = "white"
		}
		game.sendAll(map[string]interface{}{
			"type":    "game_over",
			"outcome": "disconnect",
			"winner":  winner,
		})
		game.mutex.Unlock()
		cleanup(ctx, game, "disconnect", winner)
	} else {
		game.mutex.Unlock()
	}

}

func (g *Game) sendAll(msg interface{}) {
	data, _ := json.Marshal(msg)
	for _, p := range g.Players {
		if p != nil && p.Conn != nil {
			p.Conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

func (g *Game) sendExcept(except *websocket.Conn, msg interface{}) {
	data, _ := json.Marshal(msg)
	for _, p := range g.Players {
		if p != nil && p.Conn != nil && p.Conn != except {
			p.Conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

func (g *Game) broadcastExcept(except *websocket.Conn, msg interface{}) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.sendExcept(except, msg)
}

func (g *Game) removePlayer(conn *websocket.Conn) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	for i, p := range g.Players {
		if p != nil && p.Conn == conn {
			g.Players[i] = nil
			break
		}
	}
	g.sendAll(map[string]string{"type": "opponent_disconnected", "message": "Adversário saiu"})
}

func CreateGame(c *gin.Context) {
	id := uuid.New().String()[:8]
	gamesMu.Lock()
	games[id] = &Game{
		ID:    id,
		chess: chess.NewGame(),
	}
	gamesMu.Unlock()

	c.JSON(200, gin.H{"room": id, "url": fmt.Sprintf("http://localhost:3000/?room=%s", id)})
}

func cleanup(ctx context.Context, game *Game, outcome string, method string) {
	close(game.TimerStop)

	playerInfos := [2]PlayerInfo{
		{ID: game.Players[0].ID, Color: game.Players[0].Color.String()},
		{ID: game.Players[1].ID, Color: game.Players[1].Color.String()},
	}

	gameResult := GameResult{
		ID:       game.ID,
		Players:  playerInfos,
		FEN:      game.chess.FEN(),
		Outcome:  outcome,
		Method:   method,
		TimeLeft: [2]int{int(game.TimeLeft[0].Seconds()), int(game.TimeLeft[1].Seconds())},
	}
	SaveGameResult(ctx, gameResult)

	for _, p := range game.Players {
		if p != nil && p.Conn != nil {
			p.Conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "game over"))
			p.Conn.Close()
		}
	}

	gamesMu.Lock()
	delete(games, game.ID)
	gamesMu.Unlock()

	for _, p := range game.Players {
		if p != nil {
			UpdatePlayerRoom(ctx, p.ID, "idle", "")
		}
	}
}
