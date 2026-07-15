package main

import "fmt"

// @region game:model

// Core game models, source-agnostic (OGS, gnugo, hotseat). Field semantics
// copied from OGS; see termsuji api/onlinego.go for the wire shapes.

// Scoring/rules variant, matching OGS ruleset identifiers.
type ruleset string

const (
	rulesetJapanese   ruleset = "japanese"
	rulesetChinese    ruleset = "chinese"
	rulesetKorean     ruleset = "korean"
	rulesetAGA        ruleset = "aga"
	rulesetIng        ruleset = "ing"
	rulesetNewZealand ruleset = "nz"
)

// Preset square side length for local games. OGS games carry arbitrary
// width/height (rectangular, 1–25) on game directly.
type boardSize int

const (
	size9  boardSize = 9
	size13 boardSize = 13
	size19 boardSize = 19
)

// Width and height for a square preset.
func (s boardSize) dims() (int, int) { return int(s), int(s) }

// Cell/stone value, matching OGS board encoding (0/1/2).
type stoneColor int

const (
	empty stoneColor = iota
	black
	white
)

// OGS game phase.
type gamePhase string

const (
	phasePlay     gamePhase = "play"
	phaseRemoval  gamePhase = "stone removal"
	phaseFinished gamePhase = "finished"
)

// A game participant. rank is the raw OGS ranking; use rankString for display.
type player struct {
	name string
	rank float64
}

// Formats the raw ranking as kyu/dan, matching OGS.
func (p player) rankString() string {
	if p.rank < 30 {
		return fmt.Sprintf("%d kyu", int(30-p.rank+0.5))
	}
	return fmt.Sprintf("%d dan", int(p.rank-30+0.5)+1)
}

// A single play. A pass has x == y == -1.
type move struct {
	x, y  int
	color stoneColor
}

func (m move) isPass() bool { return m.x == -1 && m.y == -1 }

// The grid plus turn/phase info. grid is indexed [y][x], matching OGS.
type boardState struct {
	grid         [][]stoneColor
	moveNumber   int
	playerToMove stoneColor
	lastMove     move
	phase        gamePhase
}

func (b boardState) height() int { return len(b.grid) }

func (b boardState) width() int {
	if len(b.grid) == 0 {
		return 0
	}
	return len(b.grid[0])
}

func (b boardState) finished() bool { return b.phase == phaseFinished }

// The core game: metadata, both players, and current board state.
// width/height hold any OGS-supported dimensions; local games set them
// from a boardSize preset via dims().
type game struct {
	id       int64 // OGS game id; 0 for local games
	name     string
	ruleset  ruleset
	width    int
	height   int
	komi     float64
	handicap int
	you      stoneColor // side the local user plays; empty for none (e.g. hotseat)
	black    player
	white    player
	state    boardState
}

// Reports whether it is the local user's move.
func (g game) yourTurn() bool {
	return g.you != empty && g.you == g.state.playerToMove
}
