package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// @region board:stone-size

// stoneShape is how a side's stone renders. Filled/hollow are circles that pick
// their glyph from the global stone size; shapeLiteral is a size-independent
// character (e.g. GnuGo's X/O).
type stoneShape int

const (
	shapeFilled stoneShape = iota
	shapeHollow
	shapeLiteral
)

// stoneSize scales the circle stones. Regular (default) is single-width-safe on
// nearly every terminal; large uses ⬤/◯, which some fonts render double-width
// and overflow the grid.
type stoneSize int

const (
	stoneRegular stoneSize = iota
	stoneLarge
)

// Circle glyphs by fill and size.
const (
	filledRegular = "●"
	filledLarge   = "⬤"
	hollowRegular = "○"
	hollowLarge   = "◯"
)

// Active stone size, shared by every theme.
var currentStoneSize = stoneRegular

// Resolves a shape to its glyph under the current stone size; literal shapes
// return their verbatim glyph unchanged.
func stoneGlyph(shape stoneShape, literal string) string {
	switch shape {
	case shapeFilled:
		if currentStoneSize == stoneLarge {
			return filledLarge
		}
		return filledRegular
	case shapeHollow:
		if currentStoneSize == stoneLarge {
			return hollowLarge
		}
		return hollowRegular
	default:
		return literal
	}
}

// Human-readable label for the current stone size (flash + persisted pref).
func stoneSizeName() string {
	if currentStoneSize == stoneLarge {
		return "Large"
	}
	return "Regular"
}

// Toggles between regular and large stones; returns the new size's name.
func toggleStoneSize() string {
	if currentStoneSize == stoneLarge {
		currentStoneSize = stoneRegular
	} else {
		currentStoneSize = stoneLarge
	}
	return stoneSizeName()
}

// Restores stone size from a persisted name (no-op on unknown; defaults regular).
func setStoneSizeByName(name string) {
	if name == "Large" {
		currentStoneSize = stoneLarge
		return
	}
	currentStoneSize = stoneRegular
}

// @region board:theme

// A board's characters and colors. Rendering goes through currentTheme rather
// than hard-coded styles so a theme selector can offer alternatives later
// (see _spec/themes.md).
type boardTheme struct {
	name string

	// Board-area background; empty means the terminal default. When set, the
	// grid gaps and margins are painted with it (see boardTheme.space).
	bg lipgloss.Color

	// How each side's stone is drawn. Circle shapes (filled/hollow) resolve to a
	// glyph via the global stone size; shapeLiteral uses the *Glyph field verbatim.
	blackShape stoneShape
	whiteShape stoneShape

	blackGlyph string // black stone, only when blackShape == shapeLiteral
	whiteGlyph string // white stone, only when whiteShape == shapeLiteral
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

// Styled glyph for a committed stone sitting on the recency trail: the stone's
// normal foreground over the trail background, so a stone keeps its color and
// only the background marks recency.
func (t boardTheme) trailCell(c stoneColor, rank int) string {
	st := t.black
	if c == white {
		st = t.white
	}
	return lipgloss.NewStyle().Bold(true).Foreground(st.GetForeground()).Background(t.trail[rank]).Render(t.glyph(c))
}

// Glyph for a side's stone under the current shape + stone size.
func (t boardTheme) glyph(c stoneColor) string {
	if c == white {
		return stoneGlyph(t.whiteShape, t.whiteGlyph)
	}
	return stoneGlyph(t.blackShape, t.blackGlyph)
}

// Styled mark for a point whose stone was captured on the last move.
func (t boardTheme) captureCell() string { return t.capture.Render(t.captureGlyph) }

// n spaces painted with the board background (plain spaces when the theme has
// none). Used for the grid gaps, margins, and label-row padding.
func (t boardTheme) space(n int) string {
	if t.bg == "" {
		return strings.Repeat(" ", n)
	}
	return lipgloss.NewStyle().Background(t.bg).Render(strings.Repeat(" ", n))
}

// Cursor bracket style tinted to the stone the local player places: bright for
// white, gray for black. Falls back to the plain cursor style when sideless.
func (t boardTheme) cursorStyle(c stoneColor) lipgloss.Style {
	switch c {
	case white:
		return t.white
	case black:
		return t.black
	default:
		return t.cursor
	}
}

// Styled glyph for a committed stone.
func (t boardTheme) stoneCell(c stoneColor) string {
	if c == white {
		return t.white.Render(t.glyph(white))
	}
	return t.black.Render(t.glyph(black))
}

// Styled glyph for an uncommitted placeholder: the stone over the placeholder tile.
func (t boardTheme) ghostCell(c stoneColor) string {
	st := t.black
	if c == white {
		st = t.white
	}
	return st.Background(t.placeholder.GetBackground()).Render(t.glyph(c))
}

// Bakes a board background into every on-board style so grid cells, labels, and
// the cursor sit on the same color. The placeholder/trail keep their own
// backgrounds (they mark ghost/recency), and turn is meta-column-only.
func withBG(t boardTheme, bg lipgloss.Color) boardTheme {
	t.bg = bg
	t.black = t.black.Background(bg)
	t.white = t.white.Background(bg)
	t.grid = t.grid.Background(bg)
	t.label = t.label.Background(bg)
	t.cursor = t.cursor.Background(bg)
	t.capture = t.capture.Background(bg)
	return t
}

// "Default": both sides filled discs, correct polarity (black darker, white
// bright), on a dim-yellow grid for the traditional go-board feel. Placeholder
// sits on a soft dark-blue tile.
func defaultTheme() boardTheme {
	return boardTheme{
		name:        "Default",
		blackShape:  shapeFilled,
		whiteShape:  shapeFilled,
		emptyGlyph:  "·",
		starGlyph:   "+",
		black:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("240")), // dark gray
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

// "Traditional": solid black/white stones on a goban-gold board with dark-gray
// grid marks.
func traditionalTheme() boardTheme {
	t := defaultTheme()
	t.name = "Traditional"
	t.black = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16"))  // true black
	t.white = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")) // bright white
	t.grid = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))             // dark gray
	t.label = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	t.cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("236"))
	t.placeholder = lipgloss.NewStyle().Background(lipgloss.Color("136")) // darker gold highlight
	t.trail = [trailLen]lipgloss.Color{
		lipgloss.Color("208"), // most recent: orange, contrasts with white
		lipgloss.Color("130"), // second: darker orange
	}
	return withBG(t, lipgloss.Color("179")) // goban gold
}

