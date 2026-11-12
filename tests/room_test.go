package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/vareja0/go-jwt/controllers"
	"github.com/vareja0/go-jwt/initializers"
	"github.com/vareja0/go-jwt/models"
	"gorm.io/gorm"
)

const (
	whiteID uint = 1
	blackID uint = 2
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type roomEnv struct {
	rooms *controllers.Rooms
	clock *fakeClock
	srv   *httptest.Server
	id    string
}

// newRoomEnv starts a Room for whiteID vs blackID (3 min) behind a real websocket server.
// The "as" query param stands in for the auth middleware.
func newRoomEnv(t *testing.T) *roomEnv {
	t.Helper()
	setupRedis(t)
	gin.SetMode(gin.TestMode)

	clock := &fakeClock{now: t0}
	rooms := controllers.NewRooms(clock.Now, 5*time.Millisecond)

	r := gin.New()
	r.GET("/ws/:room", func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Query("as"))
		c.Set("user", models.User{Model: gorm.Model{ID: uint(id)}})
	}, rooms.HandleWebSocket)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	id := rooms.Create(whiteID, blackID, 3*time.Minute)
	controllers.UpdatePlayerRoom(ctx, whiteID, "in_game", id)
	controllers.UpdatePlayerRoom(ctx, blackID, "in_game", id)

	// Runs after the test's sockets close: push time past any Clock or Abort window so the Room
	// ends, and wait for it, so no Room is still writing to the shared Redis client when the next test swaps it.
	t.Cleanup(func() {
		clock.Advance(time.Hour)
		deadline := time.Now().Add(2 * time.Second)
		for rooms.Exists(id) && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
	})

	return &roomEnv{rooms: rooms, clock: clock, srv: srv, id: id}
}

func (e *roomEnv) url(room string, as uint) string {
	return "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws/" + room + "?as=" + strconv.Itoa(int(as))
}

func (e *roomEnv) dial(t *testing.T, as uint) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(e.url(e.id, as), nil)
	if err != nil {
		t.Fatalf("dial as %d: %v", as, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// joinBoth connects White then Black and drains the join messages.
func (e *roomEnv) joinBoth(t *testing.T) (white, black *websocket.Conn) {
	t.Helper()
	white = e.dial(t, whiteID)
	expect(t, white, "joined")
	black = e.dial(t, blackID)
	expect(t, black, "joined")
	expect(t, white, "opponent_joined")
	return white, black
}

// expect reads the next message and fails unless it has the given type.
func expect(t *testing.T, conn *websocket.Conn, typ string) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg map[string]any
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("waiting for %q: %v", typ, err)
	}
	if msg["type"] != typ {
		t.Fatalf("got %v, want type %q", msg, typ)
	}
	return msg
}

func send(t *testing.T, conn *websocket.Conn, msg map[string]any) {
	t.Helper()
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("send %v: %v", msg, err)
	}
}

func move(from, to string) map[string]any {
	return map[string]any{"type": "move", "from": from, "to": to}
}

// expectClosed fails unless the server closes conn.
func expectClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected the socket to be closed")
	} else if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
		t.Fatal("socket still open")
	}
}

// expectEnded checks everything that must happen exactly once when a Game ends.
func expectEnded(t *testing.T, e *roomEnv, winner any, method string) {
	t.Helper()
	ctx := context.Background()

	deadline := time.Now().Add(2 * time.Second)
	for e.rooms.Exists(e.id) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if e.rooms.Exists(e.id) {
		t.Fatal("Room still registered after the Game ended")
	}

	for _, id := range []uint{whiteID, blackID} {
		if s, err := controllers.GetPlayerStatus(ctx, id); err != nil || s != "idle" {
			t.Errorf("player %d status = %q (%v), want idle", id, s, err)
		}
	}

	raw, err := initializers.RDB.Get(ctx, "game_result:"+e.id).Bytes()
	if err != nil {
		t.Fatalf("stored Result: %v", err)
	}
	var stored map[string]any
	json.Unmarshal(raw, &stored)
	if stored["winner"] != winner || stored["method"] != method {
		t.Errorf("stored Result = %v, want winner %v by %s", stored, winner, method)
	}
}

func TestRoom_JoinedCarriesTheFullState(t *testing.T) {
	e := newRoomEnv(t)

	white := e.dial(t, whiteID)
	joined := expect(t, white, "joined")
	if joined["color"] != "white" || joined["turn"] != "white" || joined["opponent_connected"] != false {
		t.Errorf("White's joined = %v", joined)
	}
	if joined["fen"] != "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1" {
		t.Errorf("fen = %v", joined["fen"])
	}
	clocks, _ := joined["clocks"].(map[string]any)
	if clocks["white_ms"] != float64(180000) || clocks["black_ms"] != float64(180000) || clocks["running"] != nil {
		t.Errorf("clocks = %v", joined["clocks"])
	}
	if moves, ok := joined["moves"].([]any); !ok || len(moves) != 0 {
		t.Errorf("moves = %v", joined["moves"])
	}

	black := e.dial(t, blackID)
	if j := expect(t, black, "joined"); j["color"] != "black" || j["opponent_connected"] != true {
		t.Errorf("Black's joined = %v", j)
	}
	expect(t, white, "opponent_joined")
}

