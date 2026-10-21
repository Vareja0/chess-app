package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/vareja0/go-jwt/game"
	"github.com/vareja0/go-jwt/models"
)

// Rooms is the registry of live Rooms. Each Room runs one goroutine that owns its Game,
// so nothing outside that goroutine touches the Game or writes to its sockets.
type Rooms struct {
	now       func() time.Time
	tickEvery time.Duration

	mu    sync.Mutex
	rooms map[string]*room
}

// NewRooms returns an empty registry. now is the Clocks' time source; tickEvery is how often each Room checks its Clocks.
func NewRooms(now func() time.Time, tickEvery time.Duration) *Rooms {
	return &Rooms{now: now, tickEvery: tickEvery, rooms: make(map[string]*room)}
}

// GameRooms is the registry used by the HTTP handlers.
var GameRooms = NewRooms(time.Now, time.Second)

// Create opens a Room for a new Game between white and black, starts its loop, and returns the Room ID.
func (rs *Rooms) Create(white, black uint, timeControl time.Duration) string {
	r := &room{
		id:       uuid.New().String()[:8],
		players:  map[game.Color]uint{game.White: white, game.Black: black},
		conns:    make(map[game.Color]*websocket.Conn),
		game:     game.New(timeControl, rs.now()),
		commands: make(chan command),
		done:     make(chan struct{}),
	}
	rs.mu.Lock()
	rs.rooms[r.id] = r
	rs.mu.Unlock()

	go r.run(rs)
	return r.id
}

// Exists reports whether the Room is still live.
func (rs *Rooms) Exists(id string) bool {
	return rs.get(id) != nil
}

func (rs *Rooms) get(id string) *room {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.rooms[id]
}

func (rs *Rooms) remove(id string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	delete(rs.rooms, id)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// HandleWebSocket connects the authenticated player (set by RequireAuth) to their seat in the Room,
// then forwards every message they send to the Room until the socket closes.
func (rs *Rooms) HandleWebSocket(c *gin.Context) {
	r := rs.get(c.Param("room"))
	if r == nil {
		c.String(http.StatusNotFound, "Sala não encontrada")
		return
	}
	user, ok := c.MustGet("user").(models.User)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	color := r.colorOf(user.ID)
	if color == game.NoColor {
		c.String(http.StatusForbidden, "Não autorizado")
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	if !r.send(command{kind: cmdJoin, color: color, conn: conn}) {
		return
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			r.send(command{kind: cmdLeave, color: color, conn: conn})
			return
		}
		if !r.send(command{kind: cmdMessage, color: color, conn: conn, data: data}) {
			return
		}
	}
}

type commandKind int

const (
	cmdJoin commandKind = iota
	cmdMessage
	cmdLeave
)

type command struct {
	kind  commandKind
	color game.Color
	conn  *websocket.Conn
	data  []byte
}

// room is the live home of one Game. Only its run goroutine reads or writes its fields after Create.
type room struct {
	id       string
	players  map[game.Color]uint // fixed at Create; safe to read from any goroutine
	conns    map[game.Color]*websocket.Conn
	game     *game.Game
	commands chan command
	done     chan struct{}
}

func (r *room) colorOf(userID uint) game.Color {
	for color, id := range r.players {
		if id == userID {
			return color
		}
	}
	return game.NoColor
}

// send hands cmd to the Room's loop. It returns false if the Room has already closed.
func (r *room) send(cmd command) bool {
	select {
	case r.commands <- cmd:
		return true
	case <-r.done:
		return false
	}
}

// run is the Room's loop: it applies commands and Clock checks to the Game until the Game ends.
func (r *room) run(rs *Rooms) {
	ticker := time.NewTicker(rs.tickEvery)
	defer ticker.Stop()

	for {
		select {
		case cmd := <-r.commands:
			r.handle(cmd, rs.now())
		case <-ticker.C:
		}
		if result := r.game.Result(rs.now()); result != nil {
			r.finish(rs, *result)
			return
		}
	}
}

// handle applies one command to the Game. The loop checks for a Result afterwards.
func (r *room) handle(cmd command, now time.Time) {
	switch cmd.kind {
	case cmdJoin:
		if old := r.conns[cmd.color]; old != nil {
			old.Close()
		}
		r.conns[cmd.color] = cmd.conn
		state := r.game.State(now)
		cmd.conn.WriteJSON(joinedMessage{
			Type:              "joined",
			Color:             cmd.color.String(),
			FEN:               state.FEN,
			Moves:             state.Moves,
			Turn:              state.Turn.String(),
			Clocks:            clocksJSON(state.Clocks),
			OpponentConnected: r.conns[cmd.color.Opponent()] != nil,
		})
		r.sendTo(cmd.color.Opponent(), gin.H{"type": "opponent_joined", "message": "Adversário conectado!"})

	case cmdLeave:
		if r.conns[cmd.color] != cmd.conn {
			return // a socket that was already replaced
		}
		delete(r.conns, cmd.color)
		r.game.Disconnect(cmd.color, now)

	case cmdMessage:
		r.handleMessage(cmd, now)
	}
}

type clientMessage struct {
	Type      string `json:"type"`
	From      string `json:"from"`
	To        string `json:"to"`
	Promotion string `json:"promotion"`
}

func (r *room) handleMessage(cmd command, now time.Time) {
	var msg clientMessage
	if err := json.Unmarshal(cmd.data, &msg); err != nil {
		reject(cmd.conn, "bad_message", "Formato inválido")
		return
	}

	var err error
	switch msg.Type {
	case "move":
		var moved game.Moved
		moved, err = r.game.Move(cmd.color, msg.From+msg.To+msg.Promotion, now)
		if err == nil {
			r.broadcast(gin.H{
				"type":      "move",
				"from":      msg.From,
				"to":        msg.To,
				"promotion": msg.Promotion,
				"fen":       moved.FEN,
				"turn":      moved.Turn.String(),
				"clocks":    clocksJSON(moved.Clocks),
			})
		}
	case "resign":
		_, err = r.game.Resign(cmd.color, now)
	case "abort":
		_, err = r.game.Abort(cmd.color, now)
	default:
		reject(cmd.conn, "bad_message", "Tipo desconhecido")
		return
	}

	switch {
	case errors.Is(err, game.ErrNotYourTurn):
		reject(cmd.conn, "not_your_turn", "Não é sua vez")
	case errors.Is(err, game.ErrIllegalMove):
		reject(cmd.conn, "illegal_move", "Movimento ilegal")
	case errors.Is(err, game.ErrCannotAbort):
		reject(cmd.conn, "cannot_abort", "Não é possível abortar após os primeiros lances")
	}
	// game.ErrGameOver needs no reply: the Game ended during this call, and the loop announces it next.
}

// finish runs once per Room: announces the Result, closes both sockets, stores the Result,
// frees both players, and finally removes the Room from the registry.
func (r *room) finish(rs *Rooms, result game.Result) {
	state := r.game.State(rs.now())
	r.broadcast(gin.H{
		"type":   "game_over",
		"winner": colorJSON(result.Winner),
		"method": string(result.Method),
		"clocks": clocksJSON(state.Clocks),
	})

	close(r.done)

	for _, conn := range r.conns {
		conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "game over"), time.Now().Add(time.Second))
		conn.Close()
	}

	ctx := context.Background()
	if err := SaveGameResult(ctx, GameResult{
		ID: r.id,
		Players: [2]PlayerInfo{
			{ID: r.players[game.White], Color: "white"},
			{ID: r.players[game.Black], Color: "black"},
		},
		FEN:        state.FEN,
		Moves:      state.Moves,
		Winner:     colorJSON(result.Winner),
		Method:     string(result.Method),
		TimeLeftMs: [2]int64{state.Clocks.White.Milliseconds(), state.Clocks.Black.Milliseconds()},
	}); err != nil {
		log.Printf("save game result %s: %v", r.id, err)
	}
	for _, id := range r.players {
		if err := UpdatePlayerRoom(ctx, id, "idle", ""); err != nil {
			log.Printf("reset player %d: %v", id, err)
		}
	}
	rs.remove(r.id)
}

