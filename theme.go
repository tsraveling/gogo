package main

import "github.com/charmbracelet/lipgloss"

// @region board:theme

// A board's characters and colors. Rendering goes through currentTheme rather
// than hard-coded styles so a theme selector can offer alternatives later
// (see _spec/themes.md).
type boardTheme struct {
	name string

	blackGlyph string // black stone
	whiteGlyph string // white stone
	emptyGlyph string // empty intersection
	starGlyph  string // star point (hoshi)

	black lipgloss.Style
	white lipgloss.Style

	// Uncommitted placeholder: the real stone over this tile (no separate glyph)
	// so it reads as "about to be placed here".
	placeholder lipgloss.Style

	grid   lipgloss.Style // empty + star points
	label  lipgloss.Style // coordinate numbers/letters
	cursor lipgloss.Style // the [ ] brackets

	turnGlyph string         // current-turn marker
	turn      lipgloss.Style // its color

	// Recency trail backgrounds, newest → oldest (len trailLen). A stone played
	// in the last trailLen moves sits on trail[rank]; rank 0 is the freshest.
	trail [trailLen]lipgloss.Color

	captureGlyph string         // mark on a point cleared last turn
	capture      lipgloss.Style // its color
}

// Styled glyph for a committed stone sitting on the recency trail: the trail
// background with a forced high-contrast foreground so both poles stay legible.
func (t boardTheme) trailCell(c stoneColor, rank int) string {
	fg := lipgloss.Color("232") // near-black for black stones
	g := t.blackGlyph
	if c == white {
		fg, g = lipgloss.Color("231"), t.whiteGlyph
	}
	return lipgloss.NewStyle().Bold(true).Foreground(fg).Background(t.trail[rank]).Render(g)
}

// Styled mark for a point whose stone was captured on the last move.
func (t boardTheme) captureCell() string { return t.capture.Render(t.captureGlyph) }

// Styled glyph for a committed stone.
func (t boardTheme) stoneCell(c stoneColor) string {
	if c == white {
		return t.white.Render(t.whiteGlyph)
	}
	return t.black.Render(t.blackGlyph)
}

// Styled glyph for an uncommitted placeholder: the stone over the placeholder tile.
func (t boardTheme) ghostCell(c stoneColor) string {
	st, g := t.black, t.blackGlyph
	if c == white {
		st, g = t.white, t.whiteGlyph
	}
	return st.Background(t.placeholder.GetBackground()).Render(g)
}

// "Classic": both sides filled ●, correct polarity (black darker, white bright),
// on a dim-yellow grid for the traditional go-board feel. Placeholder sits on a
// soft dark-blue tile.
func classicTheme() boardTheme {
	return boardTheme{
		name:        "classic",
		blackGlyph:  "●",
		whiteGlyph:  "●",
		emptyGlyph:  "·",
		starGlyph:   "+",
		black:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")), // mid gray
		white:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")), // bright white
		placeholder: lipgloss.NewStyle().Background(lipgloss.Color("26")),             // bright soft blue
		grid:        lipgloss.NewStyle().Foreground(lipgloss.Color("136")),            // dim yellow
		label:       lipgloss.NewStyle().Foreground(dimColor),
		cursor:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")),
		turnGlyph:   "▶",
		turn:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226")), // bright yellow
		trail: [trailLen]lipgloss.Color{
			lipgloss.Color("#009600"), // most recent move
			lipgloss.Color("#003200"), // second most recent
		},
		captureGlyph: "×",
		capture:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")), // red
	}
}

// Active board theme. Swap-point for a future theme selector.
var currentTheme = classicTheme()
