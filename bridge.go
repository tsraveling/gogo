package main

// @region ogs:bridge

// Bridges the callback-based OGS socket (ogsSocket.go) to Bubble Tea's message
// loop using the listen pattern: the socket goroutine pushes snapshots onto a
// shared channel, a blocking cmd surfaces them as tea.Msgs, and Update
// re-issues that cmd to keep listening. See docs/NETCODE.md.

import tea "github.com/charmbracelet/bubbletea"

// A board snapshot for one game, delivered from the socket goroutine. Routed to
// the matching gameModel by gameID.
type gameEvent struct {
	gameID int64
	state  boardState
}

// Blocks on the shared channel, surfacing the next snapshot as a Msg. Update
// re-issues it after each event so the listen loop continues.
func waitForGameEvent(ch <-chan gameEvent) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}
