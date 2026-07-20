package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// @region help:modal

// A key/description pair; a section header when key is "".
type helpRow struct {
	key  string
	desc string
}

// Keys available on every view.
var globalHelpRows = []helpRow{
	{"", "Everywhere"},
	{"?", "this help"},
	{"tab / ]", "next tab"},
	{"shift+tab / [", "previous tab"},
	{"q / ctrl+c", "quit"},
}

var gameHelpRows = append([]helpRow{
	{"", "Board"},
	{"↑↓←→ / hjkl", "move cursor"},
	{"space", "place / preview stone"},
	{"enter", "confirm pending stone"},
	{"m", "toggle fast mode"},
	{"p", "pass"},
	{"g", "go to coordinate"},
	{"t", "cycle board theme"},
	{"", "Chat"},
	{"/", "start chatting"},
	{"esc", "stop chatting"},
	{"tab", "switch channel (while chatting)"},
	{"- / =", "scroll chat up / down"},
	{"", "This game"},
	{"X", "close tab"},
	{"r", "reconnect"},
}, globalHelpRows...)

var homeHelpRows = append([]helpRow{
	{"", "Home"},
	{"↑↓ / jk", "move cursor"},
	{"enter", "open game / select"},
	{"D", "delete local game"},
	{"r", "refresh game list"},
	{"Q", "sign out"},
}, globalHelpRows...)

// Full-screen centered help panel for the given rows.
func helpView(rows []helpRow, w, h int) string {
	keyStyle := lipgloss.NewStyle().Foreground(primaryColor)
	descStyle := lipgloss.NewStyle().Foreground(unemphColor)

	var b strings.Builder
	for i, r := range rows {
		if r.key == "" {
			if i > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "%s\n", dimStyle.Render(strings.ToUpper(r.desc)))
			continue
		}
		fmt.Fprintf(&b, "%s  %s\n",
			keyStyle.Render(fmt.Sprintf("%-14s", r.key)),
			descStyle.Render(r.desc))
	}
	fmt.Fprintf(&b, "\n%s", dimStyle.Italic(true).Render("any key to close"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2).
		Render(strings.TrimRight(b.String(), "\n"))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
