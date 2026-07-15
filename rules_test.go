package main

import "testing"

// Builds a grid from rows: '.'=empty, 'X'=black, 'O'=white.
func gridFrom(rows ...string) [][]stoneColor {
	g := make([][]stoneColor, len(rows))
	for y, r := range rows {
		g[y] = make([]stoneColor, len(r))
		for x, ch := range r {
			switch ch {
			case 'X':
				g[y][x] = black
			case 'O':
				g[y][x] = white
			}
		}
	}
	return g
}

// Capture: a group is removed the instant its last liberty is filled. Here the
// lone white stone at (1,1) has one liberty left, the point directly below it
// at (1,2). Black plays there; white now has zero liberties and is lifted off.
// (Left = before black's move, right = after. e marks the captured point.)
//
//	.X.        .X.
//	XOX   -->  XeX
//	...        .X.     black's new stone is the bottom X
func TestApplyMoveCapture(t *testing.T) {
	cur := boardState{
		grid: gridFrom(
			".X.",
			"XOX",
			"...",
		),
		playerToMove: black,
		phase:        phasePlay,
	}
	next, err := applyMove(cur, nil, move{x: 1, y: 2, color: black})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.grid[1][1] != empty {
		t.Fatalf("white stone not captured: %v", next.grid[1][1])
	}
}

// Suicide: playing into a point with no liberties is illegal when it captures
// nothing. The empty center is surrounded on all four sides by white; a black
// stone there would have zero liberties, and no white group is reduced to zero
// by the play, so there is nothing to capture. The move is rejected.
//
//	.O.
//	O.O    black may not play the center
//	.O.
func TestApplyMoveSuicide(t *testing.T) {
	cur := boardState{
		grid: gridFrom(
			".O.",
			"O.O",
			".O.",
		),
		playerToMove: black,
		phase:        phasePlay,
	}
	if _, err := applyMove(cur, nil, move{x: 1, y: 1, color: black}); err != errSuicide {
		t.Fatalf("want errSuicide, got %v", err)
	}
}

// Suicide that captures: the exception to the suicide rule. Captures resolve
// before the placed stone's own liberties are checked, so a move that would be
// self-atari is legal if it first removes an enemy group. The white column has
// one liberty at (0,2); black plays there. That fills white's last liberty and
// captures the whole column, which in turn gives black's new stone liberties.
//
//	OX.        .X.
//	OX.   -->  .X.     black plays (0,2): both white stones captured
//	...        X..
func TestApplyMoveSuicideThatCaptures(t *testing.T) {
	cur := boardState{
		grid: gridFrom(
			"OX.",
			"OX.",
			"...",
		),
		playerToMove: black,
		phase:        phasePlay,
	}
	next, err := applyMove(cur, nil, move{x: 0, y: 2, color: black})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.grid[0][0] != empty || next.grid[1][0] != empty {
		t.Fatalf("white group not captured: %v", next.grid)
	}
}

// Ko: a player may not immediately recapture in a way that recreates the board
// as it stood one move ago — that would loop forever. prevGrid is the position
// before black took the ko (white stone at (1,1)). cur is the position right
// after black took it (black at (2,1), (1,1) now empty). White's natural reply
// is to recapture at (1,1), which would lift black's (2,1) stone and restore the
// exact prevGrid — so the rule forbids it. White must play elsewhere first.
//
//	prevGrid        cur (black took ko)     white's illegal recapture
//	.XO.            .XO.                    would reproduce prevGrid
//	XO.O    -->     X.XO            -->     rejected (errKo)
//	.XO.            .XO.
func TestApplyMoveKo(t *testing.T) {
	prevGrid := gridFrom(
		".XO.",
		"XO.O",
		".XO.",
	)
	cur := boardState{
		grid: gridFrom(
			".XO.",
			"X.XO",
			".XO.",
		),
		playerToMove: white,
		lastMove:     move{x: 2, y: 1, color: black},
		phase:        phasePlay,
	}
	if _, err := applyMove(cur, prevGrid, move{x: 1, y: 1, color: white}); err != errKo {
		t.Fatalf("want errKo, got %v", err)
	}
}

// Two passes finish the game: when both players pass in succession, play ends.
// cur records that black has just passed (lastMove is a pass); white passing in
// turn drives the phase to finished, after which the board is read-only.
func TestApplyMoveTwoPassesFinish(t *testing.T) {
	cur := boardState{
		grid:         newBoardState(9, 9).grid,
		playerToMove: white,
		lastMove:     move{x: -1, y: -1, color: black}, // black just passed
		phase:        phasePlay,
	}
	next, err := applyMove(cur, nil, move{x: -1, y: -1, color: white})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.phase != phaseFinished {
		t.Fatalf("want finished after two passes, got %v", next.phase)
	}
}
