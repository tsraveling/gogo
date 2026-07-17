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
	// Blocking network work runs inside a tea.Cmd (connectBackendCmd). onReconnect
	// fires when a submit has to redial a dropped socket; local backends ignore it.
	Connect(emit func(boardState), onReconnect func()) error
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

// Our login could no longer be refreshed; the user must sign in again.
var errSessionExpired = errors.New("OGS session expired — press Q to log out, then sign in again")

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
		onReconnect := func() { ch <- gameEvent{gameID: gameID, reconnecting: true} }
		return backendConnectedMsg{gameID: gameID, err: b.Connect(emit, onReconnect)}
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
	emit    func(boardState) // set by Connect; reused when redialing a dead socket
	notify  func()           // fires when a submit redials a dropped socket

	mu    sync.Mutex
	ackCh chan error // set while a SubmitMove awaits the server's broadcast
}

func (b *ogsBackend) Connect(emit func(boardState), onReconnect func()) error {
	b.emit = emit
	b.notify = onReconnect
	return b.dial()
}

// dial opens (or reopens) the game socket, wiring the state-change handler. It's
// called on Connect and again to replace a socket that died while idle — the
// realtime connection has no reconnect of its own, so a dropped socket silently
// swallows emits until redialed.
func (b *ogsBackend) dial() error {
	// Refreshes the board on any state change (connect, move, scoring).
	onChange := func() {
		st, err := fetchBoardState(b.accessToken(), b.gameID, b.whiteID)
		if err != nil {
			b.signalAck(err) // unblock a pending submit even if the refetch failed
			return
		}
		b.emit(st)
	}
	// Confirms a submission — fires only on a move broadcast, so the gamedata
	// that arrives on connect can't falsely ack a move that never got sent.
	onMove := func() { b.signalAck(nil) }
	auth, _ := b.buildAuth() // best-effort; empty chat_auth = unauthenticated read
	s, err := connectGame(b.gameID, auth, onChange, onMove)
	if err != nil {
		return err
	}
	b.socket = s
	return nil
}

// accessToken reads the current token under lock; reauth may swap it from
// another goroutine after refreshing.
func (b *ogsBackend) accessToken() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ogs.AccessToken
}

// buildAuth fetches the short-lived chat_auth token for the socket. A non-nil
// error (e.g. 401) means the access token itself is no longer valid.
func (b *ogsBackend) buildAuth() (socketAuth, error) {
	chatAuth, err := fetchChatAuth(b.accessToken())
	auth := socketAuth{playerID: b.ogs.UserID, username: b.ogs.Username, chatAuth: chatAuth}
	return auth, err
}

// Emits the move, then waits for OGS to broadcast the new state (gamedata →
// board refetch). Server is authoritative; a timeout surfaces as a reject.
//
// OGS silently ignores moves from a socket whose auth has lapsed (the chat_auth
// token is short-lived), which reads as a timeout. So on timeout we refresh the
// session and retry once before giving up.
func (b *ogsBackend) SubmitMove(m move) error {
	if err := b.submitOnce(m); err != errMoveTimeout {
		return err
	}
	if err := b.reauth(); err != nil {
		return err
	}
	return b.submitOnce(m)
}

func (b *ogsBackend) submitOnce(m move) error {
	// A socket dropped while idle silently swallows emits, so redial before
	// submitting. The redial reauthenticates in OnConnection; if the move still
	// races ahead of that, SubmitMove's timeout-and-reauth retry recovers it.
	if !b.socket.isAlive() {
		if b.notify != nil {
			b.notify() // surface "reconnecting" while the redial + handshake runs
		}
		if err := b.dial(); err != nil {
			return err
		}
	}
	// Don't emit the move until the handshake has queued authenticate +
	// game/connect ahead of it, or the server drops an unauthenticated move.
	b.socket.awaitReady(ogsMoveTimeout)

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

// Re-establishes the socket's authenticated session. If chat_auth can't be
// fetched, the access token has expired too, so refresh it (rotating the
// refresh token) and persist before retrying.
func (b *ogsBackend) reauth() error {
	auth, err := b.buildAuth()
	if err != nil {
		refreshed, rErr := authenticateRefresh(b.ogs.RefreshToken)
		if rErr != nil {
			return errSessionExpired
		}
		b.mu.Lock()
		b.ogs = refreshed
		b.mu.Unlock()
		_ = refreshed.save() // disk is the source of truth for next launch
		if auth, err = b.buildAuth(); err != nil {
			return errSessionExpired
		}
	}
	b.socket.authenticate(auth)
	return nil
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
