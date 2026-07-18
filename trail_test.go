package main

import (
	"encoding/json"
	"testing"
)

// trail() returns the last trailLen moves, newest first.
func TestBoardStateTrail(t *testing.T) {
	st := boardState{moves: []move{
		{x: 0, y: 0}, {x: 1, y: 1}, {x: 2, y: 2}, {x: 3, y: 3},
	}}
	tr := st.trail()
	if len(tr) != trailLen {
		t.Fatalf("want %d, got %d", trailLen, len(tr))
	}
	if tr[0].x != 3 || tr[1].x != 2 {
		t.Fatalf("wrong order/contents: %+v", tr)
	}
}

// A pass occupies a slot but matches no point, so the freshest visible stone
// carries the second-newest rank.
func TestTrailRankWithPass(t *testing.T) {
	var b boardModel
	b.grid = [][]stoneColor{{black, white, black}}
	b.setTrail([]move{{x: -1, y: -1}, {x: 0, y: 0}}) // pass, then a stone
	if r := b.trailRank(0, 0); r != 1 {
		t.Fatalf("stone after a pass should be rank 1, got %d", r)
	}
	if r := b.trailRank(2, 0); r != -1 {
		t.Fatalf("untouched point should be off-trail, got %d", r)
	}
}

// Ko recapture: the newest occurrence wins.
func TestTrailRankNewestWins(t *testing.T) {
	var b boardModel
	b.grid = [][]stoneColor{{black}}
	b.setTrail([]move{{x: 0, y: 0}, {x: 0, y: 0}})
	if r := b.trailRank(0, 0); r != 0 {
		t.Fatalf("want rank 0, got %d", r)
	}
}

// capturedPoints reports stones removed between two positions, never the played point.
func TestCapturedPoints(t *testing.T) {
	prev := [][]stoneColor{{black, white}, {empty, empty}}
	cur := [][]stoneColor{{black, empty}, {black, empty}} // (1,0) captured, (0,1) played
	got := capturedPoints(prev, cur)
	if len(got) != 1 || got[0].x != 1 || got[0].y != 0 {
		t.Fatalf("want [(1,0)], got %+v", got)
	}
}

// applySnapshot marks captures in play phase and clears them at scoring.
func TestApplySnapshotCaptures(t *testing.T) {
	g := game{width: 2, height: 2, you: empty, state: boardState{
		grid: [][]stoneColor{{black, white}, {empty, empty}}, phase: phasePlay,
	}}
	gm := newGameModel(0, g, nil)
	gm.applySnapshot(boardState{
		grid: [][]stoneColor{{black, empty}, {black, empty}}, phase: phasePlay,
	})
	if !gm.board.captured(1, 0) {
		t.Fatalf("expected (1,0) marked captured")
	}
	// Scoring/finished snapshots clear the marks (removal also empties points).
	gm.applySnapshot(boardState{
		grid: [][]stoneColor{{empty, empty}, {empty, empty}}, phase: phaseFinished,
	})
	if gm.board.captured(0, 0) || len(gm.board.captures) != 0 {
		t.Fatalf("captures should be cleared outside play phase")
	}
}

// OGS move arrays decode to coordinates; pass stays a pass.
func TestOGSMoveDecode(t *testing.T) {
	var p ogsGamedataPayload
	if err := json.Unmarshal([]byte(`{"moves":[[3,4,1000],[-1,-1,0]]}`), &p); err != nil {
		t.Fatal(err)
	}
	ms := gamedataMoves(p)
	if len(ms) != 2 || ms[0].x != 3 || ms[0].y != 4 || !ms[1].isPass() {
		t.Fatalf("bad decode: %+v", ms)
	}
}

// End-to-end: a hotseat move emits a snapshot carrying the history, and applying
// it highlights the just-played stone as freshest.
func TestHotseatTrailEndToEnd(t *testing.T) {
	g := game{id: -1, width: 9, height: 9, you: empty, state: newBoardState(9, 9)}
	b := newHotseatBackend(g.state, nil)
	var last boardState
	_ = b.Connect(func(st boardState) { last = st }, nil, nil)
	gm := newGameModel(0, g, b)

	if err := b.SubmitMove(move{x: 4, y: 4, color: black}); err != nil {
		t.Fatalf("move: %v", err)
	}
	if len(last.moves) != 1 || last.moves[0].x != 4 {
		t.Fatalf("emitted snapshot should carry the move history, got %+v", last.moves)
	}
	gm.applySnapshot(last)
	if r := gm.board.trailRank(4, 4); r != 0 {
		t.Fatalf("just-played stone should be freshest (rank 0), got %d", r)
	}
}

// recordMove self-heals ordering: appends by number, replaces a replayed tail.
func TestOGSBackendRecordMove(t *testing.T) {
	b := &ogsBackend{}
	b.moves = []move{{x: 0, y: 0}}
	b.recordMove(move{x: 1, y: 1}, 2) // append
	b.recordMove(move{x: 2, y: 2}, 2) // replay of #2 → replace
	b.recordMove(move{x: 5, y: 5}, 0) // unknown number → append
	if len(b.moves) != 3 || b.moves[1].x != 2 || b.moves[2].x != 5 {
		t.Fatalf("want 3 moves [.. (2,2) (5,5)], got %+v", b.moves)
	}
}
