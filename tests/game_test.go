package tests

import (
	"testing"
	"time"

	"github.com/vareja0/go-jwt/game"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// at returns t0 shifted by d, so tests read as "n seconds into the Game".
func at(d time.Duration) time.Time { return t0.Add(d) }

func newGame(t *testing.T) *game.Game {
	t.Helper()
	return game.New(3*time.Minute, t0)
}

// play makes each move in order, alternating colours from White, and fails the test on any rejection.
func play(t *testing.T, g *game.Game, now time.Time, moves ...string) {
	t.Helper()
	color := g.State(now).Turn
	for _, m := range moves {
		if _, err := g.Move(color, m, now); err != nil {
			t.Fatalf("move %s: %v", m, err)
		}
		color = color.Opponent()
	}
}

func TestGame_LegalMovePassesTheTurn(t *testing.T) {
	g := newGame(t)

	moved, err := g.Move(game.White, "e2e4", at(time.Second))
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.Turn != game.Black {
		t.Errorf("turn = %v, want black", moved.Turn)
	}
	if moved.FEN != "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1" {
		t.Errorf("FEN = %q", moved.FEN)
	}
	if moved.Result != nil {
		t.Errorf("unexpected Result %+v", moved.Result)
	}
}

func TestGame_MoveOutOfTurnIsRejected(t *testing.T) {
	g := newGame(t)

	_, err := g.Move(game.Black, "e7e5", at(time.Second))
	if err != game.ErrNotYourTurn {
		t.Fatalf("err = %v, want ErrNotYourTurn", err)
	}
	if g.State(at(time.Second)).Turn != game.White {
		t.Errorf("turn changed after rejected move")
	}
}

func TestGame_IllegalMoveIsRejected(t *testing.T) {
	g := newGame(t)

	_, err := g.Move(game.White, "e2e5", at(time.Second))
	if err != game.ErrIllegalMove {
		t.Fatalf("err = %v, want ErrIllegalMove", err)
	}
}

func TestGame_ClocksStartAfterBothFirstMoves(t *testing.T) {
	g := newGame(t)

	play(t, g, at(10*time.Second), "e2e4")
	if c := g.State(at(20 * time.Second)).Clocks; c != (game.Clocks{White: 3 * time.Minute, Black: 3 * time.Minute, Running: game.NoColor}) {
		t.Fatalf("after White's first move: %+v, want both full and none running", c)
	}

	play(t, g, at(25*time.Second), "e7e5")
	if c := g.State(at(40 * time.Second)).Clocks; c != (game.Clocks{White: 3*time.Minute - 15*time.Second, Black: 3 * time.Minute, Running: game.White}) {
		t.Fatalf("15s into White's second move: %+v", c)
	}
}

func TestGame_OnlySideToMoveIsCharged(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")

	moved, err := g.Move(game.White, "g1f3", at(20*time.Second))
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.Clocks != (game.Clocks{White: 3*time.Minute - 20*time.Second, Black: 3 * time.Minute, Running: game.Black}) {
		t.Fatalf("after White's move: %+v", moved.Clocks)
	}
	if c := g.State(at(30 * time.Second)).Clocks; c != (game.Clocks{White: 3*time.Minute - 20*time.Second, Black: 3*time.Minute - 10*time.Second, Running: game.Black}) {
		t.Fatalf("10s into Black's move: %+v", c)
	}
}

func TestGame_IllegalFirstMoveDoesNotStartAnything(t *testing.T) {
	g := newGame(t)

	g.Move(game.Black, "e7e5", at(time.Second))
	g.Move(game.White, "e2e5", at(2*time.Second))
	play(t, g, at(3*time.Second), "e2e4", "e7e5")

	if c := g.State(at(3 * time.Second)).Clocks; c != (game.Clocks{White: 3 * time.Minute, Black: 3 * time.Minute, Running: game.White}) {
		t.Fatalf("clocks after rejected moves then both first moves: %+v", c)
	}
}

func TestGame_RunningOutOfTimeLosesOnTimeout(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")

	if r := g.Result(at(3*time.Minute - time.Millisecond)); r != nil {
		t.Fatalf("ended early: %+v", r)
	}
	r := g.Result(at(3 * time.Minute))
	if r == nil || *r != (game.Result{Winner: game.Black, Method: game.Timeout}) {
		t.Fatalf("Result = %+v, want Black by timeout", r)
	}
}

func TestGame_MoveAfterFlagFallsEndsOnTimeoutInstead(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")

	_, err := g.Move(game.White, "g1f3", at(4*time.Minute))
	if err != game.ErrGameOver {
		t.Fatalf("err = %v, want ErrGameOver", err)
	}
	if r := g.State(at(4 * time.Minute)).Result; r == nil || *r != (game.Result{Winner: game.Black, Method: game.Timeout}) {
		t.Fatalf("Result = %+v, want Black by timeout", r)
	}
}

var aborted = game.Result{Winner: game.NoColor, Method: game.Aborted}

func TestGame_EitherSideMayAbortBeforeBothHaveMoved(t *testing.T) {
	g := newGame(t)
	play(t, g, at(time.Second), "e2e4")

	r, err := g.Abort(game.White, at(2*time.Second))
	if err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if *r != aborted {
		t.Fatalf("Result = %+v, want aborted", r)
	}
}

func TestGame_AbortIsRejectedOnceBothHaveMoved(t *testing.T) {
	g := newGame(t)
	play(t, g, at(time.Second), "e2e4", "e7e5")

	if _, err := g.Abort(game.Black, at(2*time.Second)); err != game.ErrCannotAbort {
		t.Fatalf("err = %v, want ErrCannotAbort", err)
	}
}

func TestGame_AbortsWhenWhiteDoesNotMoveWithin30Seconds(t *testing.T) {
	g := newGame(t)

	if r := g.Result(at(29 * time.Second)); r != nil {
		t.Fatalf("aborted early: %+v", r)
	}
	if r := g.Result(at(30 * time.Second)); r == nil || *r != aborted {
		t.Fatalf("Result = %+v, want aborted", r)
	}
}

func TestGame_AbortsWhenBlackDoesNotAnswerWithin30Seconds(t *testing.T) {
	g := newGame(t)
	play(t, g, at(20*time.Second), "e2e4")

	if r := g.Result(at(49 * time.Second)); r != nil {
		t.Fatalf("aborted early: %+v", r)
	}
	if _, err := g.Move(game.Black, "e7e5", at(50*time.Second)); err != game.ErrGameOver {
		t.Fatalf("err = %v, want ErrGameOver", err)
	}
	if r := g.State(at(50 * time.Second)).Result; r == nil || *r != aborted {
		t.Fatalf("Result = %+v, want aborted", r)
	}
}

func TestGame_ResignHandsTheWinToTheOpponent(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")

	r, err := g.Resign(game.White, at(5*time.Second))
	if err != nil {
		t.Fatalf("Resign: %v", err)
	}
	if *r != (game.Result{Winner: game.Black, Method: game.Resign}) {
		t.Fatalf("Result = %+v, want Black by resign", r)
	}
}

func TestGame_DisconnectHandsTheWinToTheOpponent(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")

	r, err := g.Disconnect(game.Black, at(5*time.Second))
	if err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if *r != (game.Result{Winner: game.White, Method: game.Disconnect}) {
		t.Fatalf("Result = %+v, want White by disconnect", r)
	}
}

func TestGame_EndsExactlyOnce(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")

	// White resigns after their flag has already fallen: the timeout stands.
	if _, err := g.Resign(game.White, at(5*time.Minute)); err != game.ErrGameOver {
		t.Fatalf("Resign err = %v, want ErrGameOver", err)
	}
	if _, err := g.Disconnect(game.Black, at(5*time.Minute)); err != game.ErrGameOver {
		t.Fatalf("Disconnect err = %v, want ErrGameOver", err)
	}
	if _, err := g.Abort(game.Black, at(5*time.Minute)); err != game.ErrGameOver {
		t.Fatalf("Abort err = %v, want ErrGameOver", err)
	}
	if r := g.State(at(5 * time.Minute)).Result; *r != (game.Result{Winner: game.Black, Method: game.Timeout}) {
		t.Fatalf("Result = %+v, want Black by timeout", r)
	}
}

func TestGame_PositionsThatEndTheGame(t *testing.T) {
	cases := []struct {
		name  string
		fen   string // "" for the standard start
		moves []string
		want  game.Result
	}{
		{"checkmate", "", []string{"f2f3", "e7e5", "g2g4", "d8h4"}, game.Result{Winner: game.Black, Method: game.Checkmate}},
		{"stalemate", "k7/8/8/1Q6/8/8/8/7K w - - 0 1", []string{"b5b6"}, game.Result{Method: game.Stalemate}},
		{"insufficient material", "k7/8/8/8/8/8/1n6/K7 w - - 0 1", []string{"a1b2"}, game.Result{Method: game.InsufficientMaterial}},
		{"threefold repetition", "", []string{"g1f3", "g8f6", "f3g1", "f6g8", "g1f3", "g8f6", "f3g1", "f6g8"}, game.Result{Method: game.ThreefoldRepetition}},
		{"fifty moves", "k7/8/8/8/8/8/8/KR6 w - - 99 60", []string{"b1b2"}, game.Result{Method: game.FiftyMoves}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var g *game.Game
			if tc.fen == "" {
				g = newGame(t)
			} else {
				g = game.New(3*time.Minute, t0, game.StartingFEN(tc.fen))
			}

			last := tc.moves[len(tc.moves)-1]
			play(t, g, at(time.Second), tc.moves[:len(tc.moves)-1]...)
			moved, err := g.Move(g.State(at(time.Second)).Turn, last, at(time.Second))
			if err != nil {
				t.Fatalf("final move %s: %v", last, err)
			}
			if moved.Result == nil || *moved.Result != tc.want {
				t.Fatalf("Result = %+v, want %+v", moved.Result, tc.want)
			}
		})
	}
}

func TestGame_StateListsMovesSoFar(t *testing.T) {
	g := newGame(t)
	play(t, g, at(time.Second), "e2e4", "e7e5", "g1f3")

	got := g.State(at(2 * time.Second)).Moves
	want := []string{"e2e4", "e7e5", "g1f3"}
	if len(got) != len(want) {
		t.Fatalf("Moves = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Moves = %v, want %v", got, want)
		}
	}
}
