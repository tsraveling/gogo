package main

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Menu labels; sign-in hidden once authenticated.
const signInOption = "Sign in to OGS"
const gnuGoOption = "Play vs. GnuGo"

// Single menu entry.
type homeItem string

func (i homeItem) FilterValue() string { return string(i) }

// Renders each entry on one line with a selection cursor.
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

// First tab: a centered, navigable menu. No background.
// Will eventually list active games above the static options.
type homeModel struct {
	list        list.Model
	authed      bool
	authPending bool // validating a stored login; sign-in stays hidden
}

// Menu labels for the current auth state; sign-in dropped while authed or validating.
func homeMenuOptions(authed, pending bool) []string {
	if authed || pending {
		return []string{gnuGoOption}
	}
	return []string{signInOption, gnuGoOption}
}

func newHomeModel() homeModel {
	l := list.New(nil, homeDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)

	h := homeModel{list: l}
	h.rebuild()
	return h
}

// Repopulates the list for the current auth state and sizes it.
func (h *homeModel) rebuild() {
	opts := homeMenuOptions(h.authed, h.authPending)
	items := make([]list.Item, len(opts))
	maxW := 0
	for i, o := range opts {
		items[i] = homeItem(o)
		if w := lipgloss.Width(o) + 2; w > maxW {
			maxW = w
		}
	}
	h.list.SetItems(items)
	h.list.SetSize(maxW, len(items))
}

// Updates auth state and refreshes the menu.
func (h *homeModel) setAuthed(authed bool) {
	if h.authed == authed {
		return
	}
	h.authed = authed
	h.rebuild()
}

// Toggles the validating-a-stored-login state.
func (h *homeModel) setAuthPending(pending bool) {
	if h.authPending == pending {
		return
	}
	h.authPending = pending
	h.rebuild()
}

func (h homeModel) Update(msg tea.Msg) (homeModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		if item, ok := h.list.SelectedItem().(homeItem); ok && string(item) == signInOption {
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
