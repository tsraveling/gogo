package main

import (
	"encoding/json"
	"fmt"
)

// @region ogs:moves

// Move-history decoding for the realtime socket. gamedata carries the full
// ordered move list (authoritative, on connect/scoring); the move event carries
// one appended move. Both feed game.moves, whose tail drives the board trail.
// Handicap stones live in gamedata.initial_state, not moves, so the trail never
// highlights them. Move color is left empty here — the trail renders from the
// grid, and step-back can recover color by replay.

// One entry in an OGS move array: [x, y, time_ms, ...]. Pass is [-1, -1].
type ogsMovePos struct{ x, y int }

func (p *ogsMovePos) UnmarshalJSON(data []byte) error {
	var v []float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if len(v) < 2 {
		return fmt.Errorf("ogs move array too short: %s", data)
	}
	p.x, p.y = int(v[0]), int(v[1])
	return nil
}

func (p ogsMovePos) move() move { return move{x: p.x, y: p.y} }

// gamedata event payload (partial): only the move list is used here.
type ogsGamedataPayload struct {
	Moves []ogsMovePos `json:"moves"`
}

// move event payload: one appended move plus its 1-based number.
type ogsMovePayload struct {
	Move       ogsMovePos `json:"move"`
	MoveNumber int        `json:"move_number"`
}

// gamedataMoves flattens the wire move list into core moves.
func gamedataMoves(p ogsGamedataPayload) []move {
	out := make([]move, len(p.Moves))
	for i, m := range p.Moves {
		out[i] = m.move()
	}
	return out
}
