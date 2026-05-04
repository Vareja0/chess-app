package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/notnil/chess"
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

type Game struct {
	ID          string
	Players     [2]*Player
	chess       *chess.Game
	mutex       sync.Mutex
	TimeLeft    [2]time.Duration
	TimeControl time.Duration
	LastTick    time.Time
	TimerStop   chan struct{}
	FirstMove   bool
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

// runTimer ticks every second, decrements the active player's clock, broadcasts time to both clients,
// and ends the game with a timeout outcome when a player's time reaches zero.
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
// If an opponent is available, creates the game, assigns random colors, and notifies both players via Pub/Sub.
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
		colors := []chess.Color{chess.White, chess.Black}
		rand.Shuffle(len(colors), func(i, j int) {
			colors[i], colors[j] = colors[j], colors[i]
		})

		id := uuid.New().String()[:8]
		gamesMu.Lock()
		games[id] = &Game{
			ID:          id,
			chess:       chess.NewGame(),
			TimeLeft:    [2]time.Duration{duration, duration},
			TimeControl: duration,
			LastTick:    time.Now(),
			TimerStop:   make(chan struct{}),
			Players: [2]*Player{
				{ID: user.ID, Color: colors[0]},
				{ID: opponentID, Color: colors[1]},
			},
			FirstMove: true,
		}
		gamesMu.Unlock()

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

// HandleWebSocket upgrades the connection, authenticates the player against the room, and processes
// "move", "start", and "resign" messages until the connection closes or the game ends.
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

// sendAll broadcasts a JSON message to every connected player in the game; caller must hold g.mutex.
func (g *Game) sendAll(msg interface{}) {
	data, _ := json.Marshal(msg)
	for _, p := range g.Players {
		if p != nil && p.Conn != nil {
			p.Conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

// sendExcept sends a JSON message to all players except the specified connection; caller must hold g.mutex.
func (g *Game) sendExcept(except *websocket.Conn, msg interface{}) {
	data, _ := json.Marshal(msg)
	for _, p := range g.Players {
		if p != nil && p.Conn != nil && p.Conn != except {
			p.Conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

// broadcastExcept acquires the mutex and sends a message to all players except the given connection.
func (g *Game) broadcastExcept(except *websocket.Conn, msg interface{}) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.sendExcept(except, msg)
}

// removePlayer sets the matching player slot to nil and notifies the remaining player of the disconnection.
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

// CreateGame allocates a new in-memory game with a random 8-char ID and returns the room URL.
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

// cleanup stops the timer, persists the game result to Redis, closes both WebSocket connections,
// removes the game from the in-memory map, and resets both players' Redis state to "idle".
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
