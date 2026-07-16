package main

import "testing"

// Full persistence loop: create → play through the backend → reload from disk,
// confirming the position, ko reference, and move list all survive.
func TestLocalGamePersistence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	g, err := newLocalGame(size9, rulesetJapanese, 6.5, "Black", "White")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.id >= 0 {
		t.Fatalf("local id should be negative, got %d", g.id)
	}

	// Reopen the game and play a move through its persisting backend.
	g2, b, ok := openLocalGame(g.id)
	if !ok {
		t.Fatalf("openLocalGame: not found")
	}
	if err := b.Connect(func(boardState) {}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := b.SubmitMove(move{x: 2, y: 2, color: black}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	_ = g2

	// A second local game claims the next negative id.
	g3, err := newLocalGame(size13, rulesetChinese, 7.5, "B", "W")
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	if g3.id != g.id-1 {
		t.Fatalf("want id %d, got %d", g.id-1, g3.id)
	}

	// Reload from disk: the committed move and turn flip must be there.
	reloaded, _, ok := openLocalGame(g.id)
	if !ok {
		t.Fatalf("reload: not found")
	}
	if reloaded.state.grid[2][2] != black {
		t.Fatalf("stone not persisted: %v", reloaded.state.grid[2][2])
	}
	if reloaded.state.playerToMove != white {
		t.Fatalf("turn not persisted: %v", reloaded.state.playerToMove)
	}

	store, err := loadLocalStore()
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	var rec *localGameRecord
	for i := range store.Games {
		if store.Games[i].ID == g.id {
			rec = &store.Games[i]
		}
	}
	if rec == nil {
		t.Fatalf("record missing")
	}
	if len(rec.Moves) != 1 || rec.Moves[0].X != 2 {
		t.Fatalf("move list not persisted: %+v", rec.Moves)
	}
	if rec.PrevGrid == nil {
		t.Fatalf("ko reference (prev grid) not persisted")
	}
}
