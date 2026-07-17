package main

import "testing"

// Drives the hotseat backend without any UI: connect, play, reject, and finish.
func TestHotseatPlay(t *testing.T) {
	b := newHotseatBackend(newBoardState(9, 9), nil)

	var last boardState
	got := 0
	emit := func(st boardState) { last = st; got++ }

	if err := b.Connect(emit, nil); err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Connect emits the starting position: empty board, black to move.
	if got != 1 || last.playerToMove != black {
		t.Fatalf("bad initial emit: got=%d toMove=%v", got, last.playerToMove)
	}

	// Black plays; stone lands and the turn flips to white.
	if err := b.SubmitMove(move{x: 2, y: 2, color: black}); err != nil {
		t.Fatalf("black move: %v", err)
	}
	if last.grid[2][2] != black || last.playerToMove != white {
		t.Fatalf("black move not applied / turn not flipped: %v", last.playerToMove)
	}

	// Replaying the same point is rejected and does not emit.
	before := got
	if err := b.SubmitMove(move{x: 2, y: 2, color: white}); err != errOccupied {
		t.Fatalf("want errOccupied, got %v", err)
	}
	if got != before {
		t.Fatalf("rejected move should not emit")
	}

	// Playing out of turn is rejected.
	if err := b.SubmitMove(move{x: 3, y: 3, color: black}); err != errNotYourTurn {
		t.Fatalf("want errNotYourTurn, got %v", err)
	}

	// Two passes in a row finish the game.
	if err := b.SubmitMove(move{x: -1, y: -1, color: white}); err != nil {
		t.Fatalf("white pass: %v", err)
	}
	if err := b.SubmitMove(move{x: -1, y: -1, color: black}); err != nil {
		t.Fatalf("black pass: %v", err)
	}
	if last.phase != phaseFinished {
		t.Fatalf("want finished after two passes, got %v", last.phase)
	}
}
