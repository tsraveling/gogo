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
	id   int64
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

// @region game:chat

// Game chat channel, matching OGS send-side types. main = discussion (both
// players + spectators); malkovich = hidden from the opponent during play,
// visible to spectators; personal = private notes only the author sees.
type chatChannel string

const (
	chatMain      chatChannel = "main"
	chatMalkovich chatChannel = "malkovich"
	chatPersonal  chatChannel = "personal"
)

// Channels the composer cycles through (tab), in order.
var chatModes = []chatChannel{chatMain, chatMalkovich, chatPersonal}

// A single chat line, source-agnostic. id is the backend's dedup key (empty
// while an outgoing message is still optimistic, awaiting the server echo).
// isVariation marks a typed analysis/review body (rendered as a marker only —
// replaying the variation on the board is future work).
type chatMessage struct {
	id          string
	playerID    int64
	username    string
	body        string
	channel     chatChannel
	moveNumber  int
	date        int64 // unix seconds
	isVariation bool
	pending     bool // optimistic local echo, not yet confirmed by the server
}

// A single play. A pass has x == y == -1.
type move struct {
	x, y  int
	color stoneColor
}

func (m move) isPass() bool { return m.x == -1 && m.y == -1 }

// How many recent moves the board trail highlights (newest → oldest).
const trailLen = 2

// The grid plus turn/phase info. grid is indexed [y][x], matching OGS.
// moves is the full ordered history (oldest first); the renderer highlights
// its tail (see boardModel trail).
type boardState struct {
	grid         [][]stoneColor
	moveNumber   int
	playerToMove stoneColor
	lastMove     move
	phase        gamePhase
	moves        []move
}

// trail returns the last trailLen moves, newest first, for recency highlighting.
func (b boardState) trail() []move {
	n := len(b.moves)
	out := make([]move, 0, trailLen)
	for i := n - 1; i >= 0 && len(out) < trailLen; i-- {
		out = append(out, b.moves[i])
	}
	return out
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
