package main

// @region ogs:realtime

// OGS realtime (Socket.IO) client. Protocol and endpoint adapted from termsuji
// (~/repos/bench/termsuji: api/realtime.go, api/onlinego.go). We authenticate
// the socket (required for OGS to stream games we're a party to) and submit
// moves via game/move. gamedata is only a "state changed" trigger; the board is
// fetched separately (see ogsState.go).

import (
	"fmt"
	"sync/atomic"
	"time"

	gosocketio "github.com/graarh/golang-socketio"
	"github.com/graarh/golang-socketio/transport"
)

const realtimeURL = "wss://online-go.com/socket.io/?EIO=3&transport=websocket"

// One game's socket connection.
type gameSocket struct {
	c      *gosocketio.Client
	gameID int64
	ready  chan struct{} // closed once OnConnection has emitted authenticate + game/connect
	// Set by Disconnect before Close so the pending OnDisconnection is recognized
	// as deliberate (tab switch / close / quit) and doesn't start the reconnect
	// ladder. A real drop leaves it false.
	intentional atomic.Bool
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

// game/chat payload. type is the channel; move_number anchors the line to the
// board position it was posted at.
type emitChatMsg struct {
	GameID     int64  `json:"game_id"`
	Type       string `json:"type"`
	MoveNumber int    `json:"move_number"`
	Body       string `json:"body"`
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

// connectGame opens a socket for one game. onGamedata fires with the full move
// list on connect / scoring / finished; onMove fires with one appended move on
// every played move (ours and the opponent's). Neither event carries a rendered
// board, so the caller refetches via fetchBoardState — the move list is the only
// history OGS streams. onDrop fires on an unintentional disconnect (a real
// network drop), never on a Disconnect() close. The caller owns the returned
// socket and must Disconnect it.
func connectGame(gameID int64, auth socketAuth, onGamedata func([]move), onMove func(move, int), onChat func(chatMessage), onDrop func()) (*gameSocket, error) {
	c, err := gosocketio.Dial(realtimeURL, transport.GetDefaultWebsocketTransport())
	if err != nil {
		return nil, err
	}
	s := &gameSocket{c: c, gameID: gameID, ready: make(chan struct{})}
	// gamedata fires on connect and during scoring/finished (full move list); move
	// fires on every played move (one appended move). Only move confirms a
	// submission — gamedata also fires on connect, so acking on it would falsely
	// confirm a move that never left the socket.
	_ = c.On(fmt.Sprintf("game/%d/gamedata", gameID), func(_ any, p ogsGamedataPayload) {
		onGamedata(gamedataMoves(p))
	})
	_ = c.On(fmt.Sprintf("game/%d/move", gameID), func(_ any, p ogsMovePayload) {
		onMove(p.Move.move(), p.MoveNumber)
	})
	// game/<id>/chat fires per line; on connect (chat enabled) the whole log is
	// replayed as a burst of these. The caller dedups by chat_id.
	if onChat != nil {
		_ = c.On(fmt.Sprintf("game/%d/chat", gameID), func(_ any, p ogsChatPayload) {
			onChat(p.chatMessage())
		})
	}
	// Emit only once the socket.io connection is open — emitting before the
	// handshake completes silently loses the subscription and no data comes.
	// Closing ready lets a submit wait until authenticate + game/connect are
	// queued, so the FIFO out-queue sends our move only after we're authed.
	_ = c.On(gosocketio.OnConnection, func(_ any) {
		s.authenticate(auth)
		close(s.ready)
	})
	// A real drop (server close / network loss) fires onDrop so the caller can
	// start the reconnect ladder. A deliberate Disconnect sets intentional first,
	// so those closes are ignored here.
	_ = c.On(gosocketio.OnDisconnection, func(_ any) {
		if s.intentional.Load() {
			return
		}
		if onDrop != nil {
			onDrop()
		}
	})
	return s, nil
}

// awaitReady blocks until the socket has completed its OnConnection handshake
// (authenticate + game/connect queued) or the timeout elapses. Returns true if
// ready. On an already-connected socket it returns immediately.
func (s *gameSocket) awaitReady(timeout time.Duration) bool {
	if s == nil {
		return false
	}
	select {
	case <-s.ready:
		return true
	case <-time.After(timeout):
		return false
	}
}

// authenticate identifies us on the socket (so OGS streams games we're a party
// to and accepts our moves) and joins the game channel. The chat_auth token is
// short-lived, so this is re-emitted to refresh a lapsed session (see reauth).
func (s *gameSocket) authenticate(auth socketAuth) {
	if s == nil || s.c == nil {
		return
	}
	if auth.chatAuth != "" {
		s.c.Emit("authenticate", &emitAuth{Auth: auth.chatAuth, PlayerID: auth.playerID, Username: auth.username})
	}
	s.c.Emit("game/connect", &emitGameConnect{GameID: s.gameID, PlayerID: auth.playerID, Chat: true})
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

// sendChat emits a chat line. The server broadcasts it back (including to us) as
// a game/<id>/chat event carrying the authoritative chat_id.
func (s *gameSocket) sendChat(m chatMessage) error {
	if s == nil || s.c == nil {
		return errNoSocket
	}
	s.c.Emit("game/chat", &emitChatMsg{
		GameID:     s.gameID,
		Type:       string(m.channel),
		MoveNumber: m.moveNumber,
		Body:       m.body,
	})
	return nil
}

// isAlive reports whether the underlying socket.io connection is still open. A
// socket dropped while idle (server close / network drop) reports false; the
// caller must redial rather than re-emit, as a dead socket silently drops emits.
func (s *gameSocket) isAlive() bool {
	return s != nil && s.c != nil && s.c.IsAlive()
}

// Disconnect closes the underlying websocket, flagging the close as deliberate so
// the resulting OnDisconnection doesn't trip the reconnect ladder. Safe on a nil
// socket.
func (s *gameSocket) Disconnect() {
	if s != nil && s.c != nil {
		s.intentional.Store(true)
		s.c.Close()
	}
}
