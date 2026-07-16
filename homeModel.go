package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Menu labels; sign-in hidden once authenticated.
const signInOption = "Sign in to OGS"
const gnuGoOption = "Play vs. GnuGo"

type homeEntryKind int

const (
	entryGame homeEntryKind = iota
	entryAction
	entrySpacer // non-selectable gap
	entryLoading
)

// One home-list row: an active game, an action, a spacer, or a loading indicator.
type homeEntry struct {
	kind   homeEntryKind
	action string // entryAction label
	game   game   // entryGame data
}

func (e homeEntry) selectable() bool {
	return e.kind == entryGame || e.kind == entryAction
}

// First tab: a centered, navigable menu. No background.
// Lists active games first, then actions.
type homeModel struct {
	entries     []homeEntry
	cursor      int    // index into entries, always on a selectable row
	games       []game // OGS active games
	localGames  []game // local (hotseat) games
	spinner     spinner.Model
	loading     bool // fetching the game list
	authed      bool
	authPending bool   // validating a stored login; sign-in stays hidden
	username    string // OGS login shown in the status bar
}

func newHomeModel() homeModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(primaryColor)
	h := homeModel{spinner: s}
	h.rebuild()
	return h
}

// Rebuilds rows for the current games/auth state and keeps the cursor valid.
func (h *homeModel) rebuild() {
	var e []homeEntry
	// Only show the games-loading row while authed (or validating a stored login).
	if h.loading && (h.authed || h.authPending) {
		e = append(e, homeEntry{kind: entryLoading})
	}
	for _, g := range h.games {
		e = append(e, homeEntry{kind: entryGame, game: g})
	}
	for _, g := range h.localGames {
		e = append(e, homeEntry{kind: entryGame, game: g})
	}
	if len(h.games)+len(h.localGames) > 0 {
		e = append(e, homeEntry{kind: entrySpacer})
	}
	if !h.authed && !h.authPending {
		e = append(e, homeEntry{kind: entryAction, action: signInOption})
	}
	e = append(e, homeEntry{kind: entryAction, action: gnuGoOption})
	h.entries = e
	h.clampCursor()
}

// Snaps the cursor onto the nearest selectable row.
func (h *homeModel) clampCursor() {
	if h.cursor < 0 {
		h.cursor = 0
	}
	if h.cursor >= len(h.entries) {
		h.cursor = len(h.entries) - 1
	}
	for i := h.cursor; i < len(h.entries); i++ {
		if h.entries[i].selectable() {
			h.cursor = i
			return
		}
	}
	for i := h.cursor; i >= 0; i-- {
		if h.entries[i].selectable() {
			h.cursor = i
			return
		}
	}
}

// Steps the cursor by dir, skipping spacers; stays put at the ends.
func (h *homeModel) moveCursor(dir int) {
	for i := h.cursor + dir; i >= 0 && i < len(h.entries); i += dir {
		if h.entries[i].selectable() {
			h.cursor = i
			return
		}
	}
}

// setGames replaces the active-games list, ends loading, and rebuilds.
func (h *homeModel) setGames(games []game) {
	h.games = games
	h.loading = false
	h.rebuild()
}

// setLocalGames replaces the local-games list and rebuilds the menu.
func (h *homeModel) setLocalGames(games []game) {
	h.localGames = games
	h.rebuild()
}

// startLoading shows the loading row and returns the spinner's first tick.
// No-op (nil) when already loading, so the tick loop isn't doubled.
func (h *homeModel) startLoading() tea.Cmd {
	if h.loading {
		return nil
	}
	h.loading = true
	h.rebuild()
	return h.spinner.Tick
}

// setAuthed updates auth state (and the status-bar login) and refreshes the menu.
func (h *homeModel) setAuthed(authed bool, username string) {
	h.username = username
	if h.authed == authed {
		return
	}
	h.authed = authed
	h.rebuild()
}

// setAuthPending toggles the validating-a-stored-login state.
func (h *homeModel) setAuthPending(pending bool) {
	if h.authPending == pending {
		return
	}
	h.authPending = pending
	h.rebuild()
}

func (h homeModel) Update(msg tea.Msg) (homeModel, tea.Cmd) {
	if _, ok := msg.(spinner.TickMsg); ok {
		if !h.loading {
			return h, nil
		}
		var cmd tea.Cmd
		h.spinner, cmd = h.spinner.Update(msg)
		return h, cmd
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return h, nil
	}
	switch key.String() {
	case "up", "k":
		h.moveCursor(-1)
	case "down", "j":
		h.moveCursor(1)
	case "enter":
		if h.cursor < len(h.entries) {
			e := h.entries[h.cursor]
			switch {
			case e.kind == entryAction && e.action == signInOption:
				return h, func() tea.Msg { return openAuthMsg{} }
			case e.kind == entryGame:
				g := e.game
				return h, func() tea.Msg { return openGameMsg{game: g} }
			}
		}
	}
	return h, nil
}