func TestRoom_StrangersAndUnknownRoomsAreRefused(t *testing.T) {
	e := newRoomEnv(t)

	_, resp, err := websocket.DefaultDialer.Dial(e.url(e.id, 99), nil)
	if err == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("stranger: err=%v status=%v, want 403", err, resp)
	}
	_, resp, err = websocket.DefaultDialer.Dial(e.url("nope", whiteID), nil)
	if err == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown room: err=%v status=%v, want 404", err, resp)
	}
}

func TestRoom_MoveIsBroadcastWithClocks(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, move("e2", "e4"))
	for _, conn := range []*websocket.Conn{white, black} {
		m := expect(t, conn, "move")
		if m["from"] != "e2" || m["to"] != "e4" || m["turn"] != "black" {
			t.Errorf("move = %v", m)
		}
		if _, ok := m["clocks"].(map[string]any); !ok {
			t.Errorf("move has no clocks: %v", m)
		}
	}
}

func TestRoom_RejectionsGoOnlyToTheSender(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, black, move("e7", "e5"))
	if m := expect(t, black, "error"); m["code"] != "not_your_turn" {
		t.Errorf("error = %v", m)
	}
	send(t, white, move("e2", "e5"))
	if m := expect(t, white, "error"); m["code"] != "illegal_move" {
		t.Errorf("error = %v", m)
	}
	white.WriteMessage(websocket.TextMessage, []byte("{not json"))
	if m := expect(t, white, "error"); m["code"] != "bad_message" {
		t.Errorf("error = %v", m)
	}
	send(t, white, map[string]any{"type": "start"})
	if m := expect(t, white, "error"); m["code"] != "bad_message" {
		t.Errorf("error = %v", m)
	}

	// Black saw none of it: the next thing Black receives is White's real move.
	send(t, white, move("e2", "e4"))
	expect(t, black, "move")
}

func TestRoom_ResignEndsTheGameOnce(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, map[string]any{"type": "resign"})
	for _, conn := range []*websocket.Conn{white, black} {
		m := expect(t, conn, "game_over")
		if m["winner"] != "black" || m["method"] != "resign" {
			t.Errorf("game_over = %v", m)
		}
		expectClosed(t, conn)
	}
	expectEnded(t, e, "black", "resign")
}

func TestRoom_TimeoutEndsTheGameOnce(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, move("e2", "e4"))
	expect(t, white, "move")
	expect(t, black, "move")
	send(t, black, move("e7", "e5"))
	expect(t, white, "move")
	expect(t, black, "move")

	e.clock.Advance(3 * time.Minute)
	for _, conn := range []*websocket.Conn{white, black} {
		m := expect(t, conn, "game_over")
		if m["winner"] != "black" || m["method"] != "timeout" {
			t.Errorf("game_over = %v", m)
		}
		expectClosed(t, conn)
	}
	expectEnded(t, e, "black", "timeout")
}

func TestRoom_DisconnectEndsTheGameOnce(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	white.Close()
	m := expect(t, black, "game_over")
	if m["winner"] != "black" || m["method"] != "disconnect" {
		t.Errorf("game_over = %v", m)
	}
	expectClosed(t, black)
	expectEnded(t, e, "black", "disconnect")
}

func TestRoom_AbortEndsTheGameWithNoWinner(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, black, map[string]any{"type": "abort"})
	for _, conn := range []*websocket.Conn{white, black} {
		m := expect(t, conn, "game_over")
		if m["winner"] != nil || m["method"] != "aborted" {
			t.Errorf("game_over = %v", m)
		}
	}
	expectEnded(t, e, nil, "aborted")
}

func TestRoom_DrawOfferReachesBothPlayers(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, map[string]any{"type": "draw_offer"})
	for _, conn := range []*websocket.Conn{white, black} {
		if m := expect(t, conn, "draw_offered"); m["from"] != "white" {
			t.Errorf("draw_offered = %v, want from white", m)
		}
	}
}

func TestRoom_DeclinedDrawOfferReachesBothAndLeavesTheRoomLive(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, map[string]any{"type": "draw_offer"})
	expect(t, white, "draw_offered")
	expect(t, black, "draw_offered")

	send(t, black, map[string]any{"type": "draw_response", "accept": false})
	for _, conn := range []*websocket.Conn{white, black} {
		if m := expect(t, conn, "draw_declined"); m["from"] != "black" {
			t.Errorf("draw_declined = %v, want from black", m)
		}
	}

	// The Room is still live: White's next move is played and broadcast as usual.
	send(t, white, move("e2", "e4"))
	expect(t, white, "move")
	expect(t, black, "move")
}