// "Hollow Black": Default scheme but the black stone is a hollow ring.
func hollowBlackTheme() boardTheme {
	t := defaultTheme()
	t.name = "Hollow Black"
	t.blackShape = shapeHollow
	return t
}

// "Hollow White": a light board with solid black stones and a light-gray hollow
// ring for white (readable against the near-white board).
func hollowWhiteTheme() boardTheme {
	t := defaultTheme()
	t.name = "Hollow White"
	t.blackShape = shapeFilled
	t.whiteShape = shapeHollow
	t.black = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16"))  // solid black
	t.white = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245")) // gray, visible on white
	t.grid = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	t.label = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	t.cursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16"))
	t.placeholder = lipgloss.NewStyle().Background(lipgloss.Color("153")) // light blue highlight
	return withBG(t, lipgloss.Color("255"))                              // near-white board
}

// "GnuGo": monochrome terminal-green ASCII — O for white, X for black.
func gnugoTheme() boardTheme {
	green := lipgloss.Color("10")
	st := lipgloss.NewStyle().Bold(true).Foreground(green)
	return boardTheme{
		name:         "GnuGo",
		blackShape:   shapeLiteral,
		whiteShape:   shapeLiteral,
		blackGlyph:   "X",
		whiteGlyph:   "O",
		emptyGlyph:   ".",
		starGlyph:    "+",
		black:        st,
		white:        st,
		placeholder:  lipgloss.NewStyle().Background(lipgloss.Color("22")), // dark green highlight
		grid:         lipgloss.NewStyle().Foreground(green),
		label:        lipgloss.NewStyle().Foreground(green),
		cursor:       st,
		turnGlyph:    "▶",
		turn:         st,
		trail:        [trailLen]lipgloss.Color{"#004d00", "#002600"},
		captureGlyph: "×",
		capture:      st,
	}
}

// @region board:theme-cycle

// All selectable themes, in cycle order. Index 0 is the startup default.
var themeCtors = []func() boardTheme{
	defaultTheme,
	traditionalTheme,
	hollowBlackTheme,
	hollowWhiteTheme,
	gnugoTheme,
}

var themeIndex = 0

// Active board theme.
var currentTheme = themeCtors[0]()

// Advances to the next theme and returns its display name.
func cycleTheme() string {
	themeIndex = (themeIndex + 1) % len(themeCtors)
	currentTheme = themeCtors[themeIndex]()
	return currentTheme.name
}

// Selects a theme by display name (no-op if unknown). For restoring a saved pref.
func setThemeByName(name string) {
	for i, ctor := range themeCtors {
		if t := ctor(); t.name == name {
			themeIndex = i
			currentTheme = t
			return
		}
	}
}
