// Package game models one chess Game: its position, moves, Clocks and Result.
// It does no I/O; callers pass the current time into every command.
package game

import (
	"errors"
	"time"

	"github.com/notnil/chess"
)

// Color is a side of the board.
type Color int

const (
	NoColor Color = iota
	White
	Black
)

// String returns "white", "black", or "" for NoColor.
func (c Color) String() string {
	switch c {
	case White:
		return "white"
	case Black:
		return "black"
	}
	return ""
}

// Opponent returns the other side.
func (c Color) Opponent() Color {
	if c == White {
		return Black
	}
	return White
}

var (
	ErrNotYourTurn = errors.New("not your turn")
	ErrIllegalMove = errors.New("illegal move")
	ErrGameOver    = errors.New("game over")
	ErrCannotAbort = errors.New("cannot abort after both sides have moved")
)

// firstMoveWindow is how long the side to move has to make their first move before the Game Aborts.
const firstMoveWindow = 30 * time.Second

// Moved is what a successful move reports.
type Moved struct {
	FEN    string
	Turn   Color
	Clocks Clocks
	Result *Result
}

// Clocks is each side's remaining time and which Clock is running (NoColor before both first moves).
type Clocks struct {
	White   time.Duration
	Black   time.Duration
	Running Color
}

// Method is how a Game ended.
type Method string

const (
	Checkmate            Method = "checkmate"
	Stalemate            Method = "stalemate"
	InsufficientMaterial Method = "insufficient_material"
	ThreefoldRepetition  Method = "threefold_repetition"
	FiftyMoves           Method = "fifty_moves"
	Resign               Method = "resign"
	Timeout              Method = "timeout"
	Disconnect           Method = "disconnect"
	Aborted              Method = "aborted"
)

// Result is how a finished Game ended. Winner is NoColor for a draw or an Abort.
type Result struct {
	Winner Color
	Method Method
}

// State is a full snapshot of the Game, as shown to a joining player.
type State struct {
	FEN    string
	Moves  []string // UCI, in order
	Turn   Color
	Clocks Clocks
	Result *Result
}

// Game is one chess Game. It is not safe for concurrent use; one owner drives it.
type Game struct {
	chess      *chess.Game
	timeLeft   map[Color]time.Duration
	lastMoveAt time.Time
	moves      []string // UCI, in order
	result     *Result
}

// Option configures a new Game.
type Option func(*[]func(*chess.Game))

// StartingFEN starts the Game from fen instead of the standard position. It panics in New if fen is invalid.
func StartingFEN(fen string) Option {
	return func(opts *[]func(*chess.Game)) {
		f, err := chess.FEN(fen)
		if err != nil {
			panic(err)
		}
		*opts = append(*opts, f)
	}
}

// New starts a Game where each Clock gets timeControl, created at now.
func New(timeControl time.Duration, now time.Time, opts ...Option) *Game {
	var chessOpts []func(*chess.Game)
	for _, o := range opts {
		o(&chessOpts)
	}
	return &Game{
		chess:      chess.NewGame(chessOpts...),
		timeLeft:   map[Color]time.Duration{White: timeControl, Black: timeControl},
		lastMoveAt: now,
	}
}

// Move plays a UCI move (e.g. "e2e4", "e7e8q") for color.
func (g *Game) Move(color Color, uci string, now time.Time) (Moved, error) {
	g.settle(now)
	if g.result != nil {
		return Moved{}, ErrGameOver
	}
	if color != g.turn() {
		return Moved{}, ErrNotYourTurn
	}
	move, err := chess.UCINotation{}.Decode(g.chess.Position(), uci)
	if err != nil {
		return Moved{}, ErrIllegalMove
	}
	if err := g.chess.Move(move); err != nil {
		return Moved{}, ErrIllegalMove
	}
	g.charge(color, now)
	g.moves = append(g.moves, uci)
	g.lastMoveAt = now
	g.endOnPosition(color)
	return Moved{FEN: g.chess.FEN(), Turn: g.turn(), Clocks: g.clocks(now), Result: g.result}, nil
}