func (r *room) broadcast(msg any) {
	for _, conn := range r.conns {
		conn.WriteJSON(msg)
	}
}

func (r *room) sendTo(color game.Color, msg any) {
	if conn := r.conns[color]; conn != nil {
		conn.WriteJSON(msg)
	}
}

func reject(conn *websocket.Conn, code, message string) {
	conn.WriteJSON(gin.H{"type": "error", "code": code, "message": message})
}

type joinedMessage struct {
	Type              string     `json:"type"`
	Color             string     `json:"color"`
	FEN               string     `json:"fen"`
	Moves             []string   `json:"moves"`
	Turn              string     `json:"turn"`
	Clocks            clocksWire `json:"clocks"`
	OpponentConnected bool       `json:"opponent_connected"`
}

type clocksWire struct {
	WhiteMs int64   `json:"white_ms"`
	BlackMs int64   `json:"black_ms"`
	Running *string `json:"running"`
}

func clocksJSON(c game.Clocks) clocksWire {
	return clocksWire{WhiteMs: c.White.Milliseconds(), BlackMs: c.Black.Milliseconds(), Running: colorJSON(c.Running)}
}

// colorJSON returns nil for NoColor so it encodes as JSON null.
func colorJSON(c game.Color) *string {
	if c == game.NoColor {
		return nil
	}
	s := c.String()
	return &s
}

// GameResult is the stored record of a finished Game.
type GameResult struct {
	ID         string        `json:"id"`
	Players    [2]PlayerInfo `json:"players"`
	FEN        string        `json:"fen"`
	Moves      []string      `json:"moves"`
	Winner     *string       `json:"winner"`
	Method     string        `json:"method"`
	TimeLeftMs [2]int64      `json:"time_left_ms"`
}

type PlayerInfo struct {
	ID    uint   `json:"id"`
	Color string `json:"color"`
}
