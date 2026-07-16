package main

// @region ogs:realtime

// OGS realtime (Socket.IO) client. Protocol and endpoint adapted from termsuji
// (~/repos/bench/termsuji: api/realtime.go, api/onlinego.go). We authenticate
// the socket (required for OGS to stream games we're a party to) and submit
// moves via game/move. gamedata is only a "state changed" trigger; the board is
// fetched separately (see ogsState.go).

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

// game/move payload. Move is SGF-encoded (see sgfCoord).
type emitMove struct {
	GameID   int64  `json:"game_id"`
	PlayerID int64  `json:"player_id"`
	Move     string `json:"move"`
}

// SGF coordinate for a move: two letters, "aa" = top-left, x then y. Pass is
// ".." (OGS convention). Boards up to 26 wide (lowercase only), matching termsuji.
func sgfCoord(m move) string {
	if m.isPass() {
		return ".."
	}
	return string(rune('a'+m.x)) + string(rune('a'+m.y))
}

// Connection credentials for the game socket.
type socketAuth struct {
	playerID int64
	username string
	chatAuth string
}

// connectGame opens a socket for one game and calls onChange whenever a gamedata
// (connect / scoring / finished) or move event fires. Neither carries a rendered
// board, so they serve purely as "state changed" triggers — the caller fetches
// the board via fetchBoardState. The caller owns the returned socket and must
// Disconnect it.
func connectGame(gameID int64, auth socketAuth, onChange func()) (*gameSocket, error) {
	c, err := gosocketio.Dial(realtimeURL, transport.GetDefaultWebsocketTransport())
	if err != nil {
		return nil, err
	}
	// gamedata fires on connect and during scoring/finished; move fires on every
	// played move (ours and the opponent's). Both are only "state changed"
	// triggers — the board is refetched in onChange.
	_ = c.On(fmt.Sprintf("game/%d/gamedata", gameID), func(_ any, _ map[string]any) {
		onChange()
	})
	_ = c.On(fmt.Sprintf("game/%d/move", gameID), func(_ any, _ map[string]any) {
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

// submitMove emits a move over the game socket. The authoritative result comes
// back as a gamedata event (→ board refetch), not a return value.
func (s *gameSocket) submitMove(playerID int64, m move) error {
	if s == nil || s.c == nil {
		return errNoSocket
	}
	s.c.Emit("game/move", &emitMove{GameID: s.gameID, PlayerID: playerID, Move: sgfCoord(m)})
	return nil
}

// Disconnect closes the underlying websocket. Safe on a nil socket.
func (s *gameSocket) Disconnect() {
	if s != nil && s.c != nil {
		s.c.Close()
	}
}
