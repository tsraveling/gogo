package main

import "github.com/charmbracelet/lipgloss"

var (
	primaryColor = lipgloss.Color("206")
	dimColor     = lipgloss.Color("243")

	dimStyle   = lipgloss.NewStyle().Foreground(dimColor)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
)