// A draw_response with no accept field declines, rather than ending the Game on a malformed message.
func TestRoom_DrawResponseWithoutAnAcceptFieldDeclines(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, map[string]any{"type": "draw_offer"})
	expect(t, white, "draw_offered")
	expect(t, black, "draw_offered")

	send(t, black, map[string]any{"type": "draw_response"})
	for _, conn := range []*websocket.Conn{white, black} {
		if m := expect(t, conn, "draw_declined"); m["from"] != "black" {
			t.Errorf("draw_declined = %v, want from black", m)
		}
	}
}

func TestRoom_DrawRejectionsGoOnlyToTheSender(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	// Black has nothing to respond to.
	send(t, black, map[string]any{"type": "draw_response", "accept": true})
	if m := expect(t, black, "error"); m["code"] != "no_draw_offer" {
		t.Errorf("error = %v, want no_draw_offer", m)
	}

	send(t, white, map[string]any{"type": "draw_offer"})
	expect(t, white, "draw_offered")
	expect(t, black, "draw_offered")

	// White has already offered since their last move.
	send(t, white, map[string]any{"type": "draw_offer"})
	if m := expect(t, white, "error"); m["code"] != "draw_already_offered" {
		t.Errorf("error = %v, want draw_already_offered", m)
	}

	// Neither error reached the other player: the next thing each receives is White's real move.
	send(t, white, move("e2", "e4"))
	expect(t, white, "move")
	expect(t, black, "move")
}

func TestRoom_AcceptedDrawEndsTheGameOnce(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, map[string]any{"type": "draw_offer"})
	expect(t, white, "draw_offered")
	expect(t, black, "draw_offered")

	send(t, black, map[string]any{"type": "draw_response", "accept": true})
	for _, conn := range []*websocket.Conn{white, black} {
		m := expect(t, conn, "game_over")
		if m["winner"] != nil || m["method"] != "agreement" {
			t.Errorf("game_over = %v, want a draw by agreement", m)
		}
		expectClosed(t, conn)
	}
	expectEnded(t, e, nil, "agreement")
}

func TestRoom_OfferingIntoAnOpenOfferEndsTheGame(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, map[string]any{"type": "draw_offer"})
	expect(t, white, "draw_offered")
	expect(t, black, "draw_offered")

	send(t, black, map[string]any{"type": "draw_offer"})
	for _, conn := range []*websocket.Conn{white, black} {
		m := expect(t, conn, "game_over")
		if m["winner"] != nil || m["method"] != "agreement" {
			t.Errorf("game_over = %v, want a draw by agreement", m)
		}
	}
	expectEnded(t, e, nil, "agreement")
}

func TestRoom_OfferersMoveAnnouncesTheWithdrawal(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, map[string]any{"type": "draw_offer"})
	expect(t, white, "draw_offered")
	expect(t, black, "draw_offered")

	send(t, white, move("e2", "e4"))
	for _, conn := range []*websocket.Conn{white, black} {
		expect(t, conn, "move")
		if m := expect(t, conn, "draw_withdrawn"); m["from"] != "white" {
			t.Errorf("draw_withdrawn = %v, want from white", m)
		}
	}
}

// A reconnecting player is told about an offer that is still standing, in both directions: the
// offerer so their Draw button stays spent, the recipient so they can still answer it.
func TestRoom_JoinedReplaysAStandingDrawOffer(t *testing.T) {
	e := newRoomEnv(t)

	white := e.dial(t, whiteID)
	if j := expect(t, white, "joined"); j["draw_offer"] != nil || j["can_offer"] != true {
		t.Errorf("White's joined = %v, want no offer and can_offer true", j)
	}
	black := e.dial(t, blackID)
	expect(t, black, "joined")
	expect(t, white, "opponent_joined")

	send(t, white, map[string]any{"type": "draw_offer"})
	expect(t, white, "draw_offered")
	expect(t, black, "draw_offered")

	// The offerer reconnects: the offer is theirs, and their right to offer is spent.
	white2 := e.dial(t, whiteID)
	if j := expect(t, white2, "joined"); j["draw_offer"] != "white" || j["can_offer"] != false {
		t.Errorf("offerer's joined = %v, want draw_offer white and can_offer false", j)
	}
	expect(t, black, "opponent_joined")

	// The recipient reconnects: the same offer, and their own right untouched.
	black2 := e.dial(t, blackID)
	if j := expect(t, black2, "joined"); j["draw_offer"] != "white" || j["can_offer"] != true {
		t.Errorf("recipient's joined = %v, want draw_offer white and can_offer true", j)
	}
}

func TestRoom_CommandArrivingAfterTheAbortWindowStillEndsTheRoom(t *testing.T) {
	e := newRoomEnv(t)
	white, black := e.joinBoth(t)

	send(t, white, move("e2", "e4"))
	expect(t, white, "move")
	expect(t, black, "move")

	// Black answers after their 30 s window: the move must not hang the Room.
	e.clock.Advance(31 * time.Second)
	send(t, black, move("e7", "e5"))
	for _, conn := range []*websocket.Conn{white, black} {
		m := expect(t, conn, "game_over")
		if m["winner"] != nil || m["method"] != "aborted" {
			t.Errorf("game_over = %v", m)
		}
	}
	expectEnded(t, e, nil, "aborted")
}
