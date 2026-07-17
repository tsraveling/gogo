package main

import "github.com/charmbracelet/lipgloss"

var (
	primaryColor = lipgloss.Color("206")
	dimColor     = lipgloss.Color("243")
	// Unemphasized: brighter than dim, not full white. Game titles for now.
	unemphColor = lipgloss.Color("250")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)

	// Board glyphs/colors live in theme.go (boardTheme / currentTheme).

	// Pass-confirm box: yellow border, warns before ending the turn.
	passBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("11")).
			Padding(0, 1)

	// Stub panel backgrounds — placeholder until real content lands.
	boardBg   = lipgloss.Color("236")
	controlBg = lipgloss.Color("238")
	infoBg    = lipgloss.Color("24")
	chatBg    = lipgloss.Color("53")

	tabStyle       = lipgloss.NewStyle().Padding(0, 2).Foreground(dimColor)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(lipgloss.Color("231")).Background(primaryColor)

	// Home menu entries.
	itemStyle         = lipgloss.NewStyle().Foreground(unemphColor)
	selectedItemStyle = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)

	// Home list game entries. Name uses active color for selection, bold for
	// your turn; players use bold white for the side to move, gray otherwise.
	gameNameStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))
	gameNameIdleStyle     = lipgloss.NewStyle().Foreground(unemphColor)
	gameNameSelectedStyle = lipgloss.NewStyle().Foreground(primaryColor)
	gameMetaStyle         = lipgloss.NewStyle().Foreground(dimColor)
	currentPlayerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))
	inactivePlayerStyle   = lipgloss.NewStyle().Foreground(dimColor)
	// White stone glyph inside the rank paren, so the color marker stands out.
	stoneStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))

	// Game info player bars: black bg/white text, and its inverse. Turn marker
	// lives in the board theme (theme.go).
	infoBlackStyle = lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("231"))
	infoWhiteStyle = lipgloss.NewStyle().Background(lipgloss.Color("231")).Foreground(lipgloss.Color("0"))

	dimStyle     = lipgloss.NewStyle().Foreground(dimColor)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
	// Transient reconnect notice while a dropped socket is being redialed.
	reconnectStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	// Your-turn marker: yellow ▸ on a game tab, yellow count beside the home icon.
	turnMarkerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	// Finished-game marker (both players passed). Scoring is separate.
	gameOverStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))

	// Auth modal box.
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 3)
)

// Draws a flat colored rect of the given size with a centered label.
func renderPanel(label string, w, h int, bg lipgloss.Color) string {
	return lipgloss.NewStyle().
		Width(w).
		Height(h).
		Background(bg).
		Foreground(lipgloss.Color("231")).
		Bold(true).
		Align(lipgloss.Center, lipgloss.Center).
		Render(label)
}
