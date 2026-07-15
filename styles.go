package main

import "github.com/charmbracelet/lipgloss"

var (
	primaryColor = lipgloss.Color("206")
	dimColor     = lipgloss.Color("243")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)

	// Stub panel backgrounds — placeholder until real content lands.
	boardBg   = lipgloss.Color("236")
	controlBg = lipgloss.Color("238")
	infoBg    = lipgloss.Color("24")
	chatBg    = lipgloss.Color("53")

	tabStyle       = lipgloss.NewStyle().Padding(0, 2).Foreground(dimColor)
	activeTabStyle = lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(lipgloss.Color("231")).Background(primaryColor)

	// Home menu entries.
	itemStyle         = lipgloss.NewStyle().Foreground(dimColor)
	selectedItemStyle = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
)

// renderPanel draws a flat colored rect of the given size with a centered label.
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
