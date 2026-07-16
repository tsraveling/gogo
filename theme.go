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
}

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
	}
}

// Active board theme. Swap-point for a future theme selector.
var currentTheme = classicTheme()
