package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	width  int
	height int
}

func newModel() model {
	return model{}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	body := lipgloss.JoinVertical(
		lipgloss.Center,
		titleStyle.Render("GoGo"),
		"",
		dimStyle.Render("press q to quit"),
	)

	if m.width == 0 {
		return body
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
}
