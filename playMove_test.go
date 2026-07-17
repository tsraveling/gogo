package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Builds a hotseat game model with a loaded 9x9 board, ready to play.
func newTestGameModel() (gameModel, *hotseatBackend, *[]boardState) {
	g := game{id: -1, width: 9, height: 9, you: empty, state: newBoardState(9, 9)}
	b := newHotseatBackend(g.state, nil)
	var snaps []boardState
	_ = b.Connect(func(st boardState) { snaps = append(snaps, st) }, nil)
	gm := newGameModel(0, g, b)
	gm.applySnapshot(g.state) // load the grid, clear "connecting"
	gm.fastMode = false       // most tests exercise the normal ghost flow
	return gm, b, &snaps
}

// Hotseat games (you == empty) default to fast mode; sided games do not.
func TestHotseatDefaultsFast(t *testing.T) {
	hs := newGameModel(0, game{width: 9, height: 9, you: empty}, nil)
	if !hs.fastMode {
		t.Fatalf("hotseat should default to fast mode")
	}
	sided := newGameModel(0, game{width: 9, height: 9, you: black}, nil)
	if sided.fastMode {
		t.Fatalf("sided game should not default to fast mode")
	}
}

// Ghost placement: space places, relocates, and toggles off.
func TestGhostToggle(t *testing.T) {
	gm, _, _ := newTestGameModel()

	gm.board.SetCursor(2, 2)
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !gm.board.ghostActive || gm.board.ghostX != 2 || gm.board.ghostY != 2 {
		t.Fatalf("space should ghost the cursor point")
	}
	// Space again on the same point clears it.
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeySpace})
	if gm.board.ghostActive {
		t.Fatalf("space on the ghost cell should clear it")
	}
	// Relocate: ghost one point, move, ghost again.
	gm.board.SetCursor(2, 2)
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeySpace})
	gm.board.SetCursor(3, 3)
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !gm.board.ghostActive || gm.board.ghostX != 3 {
		t.Fatalf("space at a new point should relocate the ghost")
	}
}

// Commit path: enter sets committing, the backend applies + emits, and the
// result clears the ghost.
func TestCommitMove(t *testing.T) {
	gm, b, snaps := newTestGameModel()

	gm.board.SetCursor(2, 2)
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeySpace})
	gm, cmd := gm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !gm.committing || cmd == nil {
		t.Fatalf("enter should start committing")
	}

	// The submission runs off the UI loop; drive it directly (avoids the batch).
	res := submitMoveCmd(b, gm.game.id, move{x: 2, y: 2, color: black})().(moveResultMsg)
	if res.err != nil {
		t.Fatalf("hotseat move should succeed: %v", res.err)
	}
	gm.applyMoveResult(res.err)
	if gm.committing || gm.board.ghostActive {
		t.Fatalf("success should clear committing and the ghost")
	}
	last := (*snaps)[len(*snaps)-1]
	if last.grid[2][2] != black || last.playerToMove != white {
		t.Fatalf("backend did not apply the move / flip turn")
	}
}

// A rejected move keeps the ghost and surfaces the reason.
func TestCommitRejectKeepsGhost(t *testing.T) {
	gm, b, _ := newTestGameModel()
	// Occupy (2,2) as black.
	if err := b.SubmitMove(move{x: 2, y: 2, color: black}); err != nil {
		t.Fatalf("setup move: %v", err)
	}
	gm.applySnapshot(b.state)

	gm.board.SetGhost(2, 2, white)
	res := submitMoveCmd(b, gm.game.id, move{x: 2, y: 2, color: white})().(moveResultMsg)
	if res.err != errOccupied {
		t.Fatalf("want errOccupied, got %v", res.err)
	}
	gm.applyMoveResult(res.err)
	if !gm.board.ghostActive {
		t.Fatalf("rejected move should keep the ghost for retry")
	}
	if gm.moveErr == "" {
		t.Fatalf("rejected move should surface an error message")
	}
}

