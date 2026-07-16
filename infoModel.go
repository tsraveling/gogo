package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shows metadata about the game: players/turn, handicap/komi, captures.
type infoModel struct{}

func newInfoModel() infoModel {
	return infoModel{}
}

// Yellow turn marker; a space keeps the bar aligned when it's not that side's move.
const turnDot = "●"

func (i infoModel) View(g game, w, h int) string {
	lines := []string{
		i.playerRow(g, w),
		splitLR(dimStyle.Render(fmt.Sprintf("Handicap: %d", g.handicap)),
			dimStyle.Render(fmt.Sprintf("Komi: %.1f", g.komi)), w),
		dimStyle.Render("0 captures"),
	}
	if g.state.finished() {
		lines = append(lines, gameOverStyle.Render("⚑ finished")+dimStyle.Render(" — both passed"))
	}
	rows := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.NewStyle().Width(w).Height(h).Render(rows)
}

// `● black    vs    white ●` — black bg left, white bg right, turn dot outside.
func (i infoModel) playerRow(g game, w int) string {
	bBar := infoBlackStyle.Render(" " + g.black.name + " ")
	wBar := infoWhiteStyle.Render(" " + g.white.name + " ")

	lTurn, rTurn := " ", " "
	if g.state.playerToMove == black {
		lTurn = turnDotStyle.Render(turnDot)
	}
	if g.state.playerToMove == white {
		rTurn = turnDotStyle.Render(turnDot)
	}

	left := lTurn + " " + bBar
	right := wBar + " " + rTurn
	return splitMid(left, " vs ", right, w)
}

// Left-aligns l, right-aligns r, fills the gap with normal-background spaces.
func splitLR(l, r string, w int) string {
	gap := w - lipgloss.Width(l) - lipgloss.Width(r)
	if gap < 1 {
		return l + " " + r
	}
	return l + strings.Repeat(" ", gap) + r
}

// Places l left, m centered, r right within width w.
func splitMid(l, m, r string, w int) string {
	gap := w - lipgloss.Width(l) - lipgloss.Width(m) - lipgloss.Width(r)
	if gap < 2 {
		return l + m + r
	}
	lgap := gap / 2
	return l + strings.Repeat(" ", lgap) + m + strings.Repeat(" ", gap-lgap) + r
}