// Abort ends the Game with no winner. Allowed for either side only before both have made their first move.
func (g *Game) Abort(color Color, now time.Time) (*Result, error) {
	g.settle(now)
	if g.result != nil {
		return nil, ErrGameOver
	}
	if g.clocksRunning() {
		return nil, ErrCannotAbort
	}
	g.end(Result{Method: Aborted})
	return g.result, nil
}

// Resign ends the Game as a win for color's opponent.
func (g *Game) Resign(color Color, now time.Time) (*Result, error) {
	return g.forfeit(color, Resign, now)
}

// Disconnect ends the Game as a win for color's opponent, because color left the Room.
func (g *Game) Disconnect(color Color, now time.Time) (*Result, error) {
	return g.forfeit(color, Disconnect, now)
}

func (g *Game) forfeit(color Color, method Method, now time.Time) (*Result, error) {
	g.settle(now)
	if g.result != nil {
		return nil, ErrGameOver
	}
	g.charge(g.turn(), now)
	g.end(Result{Winner: color.Opponent(), Method: method})
	return g.result, nil
}

// Result settles the Clocks at now and returns how the Game ended, or nil while it is still in progress.
// A Game can end inside any call (a Clock or the Abort window may have run out), so owners check this after each one.
func (g *Game) Result(now time.Time) *Result {
	g.settle(now)
	return g.result
}

// State returns the current snapshot of the Game.
func (g *Game) State(now time.Time) State {
	g.settle(now)
	return State{FEN: g.chess.FEN(), Moves: append([]string{}, g.moves...), Turn: g.turn(), Clocks: g.clocks(now), Result: g.result}
}

// settle ends the Game if time has run out by now, or Aborts it if a first move is overdue.
func (g *Game) settle(now time.Time) {
	if g.result != nil {
		return
	}
	if !g.clocksRunning() {
		if now.Sub(g.lastMoveAt) >= firstMoveWindow {
			g.end(Result{Method: Aborted})
		}
		return
	}
	side := g.turn()
	if g.timeLeft[side]-now.Sub(g.lastMoveAt) <= 0 {
		g.timeLeft[side] = 0
		g.end(Result{Winner: side.Opponent(), Method: Timeout})
	}
}

// endOnPosition ends the Game if mover's move produced a finished position.
// Threefold repetition and the fifty-move rule end it automatically, with no claim.
func (g *Game) endOnPosition(mover Color) {
	switch g.chess.Method() {
	case chess.Checkmate:
		g.end(Result{Winner: mover, Method: Checkmate})
		return
	case chess.Stalemate:
		g.end(Result{Method: Stalemate})
		return
	case chess.InsufficientMaterial:
		g.end(Result{Method: InsufficientMaterial})
		return
	case chess.FivefoldRepetition:
		g.end(Result{Method: ThreefoldRepetition})
		return
	case chess.SeventyFiveMoveRule:
		g.end(Result{Method: FiftyMoves})
		return
	}
	for _, m := range g.chess.EligibleDraws() {
		switch m {
		case chess.ThreefoldRepetition:
			g.end(Result{Method: ThreefoldRepetition})
			return
		case chess.FiftyMoveRule:
			g.end(Result{Method: FiftyMoves})
			return
		}
	}
}

// charge deducts the time since the last move from side, once Clocks are running.
// side must be the side whose Clock was running since the last move.
func (g *Game) charge(side Color, now time.Time) {
	if g.clocksRunning() {
		g.timeLeft[side] -= now.Sub(g.lastMoveAt)
	}
}

// end freezes the Game with r as its Result.
func (g *Game) end(r Result) {
	g.result = &r
}

// clocksRunning reports whether both sides have made their first move.
func (g *Game) clocksRunning() bool { return len(g.moves) >= 2 }

// clocks returns the Clocks as of now, charging the side to move for time since the last move.
func (g *Game) clocks(now time.Time) Clocks {
	c := Clocks{White: g.timeLeft[White], Black: g.timeLeft[Black]}
	if !g.clocksRunning() || g.result != nil {
		return c
	}
	c.Running = g.turn()
	elapsed := now.Sub(g.lastMoveAt)
	if c.Running == White {
		c.White -= elapsed
	} else {
		c.Black -= elapsed
	}
	return c
}

func (g *Game) turn() Color {
	if g.chess.Position().Turn() == chess.White {
		return White
	}
	return Black
}
