package main

import "fmt"

// @region ogs:state

// Fetches the computed board from OGS. The realtime gamedata event carries only
// moves + initial state, not a rendered board, so the current position comes
// from termination-api/game/<id>/state (as termsuji does). This endpoint
// returns board as [y][x] ints (0/1/2 = empty/black/white).

const termBaseURL = ogsBaseURL + "/termination-api"

type ogsBoardWire struct {
	MoveNumber   int     `json:"move_number"`
	PlayerToMove int64   `json:"player_to_move"`
	Phase        string  `json:"phase"`
	Board        [][]int `json:"board"`
	LastMove     struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"last_move"`
}

// Returns the current board state. whiteID maps player_to_move (a player id) to
// a side; anything else is treated as black.
func fetchBoardState(accessToken string, gameID, whiteID int64) (boardState, error) {
	url := fmt.Sprintf("%s/game/%d/state", termBaseURL, gameID)
	var w ogsBoardWire
	if err := authGet(url, accessToken, &w); err != nil {
		return boardState{}, err
	}
	grid := make([][]stoneColor, len(w.Board))
	for y, row := range w.Board {
		grid[y] = make([]stoneColor, len(row))
		for x, v := range row {
			grid[y][x] = stoneColor(v)
		}
	}
	toMove := black
	if w.PlayerToMove == whiteID {
		toMove = white
	}
	return boardState{
		grid:         grid,
		moveNumber:   w.MoveNumber,
		playerToMove: toMove,
		lastMove:     move{x: w.LastMove.X, y: w.LastMove.Y},
		phase:        gamePhase(w.Phase),
	}, nil
}
