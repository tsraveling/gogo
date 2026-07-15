package main

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// static tail options; do nothing for now.
var homeOptions = []string{"Sign in to OGS", "Play vs. GnuGo"}

// homeItem is a single menu entry.
type homeItem string

func (i homeItem) FilterValue() string { return string(i) }

// homeDelegate renders each entry on one line with a selection cursor.
type homeDelegate struct{}

func (homeDelegate) Height() int                         { return 1 }
func (homeDelegate) Spacing() int                        { return 0 }
func (homeDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (homeDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	label := string(item.(homeItem))
	if index == m.Index() {
		fmt.Fprint(w, selectedItemStyle.Render("> "+label))
		return
	}
	fmt.Fprint(w, itemStyle.Render("  "+label))
}

// homeModel is the first tab: a centered, navigable menu. No background.
// Will eventually list active games above the two static options.
type homeModel struct {
	list list.Model
}

func newHomeModel() homeModel {
	items := make([]list.Item, len(homeOptions))
	maxW := 0
	for i, o := range homeOptions {
		items[i] = homeItem(o)
		if w := lipgloss.Width(o) + 2; w > maxW {
			maxW = w
		}
	}

	l := list.New(items, homeDelegate{}, maxW, len(items))
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.SetSize(maxW, len(items)) // recompute layout now that chrome is off

	return homeModel{list: l}
}

func (h homeModel) Update(msg tea.Msg) (homeModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		if item, ok := h.list.SelectedItem().(homeItem); ok && string(item) == homeOptions[0] {
			return h, func() tea.Msg { return openAuthMsg{} }
		}
	}
	var cmd tea.Cmd
	h.list, cmd = h.list.Update(msg)
	return h, cmd
}

func (h homeModel) View(w, hgt int) string {
	return lipgloss.Place(w, hgt, lipgloss.Center, lipgloss.Center, h.list.View())
}
