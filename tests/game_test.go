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

var drawn = game.Result{Winner: game.NoColor, Method: game.Agreement}

func TestGame_AcceptedDrawOfferEndsTheGameByAgreement(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")

	if _, err := g.OfferDraw(game.White, at(time.Second)); err != nil {
		t.Fatalf("OfferDraw: %v", err)
	}
	r, err := g.RespondToDraw(game.Black, true, at(2*time.Second))
	if err != nil {
		t.Fatalf("RespondToDraw: %v", err)
	}
	if r == nil || *r != drawn {
		t.Fatalf("Result = %+v, want a draw by agreement", r)
	}
	if sr := g.State(at(2 * time.Second)).Result; sr == nil || *sr != drawn {
		t.Fatalf("State Result = %+v, want a draw by agreement", sr)
	}
}

func TestGame_DeclinedDrawOfferLeavesTheGameInProgress(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")
	g.OfferDraw(game.White, at(time.Second))

	r, err := g.RespondToDraw(game.Black, false, at(2*time.Second))
	if err != nil {
		t.Fatalf("RespondToDraw: %v", err)
	}
	if r != nil {
		t.Fatalf("Result = %+v, want the Game still in progress", r)
	}
	state := g.State(at(2 * time.Second))
	if state.Result != nil {
		t.Fatalf("State Result = %+v, want nil", state.Result)
	}
	if state.DrawOffer != game.NoColor {
		t.Errorf("DrawOffer = %v, want none after a decline", state.DrawOffer)
	}
	if _, err := g.Move(game.White, "g1f3", at(3*time.Second)); err != nil {
		t.Fatalf("Move after a declined offer: %v", err)
	}
}

func TestGame_OnlyTheSideAnOfferStandsAgainstMayRespond(t *testing.T) {
	t.Run("no offer open", func(t *testing.T) {
		for _, accept := range []bool{true, false} {
			g := newGame(t)
			play(t, g, at(0), "e2e4", "e7e5")

			if _, err := g.RespondToDraw(game.Black, accept, at(time.Second)); err != game.ErrNoDrawOffer {
				t.Fatalf("RespondToDraw(accept=%v) err = %v, want ErrNoDrawOffer", accept, err)
			}
			if r := g.State(at(time.Second)).Result; r != nil {
				t.Fatalf("Result = %+v, want the Game still in progress", r)
			}
		}
	})

	t.Run("responding to your own offer", func(t *testing.T) {
		g := newGame(t)
		play(t, g, at(0), "e2e4", "e7e5")
		g.OfferDraw(game.White, at(time.Second))

		if _, err := g.RespondToDraw(game.White, true, at(2*time.Second)); err != game.ErrNoDrawOffer {
			t.Fatalf("err = %v, want ErrNoDrawOffer", err)
		}
		state := g.State(at(2 * time.Second))
		if state.Result != nil {
			t.Fatalf("Result = %+v, want the Game still in progress", state.Result)
		}
		if state.DrawOffer != game.White {
			t.Errorf("DrawOffer = %v, want White's offer still standing", state.DrawOffer)
		}
	})
}

func TestGame_DrawCommandsAreRejectedOnceTheGameIsOver(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")
	g.OfferDraw(game.White, at(time.Second))

	// White's flag falls while the offer is still standing.
	if _, err := g.OfferDraw(game.Black, at(4*time.Minute)); err != game.ErrGameOver {
		t.Fatalf("OfferDraw err = %v, want ErrGameOver", err)
	}
	if _, err := g.RespondToDraw(game.Black, true, at(4*time.Minute)); err != game.ErrGameOver {
		t.Fatalf("RespondToDraw err = %v, want ErrGameOver", err)
	}
	state := g.State(at(4 * time.Minute))
	if state.Result == nil || *state.Result != (game.Result{Winner: game.Black, Method: game.Timeout}) {
		t.Fatalf("Result = %+v, want Black by timeout", state.Result)
	}
	if state.DrawOffer != game.NoColor {
		t.Errorf("DrawOffer = %v, want none once the Game is over", state.DrawOffer)
	}
}

func TestGame_StateReportsWhoHasAnOfferOpen(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")

	if o := g.State(at(time.Second)).DrawOffer; o != game.NoColor {
		t.Errorf("DrawOffer = %v, want none before any offer", o)
	}
	g.OfferDraw(game.White, at(time.Second))
	if o := g.State(at(2 * time.Second)).DrawOffer; o != game.White {
		t.Errorf("DrawOffer = %v, want White", o)
	}
	g.Resign(game.Black, at(3*time.Second))
	if o := g.State(at(3 * time.Second)).DrawOffer; o != game.NoColor {
		t.Errorf("DrawOffer = %v, want none once the Game has ended", o)
	}
}

