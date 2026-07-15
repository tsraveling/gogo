package main

import (
	"errors"

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
	// Releases resources (socket, subprocess). Safe on a nil backend.
	Disconnect()
}

// Move submission not yet wired for this backend (see _spec/playing.md).
var errSubmitUnsupported = errors.New("move submission not supported yet")

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
}

func (b *ogsBackend) Connect(emit func(boardState)) error {
	chatAuth, _ := fetchChatAuth(b.ogs.AccessToken) // best-effort; empty = unauthenticated read
	auth := socketAuth{playerID: b.ogs.UserID, username: b.ogs.Username, chatAuth: chatAuth}
	onChange := func() {
		st, err := fetchBoardState(b.ogs.AccessToken, b.gameID, b.whiteID)
		if err != nil {
			return
		}
		emit(st)
	}
	s, err := connectGame(b.gameID, auth, onChange)
	if err != nil {
		return err
	}
	b.socket = s
	return nil
}

func (b *ogsBackend) SubmitMove(move) error { return errSubmitUnsupported }

func (b *ogsBackend) Disconnect() { b.socket.Disconnect() }
