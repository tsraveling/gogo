package main

// @region ogs:realtime

// OGS realtime (Socket.IO) client. Protocol and endpoint adapted from termsuji
// (~/repos/bench/termsuji: api/realtime.go, api/onlinego.go). We authenticate
// the socket (required for OGS to stream games we're a party to) but stay
// read-only — no move submission. gamedata is only a "state changed" trigger;
// the board is fetched separately (see ogsState.go).

import (
	"fmt"

	gosocketio "github.com/graarh/golang-socketio"
	"github.com/graarh/golang-socketio/transport"
)

const realtimeURL = "wss://online-go.com/socket.io/?EIO=3&transport=websocket"

// One game's socket connection.
type gameSocket struct {
	c      *gosocketio.Client
	gameID int64
}

// game/connect subscribe payload.
type emitGameConnect struct {
	GameID   int64 `json:"game_id"`
	PlayerID int64 `json:"player_id"`
	Chat     bool  `json:"chat"`
}

// authenticate payload: identifies us on the socket so OGS streams our games.
type emitAuth struct {
	Auth     string `json:"auth"`
	PlayerID int64  `json:"player_id"`
	Username string `json:"username"`
}

// Connection credentials for the game socket.
type socketAuth struct {
	playerID int64
	username string
	chatAuth string
}

// connectGame opens a socket for one game and calls onChange whenever a
// gamedata event fires (on connect, then during scoring/finished). gamedata
// carries only moves + initial state, not a rendered board, so it serves purely
// as a "state changed" trigger — the caller fetches the board via fetchBoardState.
// The caller owns the returned socket and must Disconnect it.
func connectGame(gameID int64, auth socketAuth, onChange func()) (*gameSocket, error) {
	c, err := gosocketio.Dial(realtimeURL, transport.GetDefaultWebsocketTransport())
	if err != nil {
		return nil, err
	}
	_ = c.On(fmt.Sprintf("game/%d/gamedata", gameID), func(_ any, _ map[string]any) {
		onChange()
	})
	// Emit only once the socket.io connection is open — emitting before the
	// handshake completes silently loses the subscription and no data comes.
	// authenticate first so OGS streams the games we're a party to.
	_ = c.On(gosocketio.OnConnection, func(_ any) {
		if auth.chatAuth != "" {
			c.Emit("authenticate", &emitAuth{Auth: auth.chatAuth, PlayerID: auth.playerID, Username: auth.username})
		}
		c.Emit("game/connect", &emitGameConnect{GameID: gameID, PlayerID: auth.playerID, Chat: false})
	})
	return &gameSocket{c: c, gameID: gameID}, nil
}

// Disconnect closes the underlying websocket. Safe on a nil socket.
func (s *gameSocket) Disconnect() {
	if s != nil && s.c != nil {
		s.c.Close()
	}
}
