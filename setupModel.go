package main

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// @region setup:ui

// Asks the root model to open the game-setup modal.
type openSetupMsg struct{}

// Asks the root model to dismiss the setup modal.
type closeSetupMsg struct{}

// Result of creating a local game from the setup modal.
type localGameCreatedMsg struct {
	game game
	err  error
}

// Defaults for a new local game (only board size is chosen in the MVP).
const (
	setupDefaultRuleset = rulesetJapanese
	setupDefaultKomi    = 6.5
	setupBlackName      = "Black"
	setupWhiteName      = "White"
)

// A selectable board size in the setup list.
type sizeItem struct {
	label string
	size  boardSize
}

func (s sizeItem) Title() string       { return s.label }
func (s sizeItem) Description() string { return "" }
func (s sizeItem) FilterValue() string { return s.label }

// Modal to create a local (hotseat) game. Board size only for now; reused for
// OGS-online setup later.
type setupModel struct {
	list list.Model
}

func newSetupModel() setupModel {
	items := []list.Item{
		sizeItem{"9 × 9", size9},
		sizeItem{"13 × 13", size13},
		sizeItem{"19 × 19", size19},
	}
	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1) // one line per item (default is 2, for the hidden description)
	d.SetSpacing(0)

	l := list.New(items, d, 24, len(items)+1) // +1 for the list's internal overhead
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	return setupModel{list: l}
}

// Resets the selection to the top.
func (m *setupModel) reset() { m.list.Select(0) }

// Creates and persists a local game off the UI goroutine.
func createLocalGameCmd(size boardSize) tea.Cmd {
	return func() tea.Msg {
		g, err := newLocalGame(size, setupDefaultRuleset, setupDefaultKomi, setupBlackName, setupWhiteName)
		return localGameCreatedMsg{game: g, err: err}
	}
}

func (m setupModel) Update(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			return m, func() tea.Msg { return closeSetupMsg{} }
		case "enter":
			if it, ok := m.list.SelectedItem().(sizeItem); ok {
				return m, createLocalGameCmd(it.size)
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m setupModel) View(w, h int) string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("New Hotseat Game"),
		"",
		"Board size",
		m.list.View(),
		"",
		dimStyle.Render("↑/↓: size · enter: start · esc: cancel"),
	)
	box := modalStyle.Render(body)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
