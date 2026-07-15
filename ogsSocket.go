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

// OGS gamedata event: a full board snapshot. Board is [y][x] with 0/1/2 =
// empty/black/white — the same encoding as our stoneColor.
type ogsGameData struct {
	MoveNumber   int     `json:"move_number"`
	PlayerToMove int64   `json:"player_to_move"`
	Phase        string  `json:"phase"`
	Board        [][]int `json:"board"`
	Handicap     int     `json:"handicap"`
	Komi         float64 `json:"komi"`
	Players      struct {
		Black struct {
			ID int64 `json:"id"`
		} `json:"black"`
		White struct {
			ID int64 `json:"id"`
		} `json:"white"`
	} `json:"players"`
}

// Converts the wire snapshot to the core boardState. playerToMove resolves the
// mover's id against the gamedata players; unknown ids fall back to black.
func (d ogsGameData) toBoardState() boardState {
	grid := make([][]stoneColor, len(d.Board))
	for y, row := range d.Board {
		grid[y] = make([]stoneColor, len(row))
		for x, v := range row {
			grid[y][x] = stoneColor(v)
		}
	}
	toMove := black
	if d.PlayerToMove == d.Players.White.ID {
		toMove = white
	}
	return boardState{
		grid:         grid,
		moveNumber:   d.MoveNumber,
		playerToMove: toMove,
		phase:        gamePhase(d.Phase),
	}
}

// connectGame opens a socket for one game and calls onData on every gamedata
// snapshot (fires on connect, then during scoring/finished). The caller owns
// the returned socket and must Disconnect it.
func connectGame(gameID, playerID int64, onData func(boardState)) (*gameSocket, error) {
	c, err := gosocketio.Dial(realtimeURL, transport.GetDefaultWebsocketTransport())
	if err != nil {
		return nil, err
	}
	c.On(fmt.Sprintf("game/%d/gamedata", gameID), func(_ any, d ogsGameData) {
		onData(d.toBoardState())
	})
	c.Emit("game/connect", &emitGameConnect{GameID: gameID, PlayerID: playerID, Chat: false})
	return &gameSocket{c: c, gameID: gameID}, nil
}

// Disconnect closes the underlying websocket. Safe on a nil socket.
func (s *gameSocket) Disconnect() {
	if s != nil && s.c != nil {
		s.c.Close()
	}
}
