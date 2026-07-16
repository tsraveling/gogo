package main

import (
	"errors"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// @region game:backend

// A game backend abstracts where a game's authority lives: OGS (server), a local
// GnuGo subprocess, or in-memory hotseat. All push boardState snapshots through
// emit, routed to the matching gameModel via the events channel (see bridge.go).
// Lifetime is tied to a game tab: Connect on open, Disconnect on close/quit.
type backend interface {
	// Starts streaming; emits the initial state and every state change after.
	// Blocking network work runs inside a tea.Cmd (connectBackendCmd).
	Connect(emit func(boardState)) error
	// Submits a move (a pass is a move with isPass()); the resulting state
	// arrives via emit. A non-nil error means the move was rejected.
	SubmitMove(m move) error
	// Reports whether SubmitMove applies synchronously (no submit spinner/✓).
	// True for local backends (hotseat/gnugo), false for OGS.
	Instant() bool
	// Releases resources (socket, subprocess). Safe on a nil backend.
	Disconnect()
}

// Move submission not yet wired for this backend (see _spec/playing.md).
var errSubmitUnsupported = errors.New("move submission not supported yet")

// The socket isn't connected yet.
var errNoSocket = errors.New("game socket not connected")

// The server didn't confirm the move in time (likely rejected).
var errMoveTimeout = errors.New("move not confirmed by server")

// How long to wait for OGS to broadcast our move before treating it as failed.
const ogsMoveTimeout = 5 * time.Second

// Reports a backend connection attempt's result, routed by gameID.
type backendConnectedMsg struct {
	gameID int64
	err    error
}

// Dials a game's backend off the UI goroutine. emit closes over the gameID so
// snapshots land on the right tab.
func connectBackendCmd(b backend, gameID int64, ch chan<- gameEvent) tea.Cmd {
	return func() tea.Msg {
		emit := func(st boardState) { ch <- gameEvent{gameID: gameID, state: st} }
		return backendConnectedMsg{gameID: gameID, err: b.Connect(emit)}
	}
}

// @region ogs:backend

// The OGS backend: authoritative state lives on the server. The realtime socket
// signals "state changed"; the board is fetched via REST (see ogsSocket.go /
// ogsState.go). Read-only for now — SubmitMove lands with OGS move submission.
type ogsBackend struct {
	gameID  int64
	whiteID int64 // maps player_to_move to a side
	ogs     ogsModel
	socket  *gameSocket

	mu    sync.Mutex
	ackCh chan error // set while a SubmitMove awaits the server's broadcast
}

func (b *ogsBackend) Connect(emit func(boardState)) error {
	chatAuth, _ := fetchChatAuth(b.ogs.AccessToken) // best-effort; empty = unauthenticated read
	auth := socketAuth{playerID: b.ogs.UserID, username: b.ogs.Username, chatAuth: chatAuth}
	onChange := func() {
		st, err := fetchBoardState(b.ogs.AccessToken, b.gameID, b.whiteID)
		if err != nil {
			b.signalAck(err) // unblock a pending submit even if the refetch failed
			return
		}
		emit(st)
		b.signalAck(nil) // our move (or any update) landed
	}
	s, err := connectGame(b.gameID, auth, onChange)
	if err != nil {
		return err
	}
	b.socket = s
	return nil
}

// Emits the move, then waits for OGS to broadcast the new state (gamedata →
// board refetch). Server is authoritative; a timeout surfaces as a reject.
func (b *ogsBackend) SubmitMove(m move) error {
	ch := make(chan error, 1)
	b.mu.Lock()
	b.ackCh = ch
	b.mu.Unlock()

	if err := b.socket.submitMove(b.ogs.UserID, m); err != nil {
		b.clearAck()
		return err
	}
	select {
	case err := <-ch:
		return err
	case <-time.After(ogsMoveTimeout):
		b.clearAck()
		return errMoveTimeout
	}
}

// Resolves a pending SubmitMove, if any, with the given result.
func (b *ogsBackend) signalAck(err error) {
	b.mu.Lock()
	ch := b.ackCh
	b.ackCh = nil
	b.mu.Unlock()
	if ch != nil {
		ch <- err
	}
}

func (b *ogsBackend) clearAck() {
	b.mu.Lock()
	b.ackCh = nil
	b.mu.Unlock()
}

func (b *ogsBackend) Instant() bool { return false }

func (b *ogsBackend) Disconnect() { b.socket.Disconnect() }
