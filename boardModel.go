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
	width  int // board size in points (e.g. 9 for 9x9)
	height int
}

func newBoardModel(w, h int) boardModel {
	return boardModel{width: w, height: h}
}

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
		left := boardLabelStyle.Render(fmt.Sprintf("%*d", numW, num))
		right := boardLabelStyle.Render(fmt.Sprintf("%-*d", numW, num))
		sb.WriteByte('\n')
		sb.WriteString(left)
		sb.WriteByte(' ')
		sb.WriteString(b.pointRow(y))
		sb.WriteByte(' ')
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
	return prefix + boardLabelStyle.Render(strings.Join(cells, " "))
}

// One row of intersections: "." empty, "+" star point.
func (b boardModel) pointRow(y int) string {
	cells := make([]string, b.width)
	for x := 0; x < b.width; x++ {
		cells[x] = "."
		if b.isStar(x, y) {
			cells[x] = "+"
		}
	}
	return boardPointStyle.Render(strings.Join(cells, " "))
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