func TestGame_OnlyOneDrawOfferPerOwnMove(t *testing.T) {
	t.Run("offering twice without moving", func(t *testing.T) {
		g := newGame(t)
		play(t, g, at(0), "e2e4", "e7e5")
		g.OfferDraw(game.White, at(time.Second))

		if _, err := g.OfferDraw(game.White, at(2*time.Second)); err != game.ErrDrawAlreadyOffered {
			t.Fatalf("err = %v, want ErrDrawAlreadyOffered", err)
		}
		if o := g.State(at(2 * time.Second)).DrawOffer; o != game.White {
			t.Errorf("DrawOffer = %v, want White's first offer still standing", o)
		}
	})

	t.Run("offering again after a decline", func(t *testing.T) {
		g := newGame(t)
		play(t, g, at(0), "e2e4", "e7e5")
		g.OfferDraw(game.White, at(time.Second))
		g.RespondToDraw(game.Black, false, at(2*time.Second))

		if _, err := g.OfferDraw(game.White, at(3*time.Second)); err != game.ErrDrawAlreadyOffered {
			t.Fatalf("err = %v, want ErrDrawAlreadyOffered", err)
		}
	})

	t.Run("moving earns the right back", func(t *testing.T) {
		g := newGame(t)
		play(t, g, at(0), "e2e4", "e7e5")
		g.OfferDraw(game.White, at(time.Second))
		g.RespondToDraw(game.Black, false, at(2*time.Second))
		play(t, g, at(3*time.Second), "g1f3")

		if _, err := g.OfferDraw(game.White, at(4*time.Second)); err != nil {
			t.Fatalf("OfferDraw after moving: %v", err)
		}
		if o := g.State(at(4 * time.Second)).DrawOffer; o != game.White {
			t.Errorf("DrawOffer = %v, want White", o)
		}
	})
}

func TestGame_CanOfferDrawTracksTheRightToOffer(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")

	if !g.CanOfferDraw(game.White, at(time.Second)) {
		t.Error("White cannot offer at the start of the Game")
	}
	g.OfferDraw(game.White, at(time.Second))
	if g.CanOfferDraw(game.White, at(2*time.Second)) {
		t.Error("White can offer again with their own offer still standing")
	}
	g.RespondToDraw(game.Black, false, at(2*time.Second))
	if g.CanOfferDraw(game.White, at(3*time.Second)) {
		t.Error("a decline handed White's right back without them moving")
	}
	play(t, g, at(3*time.Second), "g1f3")
	if !g.CanOfferDraw(game.White, at(4*time.Second)) {
		t.Error("moving did not earn White's right back")
	}
	g.Resign(game.Black, at(5*time.Second))
	if g.CanOfferDraw(game.White, at(5*time.Second)) {
		t.Error("White can offer a draw in a finished Game")
	}
}

func TestGame_OfferersMoveWithdrawsTheirOffer(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")
	g.OfferDraw(game.White, at(time.Second))

	moved, err := g.Move(game.White, "g1f3", at(2*time.Second))
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if !moved.DrawWithdrawn {
		t.Error("DrawWithdrawn = false, want the offer withdrawn by the offerer's move")
	}
	if o := g.State(at(2 * time.Second)).DrawOffer; o != game.NoColor {
		t.Errorf("DrawOffer = %v, want none", o)
	}
	if _, err := g.RespondToDraw(game.Black, true, at(3*time.Second)); err != game.ErrNoDrawOffer {
		t.Fatalf("accepting a withdrawn offer: err = %v, want ErrNoDrawOffer", err)
	}
}

func TestGame_OpponentsMoveLeavesTheOfferStanding(t *testing.T) {
	g := newGame(t)
	play(t, g, at(0), "e2e4", "e7e5")
	play(t, g, at(time.Second), "g1f3")
	g.OfferDraw(game.White, at(2*time.Second))

	moved, err := g.Move(game.Black, "b8c6", at(3*time.Second))
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.DrawWithdrawn {
		t.Error("DrawWithdrawn = true, want the opponent's move to leave the offer alone")
	}
	if o := g.State(at(3 * time.Second)).DrawOffer; o != game.White {
		t.Errorf("DrawOffer = %v, want White's offer still standing", o)
	}
	r, err := g.RespondToDraw(game.Black, true, at(4*time.Second))
	if err != nil {
		t.Fatalf("accepting after moving: %v", err)
	}
	if r == nil || *r != drawn {
		t.Fatalf("Result = %+v, want a draw by agreement", r)
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