func (h homeModel) View(w, hgt int) string {
	var rows []string
	for i, e := range h.entries {
		selected := i == h.cursor
		switch e.kind {
		case entryGame:
			if i > 0 && h.entries[i-1].kind == entryGame {
				rows = append(rows, "") // blank line between games
			}
			rows = append(rows, renderGameEntry(e.game, selected))
		case entryAction:
			rows = append(rows, renderActionEntry(e.action, selected))
		case entrySpacer:
			rows = append(rows, "")
		case entryLoading:
			rows = append(rows, gutter(h.spinner.View()+gameMetaStyle.Render(" Loading games…"), false))
		}
	}
	// Fixed 8-col left margin, vertically centered — avoids horizontal jitter
	// as row widths change (loading → games).
	menu := lipgloss.NewStyle().MarginLeft(8).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))

	// Bottom status bar: login state. Home-only, so game tabs keep full height.
	status := h.statusBar(w)
	bodyH := hgt
	if status != "" {
		bodyH -= lipgloss.Height(status)
	}
	body := lipgloss.Place(w, bodyH, lipgloss.Left, lipgloss.Center, menu)
	if status != "" {
		return lipgloss.JoinVertical(lipgloss.Left, body, status)
	}
	return body
}

// Login status bar: validating, or logged-in with refresh/logout hints.
func (h homeModel) statusBar(w int) string {
	switch {
	case h.authPending:
		return dimStyle.Width(w).Align(lipgloss.Center).Render("Logging in …")
	case h.authed:
		refresh := dimStyle.Width(w).Align(lipgloss.Center).Render("r to refresh")
		login := dimStyle.Width(w).Align(lipgloss.Center).
			Render("Logged in as " + h.username + ". Q to logout.")
		return lipgloss.JoinVertical(lipgloss.Left, refresh, login)
	}
	return ""
}

// A two-line game row: name + size, then both players with color markers.
func renderGameEntry(g game, selected bool) string {
	// White+bold on your turn, gray otherwise; selection overrides the color.
	nameStyle := gameNameIdleStyle
	if g.yourTurn() {
		nameStyle = gameNameStyle
	}
	if selected {
		nameStyle = gameNameSelectedStyle
	}
	prefix := ""
	if g.id < 0 {
		prefix = gameMetaStyle.Render("Local: ")
	}
	line1 := prefix + nameStyle.Bold(g.yourTurn()).Render(g.name) + " " +
		gameMetaStyle.Render(fmt.Sprintf("(%dx%d)", g.width, g.height))
	var line2 string
	if g.id < 0 { // local: no ranks
		line2 = playerName(g, black) + " " + stoneStyle.Render("●") + " " +
			gameMetaStyle.Render("vs") + " " +
			playerName(g, white) + " " + stoneStyle.Render("○")
	} else {
		line2 = playerName(g, black) + " " +
			rankParen("●", g.black.rankString()) + " " +
			gameMetaStyle.Render("vs") + " " +
			playerName(g, white) + " " +
			rankParen("○", g.white.rankString())
	}
	return gutter(lipgloss.JoinVertical(lipgloss.Left, line1, line2), selected)
}

// A rank paren with the stone glyph highlighted white: e.g. "(○ 25 kyu)".
func rankParen(stone, rank string) string {
	return gameMetaStyle.Render("(") + stoneStyle.Render(stone) +
		gameMetaStyle.Render(" "+rank+")")
}

// Player name styled bold white when it's that side's move, gray otherwise.
func playerName(g game, side stoneColor) string {
	name := g.black.name
	if side == white {
		name = g.white.name
	}
	if g.state.playerToMove == side {
		return currentPlayerStyle.Render(name)
	}
	return inactivePlayerStyle.Render(name)
}

// A single-line action row.
func renderActionEntry(label string, selected bool) string {
	if selected {
		return selectedItemStyle.Render("> " + label)
	}
	return itemStyle.Render("  " + label)
}

// Prefixes a block with a selection marker on the first line, aligning the rest.
func gutter(block string, selected bool) string {
	marker := "  "
	if selected {
		marker = selectedItemStyle.Render("> ")
	}
	lines := strings.Split(block, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = marker + lines[i]
		} else {
			lines[i] = "  " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}