// Two passes finish the game: board goes read-only, control area says so, and
// closing the finished hotseat tab deletes it from disk.
func TestTwoPassGameOverAndDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	g, err := newLocalGame(size9, rulesetJapanese, setupDefaultKomi, "B", "W")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(model)
	next, _ = m.Update(openGameMsg{game: g})
	m = next.(model)
	if len(m.games) != 1 {
		t.Fatalf("game should be open, got %d tabs", len(m.games))
	}

	// Drive two passes through the tab's backend, then load the finished state.
	hb := m.games[0].backend.(*hotseatBackend)
	if err := hb.SubmitMove(move{x: -1, y: -1, color: black}); err != nil {
		t.Fatalf("black pass: %v", err)
	}
	if err := hb.SubmitMove(move{x: -1, y: -1, color: white}); err != nil {
		t.Fatalf("white pass: %v", err)
	}
	m.games[0].applySnapshot(hb.state)

	if m.games[0].canPlay() {
		t.Fatalf("finished board should be read-only")
	}
	if !strings.Contains(m.games[0].controlView(30), "Game over") {
		t.Fatalf("control area should announce game over")
	}

	// X closes the finished tab → deleted from the store.
	m.closeTab(0)
	if len(m.games) != 0 {
		t.Fatalf("tab should be closed")
	}
	games, _ := loadLocalGames()
	if len(games) != 0 {
		t.Fatalf("finished local game should be deleted, got %d", len(games))
	}
}

// In a sided (OGS) game you can only place/play on your turn; hotseat is always live.
func TestTurnGating(t *testing.T) {
	// Sided game, opponent (white) to move.
	g := game{id: 1, width: 9, height: 9, you: black,
		state: boardState{grid: newBoardState(9, 9).grid, playerToMove: white, phase: phasePlay}}
	gm := newGameModel(0, g, &ogsBackend{})
	gm.applySnapshot(g.state)

	if gm.myTurn() || gm.canPlay() {
		t.Fatalf("should not be able to play on opponent's turn")
	}
	gm.board.SetCursor(2, 2)
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeySpace})
	if gm.board.ghostActive {
		t.Fatalf("space must not place a ghost when it's not your turn")
	}

	// Your turn now: placement works.
	st := g.state
	st.playerToMove = black
	gm.applySnapshot(st)
	if !gm.canPlay() {
		t.Fatalf("should be able to play on your turn")
	}
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !gm.board.ghostActive {
		t.Fatalf("space should place a ghost on your turn")
	}

	// Hotseat (you == empty) is always your turn.
	hs := newGameModel(0, game{width: 9, height: 9, you: empty, state: newBoardState(9, 9)}, nil)
	if !hs.myTurn() {
		t.Fatalf("hotseat should always be your turn")
	}
}

// Fast mode: m toggles it; space then plays immediately without a ghost.
func TestFastMode(t *testing.T) {
	gm, _, _ := newTestGameModel()

	// m toggles fast mode on and drops any pending ghost.
	gm.board.SetGhost(1, 1, black)
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !gm.fastMode || gm.board.ghostActive {
		t.Fatalf("m should enable fast mode and clear the ghost")
	}

	// Space plays straight away (committing) with no ghost step.
	gm.board.SetCursor(4, 4)
	gm, cmd := gm.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !gm.committing || cmd == nil {
		t.Fatalf("fast-mode space should commit immediately")
	}
	if gm.board.ghostActive {
		t.Fatalf("fast-mode space should not leave a ghost")
	}

	// m again toggles back off.
	gm, _, _ = newTestGameModel()
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if gm.fastMode {
		t.Fatalf("second m should disable fast mode")
	}
}

// Pass flow: p opens the confirm box (capturing input), enter submits a pass.
func TestPassConfirm(t *testing.T) {
	gm, b, _ := newTestGameModel()

	gm, _ = gm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !gm.passConfirm || !gm.capturingInput() {
		t.Fatalf("p should open a capturing pass-confirm box")
	}
	gm, cmd := gm.updatePassConfirm(tea.KeyMsg{Type: tea.KeyEnter})
	if gm.passConfirm || !gm.committing || cmd == nil {
		t.Fatalf("enter should confirm the pass and start committing")
	}

	res := submitMoveCmd(b, gm.game.id, move{x: -1, y: -1, color: black})().(moveResultMsg)
	if res.err != nil {
		t.Fatalf("pass should be accepted: %v", res.err)
	}
	if !b.state.lastMove.isPass() {
		t.Fatalf("backend should record a pass")
	}

	// Esc cancels instead of passing.
	gm2, _, _ := newTestGameModel()
	gm2, _ = gm2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	gm2, _ = gm2.updatePassConfirm(tea.KeyMsg{Type: tea.KeyEsc})
	if gm2.passConfirm || gm2.committing {
		t.Fatalf("esc should cancel the pass")
	}
}
