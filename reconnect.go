package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// @region ogs:reconnect

// Reconnect ladder: after a real drop on the focused OGS game we retry
// immediately, then after 2s, 10s, 30s, then give up (red state). Timing lives on
// the tea loop — each failed attempt schedules the next rung. A rung's delay is
// the wait *before* that attempt (rung 0 is immediate). Manual retries (rung -1)
// run a single attempt and never schedule a successor.
var reconnectDelays = []time.Duration{0, 2 * time.Second, 10 * time.Second, 30 * time.Second}

// Fires when it's time to run the reconnect attempt at rung. gen guards against
// stale ticks from a canceled or superseded ladder.
type reconnectTickMsg struct {
	gameID int64
	rung   int
	gen    int
}

// Reports a reconnect attempt's outcome, routed by gameID. err nil = success
// (ready + first snapshot). rung < 0 marks a manual single attempt.
type reconnectResultMsg struct {
	gameID int64
	rung   int
	gen    int
	err    error
}

// Schedules the attempt at rung after its delay (immediate for rung 0).
func scheduleReconnect(gameID int64, rung, gen int) tea.Cmd {
	msg := reconnectTickMsg{gameID: gameID, rung: rung, gen: gen}
	if reconnectDelays[rung] <= 0 {
		return func() tea.Msg { return msg }
	}
	return tea.Tick(reconnectDelays[rung], func(time.Time) tea.Msg { return msg })
}

// Runs one reconnect attempt off the UI goroutine. Local backends never drop, so
// this is effectively OGS-only; a non-OGS backend reports instant success.
func reconnectAttemptCmd(b backend, gameID int64, rung, gen int) tea.Cmd {
	return func() tea.Msg {
		ob, ok := b.(*ogsBackend)
		if !ok {
			return reconnectResultMsg{gameID: gameID, rung: rung, gen: gen}
		}
		return reconnectResultMsg{gameID: gameID, rung: rung, gen: gen, err: ob.reconnect()}
	}
}

// startReconnect enters the yellow reconnecting state and kicks off the ladder at
// rung 0. No-op for non-OGS backends.
func (m *model) startReconnect(gm *gameModel) tea.Cmd {
	if _, ok := gm.backend.(*ogsBackend); !ok {
		return nil
	}
	gm.reconnectGen++
	gm.reconnecting = true
	gm.disconnected = false
	gm.reconnectRung = 0
	return tea.Batch(gm.spinner.Tick, scheduleReconnect(gm.game.id, 0, gm.reconnectGen))
}

// manualReconnect runs a single attempt from the red give-up state (triggered by
// r). Failure drops back to red; success streams again.
func (m *model) manualReconnect(gm *gameModel) tea.Cmd {
	gm.reconnectGen++
	gm.reconnecting = true
	gm.disconnected = false
	gm.reconnectRung = 0
	return tea.Batch(gm.spinner.Tick, reconnectAttemptCmd(gm.backend, gm.game.id, -1, gm.reconnectGen))
}

// @region ogs:poll

// One always-on overview poll every 20s while authenticated is the "whose turn"
// signal for every game (open as a tab or not); the focused board relies on its
// own socket. Paused only while the login modal is open.
const pollInterval = 20 * time.Second

// Fires the recurring poll.
type pollTickMsg struct{}

// Result of one overview poll. authDead means the token is gone and can't be
// refreshed (→ auth-killed flow). A refreshed non-nil ogs carries rotated tokens
// the model should adopt.
type pollResultMsg struct {
	games    []game
	ogs      *ogsModel
	err      error
	authDead bool
}

func pollTickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollTickMsg{} })
}

// Polls the overview off the UI goroutine. A 401 triggers a refresh-and-retry;
// if the refresh fails the session is dead. Other errors are transient (the next
// tick retries).
func pollGamesCmd(o ogsModel) tea.Cmd {
	return func() tea.Msg {
		games, err := fetchActiveGames(o.AccessToken, o.UserID)
		if err == nil {
			return pollResultMsg{games: games}
		}
		if err != errUnauthorized {
			return pollResultMsg{err: err}
		}
		refreshed, rErr := authenticateRefresh(o.RefreshToken)
		if rErr != nil {
			return pollResultMsg{authDead: true}
		}
		_ = refreshed.save()
		games, err = fetchActiveGames(refreshed.AccessToken, refreshed.UserID)
		if err != nil {
			return pollResultMsg{ogs: &refreshed, err: err}
		}
		return pollResultMsg{games: games, ogs: &refreshed}
	}
}
