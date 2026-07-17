package main

import "testing"

// isAlive: local backends are always alive; an OGS backend with no socket is not.
func TestBackendIsAlive(t *testing.T) {
	if !(&hotseatBackend{}).isAlive() {
		t.Errorf("hotseat should always be alive")
	}
	if (&ogsBackend{}).isAlive() {
		t.Errorf("ogs backend with nil socket should not be alive")
	}
}

// playBlockReason gates play while reconnecting (yellow) or given-up (red).
func TestPlayBlockReason(t *testing.T) {
	if r := (gameModel{}).playBlockReason(); r != "" {
		t.Errorf("healthy game should not block play, got %q", r)
	}
	if r := (gameModel{reconnecting: true}).playBlockReason(); r != "Reconnecting…" {
		t.Errorf("reconnecting should block with Reconnecting…, got %q", r)
	}
	if r := (gameModel{disconnected: true}).playBlockReason(); r != "reconnect to play" {
		t.Errorf("disconnected should block with reconnect to play, got %q", r)
	}
	// Give-up takes precedence over reconnecting if both were somehow set.
	if r := (gameModel{reconnecting: true, disconnected: true}).playBlockReason(); r != "reconnect to play" {
		t.Errorf("disconnected should win, got %q", r)
	}
}

// A snapshot means a healthy socket: it clears every reconnect state and
// invalidates pending ladder ticks by bumping the generation.
func TestApplySnapshotClearsReconnect(t *testing.T) {
	g := gameModel{
		reconnecting:  true,
		disconnected:  true,
		connecting:    true,
		connectErr:    true,
		reconnectRung: 2,
		reconnectGen:  5,
	}
	g.board = newBoardModel(9, 9)
	g.applySnapshot(newBoardState(9, 9))
	if g.reconnecting || g.disconnected || g.connecting || g.connectErr {
		t.Errorf("snapshot should clear all connect/reconnect states")
	}
	if g.reconnectRung != 0 {
		t.Errorf("snapshot should reset the rung, got %d", g.reconnectRung)
	}
	if g.reconnectGen != 6 {
		t.Errorf("snapshot should bump the generation to invalidate ticks, got %d", g.reconnectGen)
	}
}

// cancelReconnect (blur/close) stops the ladder and clears both states.
func TestCancelReconnect(t *testing.T) {
	g := gameModel{reconnecting: true, disconnected: true, reconnectRung: 3, reconnectGen: 1}
	g.cancelReconnect()
	if g.reconnecting || g.disconnected || g.reconnectRung != 0 {
		t.Errorf("cancel should clear reconnect state")
	}
	if g.reconnectGen != 2 {
		t.Errorf("cancel should bump the generation, got %d", g.reconnectGen)
	}
}

// The ladder is immediate/2s/10s/30s, then give up — four rungs.
func TestReconnectLadderShape(t *testing.T) {
	if len(reconnectDelays) != 4 {
		t.Fatalf("expected 4 rungs, got %d", len(reconnectDelays))
	}
	if reconnectDelays[0] != 0 {
		t.Errorf("first rung must be immediate")
	}
	for i := 1; i < len(reconnectDelays); i++ {
		if reconnectDelays[i] <= reconnectDelays[i-1] {
			t.Errorf("rung %d delay must exceed the previous", i)
		}
	}
}

// hiddenTurnCount counts your-turn OGS games that aren't open as tabs.
func TestHiddenTurnCount(t *testing.T) {
	yourTurn := func(id int64) game {
		return game{id: id, you: black, state: boardState{playerToMove: black}}
	}
	theirTurn := func(id int64) game {
		return game{id: id, you: black, state: boardState{playerToMove: white}}
	}
	m := model{}
	m.home.games = []game{yourTurn(1), yourTurn(2), theirTurn(3)}
	// Game 1 is open as a tab; 2 (your turn) and 3 (their turn) are not.
	m.games = []gameModel{{game: yourTurn(1)}}
	if n := m.hiddenTurnCount(); n != 1 {
		t.Errorf("expected 1 hidden your-turn game (id 2), got %d", n)
	}
}
