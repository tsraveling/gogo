package main

// @region ogs:realtime

// OGS realtime (Socket.IO) client. Protocol, endpoint, and the gamedata board
// encoding are adapted from termsuji (~/repos/bench/termsuji: api/realtime.go,
// api/onlinego.go). Read-only for now: we subscribe to state but never
// authenticate or submit moves.

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

// Parses the gamedata event into a core boardState. gamedata is decoded as a
// generic map (not a typed struct): the object carries fields whose types vary
// between games, and this socket lib silently drops any event that fails to
// unmarshal into a fixed struct. Board is [y][x] with 0/1/2 = empty/black/white
// — the same encoding as our stoneColor. playerToMove resolves the mover's id
// against the gamedata players; unknown ids fall back to black.
func parseGameData(d map[string]any) boardState {
	var grid [][]stoneColor
	if rows, ok := d["board"].([]any); ok {
		grid = make([][]stoneColor, len(rows))
		for y, r := range rows {
			cells, _ := r.([]any)
			grid[y] = make([]stoneColor, len(cells))
			for x, v := range cells {
				grid[y][x] = stoneColor(toInt(v))
			}
		}
	}

	toMove := black
	if pm := toInt64(d["player_to_move"]); pm != 0 {
		if players, ok := d["players"].(map[string]any); ok {
			if w, ok := players["white"].(map[string]any); ok && toInt64(w["id"]) == pm {
				toMove = white
			}
		}
	}

	phase, _ := d["phase"].(string)
	return boardState{
		grid:         grid,
		moveNumber:   toInt(d["move_number"]),
		playerToMove: toMove,
		phase:        gamePhase(phase),
	}
}

// JSON numbers decode as float64 through a generic map.
func toInt(v any) int {
	f, _ := v.(float64)
	return int(f)
}

func toInt64(v any) int64 {
	f, _ := v.(float64)
	return int64(f)
}

// connectGame opens a socket for one game and calls onData on every gamedata
// snapshot (fires on connect, then during scoring/finished). The caller owns
// the returned socket and must Disconnect it.
func connectGame(gameID, playerID int64, onData func(boardState)) (*gameSocket, error) {
	debugf("connectGame: dialing game=%d player=%d", gameID, playerID)
	c, err := gosocketio.Dial(realtimeURL, transport.GetDefaultWebsocketTransport())
	if err != nil {
		debugf("connectGame: dial error: %v", err)
		return nil, err
	}
	_ = c.On(fmt.Sprintf("game/%d/gamedata", gameID), func(_ any, d map[string]any) {
		bs := parseGameData(d)
		debugf("gamedata game=%d move=%d phase=%s dims=%dx%d", gameID, bs.moveNumber, bs.phase, bs.width(), bs.height())
		onData(bs)
	})
	// Subscribe only once the socket.io connection is open — emitting before the
	// handshake completes silently loses the subscription and no gamedata comes.
	_ = c.On(gosocketio.OnConnection, func(_ any) {
		c.Emit("game/connect", &emitGameConnect{GameID: gameID, PlayerID: playerID, Chat: false})
		debugf("socket connected game=%d, emitted game/connect", gameID)
	})
	_ = c.On(gosocketio.OnDisconnection, func(_ any) { debugf("socket disconnected game=%d", gameID) })
	_ = c.On(gosocketio.OnError, func(_ any) { debugf("socket error game=%d", gameID) })
	return &gameSocket{c: c, gameID: gameID}, nil
}

// Disconnect closes the underlying websocket. Safe on a nil socket.
func (s *gameSocket) Disconnect() {
	if s != nil && s.c != nil {
		s.c.Close()
	}
}
