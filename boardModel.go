package main

import (
	"fmt"
	"strconv"
	"strings"
)

// @region board:view-model

// Column coordinate letters, Go convention (skips I). Max 25 wide.
const coordLetters = "ABCDEFGHJKLMNOPQRSTUVWXYZ"

// Renders a single game's board state.
type boardModel struct {
	width        int            // board size in points (e.g. 9 for 9x9)
	height       int            // board size in points
	grid         [][]stoneColor // current stones, [y][x]; nil renders an empty board
	interactable bool           // show and move a cursor
	cursorX      int
	cursorY      int

	// Uncommitted move preview: a hollow stone shown before enter commits it.
	ghostActive bool
	ghostX      int
	ghostY      int
	ghostColor  stoneColor
}

func newBoardModel(w, h int) boardModel {
	return boardModel{width: w, height: h, interactable: true, cursorX: w / 2, cursorY: h / 2}
}

// Feeds the renderer a stone grid ([y][x]). Reused later for replay/variants.
func (b *boardModel) setState(grid [][]stoneColor) { b.grid = grid }

// Stone at the point, or empty when off-grid / no grid loaded.
func (b boardModel) stoneAt(x, y int) stoneColor {
	if y < 0 || y >= len(b.grid) || x < 0 || x >= len(b.grid[y]) {
		return empty
	}
	return b.grid[y][x]
}

// @region board:navigation

// Clamps to the board and places the cursor. Also the parent-facing setter.
func (b *boardModel) SetCursor(x, y int) {
	b.cursorX = clamp(x, 0, b.width-1)
	b.cursorY = clamp(y, 0, b.height-1)
}

// Steps the cursor, clamped to the board edges.
func (b *boardModel) MoveCursor(dx, dy int) { b.SetCursor(b.cursorX+dx, b.cursorY+dy) }

// Shows an uncommitted move preview at a point.
func (b *boardModel) SetGhost(x, y int, c stoneColor) {
	b.ghostActive = true
	b.ghostX, b.ghostY, b.ghostColor = x, y, c
}

func (b *boardModel) ClearGhost() { b.ghostActive = false }

// Parses a coordinate like "A1" (letter + row, 1 = bottom). ok is false if
// out of range or malformed.
func (b boardModel) parsePosition(s string) (x, y int, ok bool) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if len(s) < 2 {
		return 0, 0, false
	}
	col := strings.IndexByte(coordLetters, s[0])
	if col < 0 || col >= b.width {
		return 0, 0, false
	}
	n, err := strconv.Atoi(s[1:])
	if err != nil || n < 1 || n > b.height {
		return 0, 0, false
	}
	return col, b.height - n, true // 1 is the bottom row
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// @region board:render

// Digits needed for the row numbers (1..height).
func (b boardModel) numW() int { return len(strconv.Itoa(b.height)) }

// Board rect width: numbers + margin on each side, points spaced by one char.
func (b boardModel) renderWidth() int {
	return b.numW()*2 + b.width*2 + 1
}

// Board rect height: h point rows + a letter row top and bottom.
func (b boardModel) renderHeight() int {
	return b.height + 2
}

func (b boardModel) View() string {
	numW := b.numW()
	letters := b.letterRow(numW)

	var sb strings.Builder
	sb.WriteString(letters)
	for y := 0; y < b.height; y++ {
		num := b.height - y // 1 is the bottom row, climbing upward
		left := currentTheme.label.Render(fmt.Sprintf("%*d", numW, num))
		right := currentTheme.label.Render(fmt.Sprintf("%-*d", numW, num))
		sb.WriteByte('\n')
		sb.WriteString(left)
		sb.WriteString(b.boardRow(y))
		sb.WriteString(right)
	}
	sb.WriteByte('\n')
	sb.WriteString(letters)
	return sb.String()
}

// Column-letter header, aligned over the board region.
func (b boardModel) letterRow(numW int) string {
	cells := make([]string, b.width)
	for x := 0; x < b.width; x++ {
		cells[x] = string(coordLetters[x])
	}
	prefix := strings.Repeat(" ", numW+1)
	return prefix + currentTheme.label.Render(strings.Join(cells, " "))
}

// One board line, including the left/right margin chars. Points are spaced by
// one char; the cursor's "[" and "]" occupy those gap/margin chars so column
// alignment is unchanged.
func (b boardModel) boardRow(y int) string {
	cx := -1
	if b.interactable && b.cursorY == y {
		cx = b.cursorX
	}

	var sb strings.Builder
	if cx == 0 {
		sb.WriteString(currentTheme.cursor.Render("["))
	} else {
		sb.WriteByte(' ')
	}
	for x := 0; x < b.width; x++ {
		sb.WriteString(b.cellStr(x, y))
		if x == b.width-1 {
			break
		}
		switch {
		case x == cx:
			sb.WriteString(currentTheme.cursor.Render("]"))
		case x+1 == cx:
			sb.WriteString(currentTheme.cursor.Render("["))
		default:
			sb.WriteByte(' ')
		}
	}
	if cx == b.width-1 {
		sb.WriteString(currentTheme.cursor.Render("]"))
	} else {
		sb.WriteByte(' ')
	}
	return sb.String()
}

// One point (via the active theme): a stone if occupied, a dimmed ghost if
// previewed, else a star or empty intersection.
func (b boardModel) cellStr(x, y int) string {
	switch b.stoneAt(x, y) {
	case black, white:
		return currentTheme.stoneCell(b.stoneAt(x, y))
	}
	if b.ghostActive && b.ghostX == x && b.ghostY == y {
		return currentTheme.ghostCell(b.ghostColor)
	}
	if b.isStar(x, y) {
		return currentTheme.grid.Render(currentTheme.starGlyph)
	}
	return currentTheme.grid.Render(currentTheme.emptyGlyph)
}

// Star point (hoshi) test. Square boards only; matches standard 9/13/19 layouts.
func (b boardModel) isStar(x, y int) bool {
	if b.width != b.height {
		return false
	}
	s := b.width
	var edge int
	switch {
	case s >= 13:
		edge = 3
	case s >= 7:
		edge = 2
	default:
		return false
	}
	onLine := func(v int) bool { return v == edge || v == s-1-edge }
	center := -1
	if s%2 == 1 {
		center = s / 2
	}

	if center >= 0 && x == center && y == center {
		return true // tengen / center
	}
	if onLine(x) && onLine(y) {
		return true // corner stars
	}
	// Side-midpoint stars appear on 19x19 only.
	if s >= 19 && center >= 0 {
		if (onLine(x) && y == center) || (x == center && onLine(y)) {
			return true
		}
	}
	return false
}
