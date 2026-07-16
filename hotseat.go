package main

// @region game:hotseat

// In-memory two-human backend: we are the authority, no network. State lives here
// so it survives tab blur. Moves are validated by rules.go; the resulting position
// is pushed straight back through emit. `you` on the game is empty — either side is
// local — so playerToMove simply alternates.
//
// Persistence (save after each move) is layered on in the local-persistence section
// via the onCommit hook; nil means don't persist.
type hotseatBackend struct {
	state    boardState
	prevGrid [][]stoneColor // position one ply back, for ko
	moves    []move         // committed moves, in order (persisted)
	emit     func(boardState)
	onCommit func(*hotseatBackend) // persist hook; nil = no-op
}

func newHotseatBackend(st boardState, prevGrid [][]stoneColor) *hotseatBackend {
	return &hotseatBackend{state: st, prevGrid: prevGrid}
}

func (b *hotseatBackend) Connect(emit func(boardState)) error {
	b.emit = emit
	emit(b.state) // surface the starting position
	return nil
}

// Validates and applies m locally, flips the turn, and emits the new state. A
// rejected move (illegal/occupied/suicide/ko) leaves state untouched and returns
// the error for the UI to surface.
func (b *hotseatBackend) SubmitMove(m move) error {
	next, err := applyMove(b.state, b.prevGrid, m)
	if err != nil {
		return err
	}
	b.prevGrid = b.state.grid // pre-move position becomes the ko reference
	b.state = next
	b.moves = append(b.moves, m)
	if b.emit != nil {
		b.emit(next)
	}
	if b.onCommit != nil {
		b.onCommit(b)
	}
	return nil
}

func (b *hotseatBackend) Instant() bool { return true }

// Nothing to tear down — state is in memory and persisted on commit.
func (b *hotseatBackend) Disconnect() {}
