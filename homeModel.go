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
const hotseatOption = "Play Hotseat"
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
	sessionExpired bool // auth died mid-session; red banner until next sign-in

	confirm       confirm // active yes/no prompt (delete), captures all input
	pendingDelete int64   // local game id awaiting delete confirmation
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
	e = append(e, homeEntry{kind: entryAction, action: hotseatOption})
	// TODO: re-enable GnuGo option once the backend is wired up.
	// e = append(e, homeEntry{kind: entryAction, action: gnuGoOption})
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
	if authed {
		h.sessionExpired = false // a fresh login clears the expiry banner
	}
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
	// A pending confirm swallows all input until resolved.
	if h.confirm.active() {
		switch h.confirm.handle(msg) {
		case confirmYes:
			id := h.pendingDelete
			h.confirm = confirm{}
			return h, func() tea.Msg { return deleteLocalGameMsg{id: id} }
		case confirmNo:
			h.confirm = confirm{}
		}
		return h, nil
	}
	switch key.String() {
	case "up", "k":
		h.moveCursor(-1)
	case "down", "j":
		h.moveCursor(1)
	case "D":
		// Delete only applies to local games; OGS games can't be removed here.
		if h.cursor < len(h.entries) {
			e := h.entries[h.cursor]
			if e.kind == entryGame && e.game.id < 0 {
				h.pendingDelete = e.game.id
				h.confirm = newConfirm(confirmDeleteGame,
					fmt.Sprintf("Delete local game %q? y to confirm · n to cancel", e.game.name))
			}
		}
	case "enter":
		if h.cursor < len(h.entries) {
			e := h.entries[h.cursor]
			switch {
			case e.kind == entryAction && e.action == signInOption:
				return h, func() tea.Msg { return openAuthMsg{} }
			case e.kind == entryAction && e.action == hotseatOption:
				return h, func() tea.Msg { return openSetupMsg{} }
			case e.kind == entryGame:
				g := e.game
				return h, func() tea.Msg { return openGameMsg{game: g} }
			}
		}
	}
	return h, nil
}

func (h homeModel) View(w, hgt int) string {
	// Bottom status bar: login state. Home-only, so game tabs keep full height.
	status := h.statusBar(w)
	bodyH := hgt
	if status != "" {
		bodyH -= lipgloss.Height(status)
	}

	// Render each entry to its lines, tracking where the selected entry sits so
	// the list can scroll to keep it visible when it outgrows the screen.
	var lines []string
	selStart, selCount := 0, 1
	for i, e := range h.entries {
		if e.kind == entryGame && i > 0 && h.entries[i-1].kind == entryGame {
			lines = append(lines, "") // blank line between games
		}
		start := len(lines)
		switch e.kind {
		case entryGame:
			lines = append(lines, strings.Split(renderGameEntry(e.game, i == h.cursor), "\n")...)
		case entryAction:
			lines = append(lines, renderActionEntry(e.action, i == h.cursor))
		case entrySpacer:
			lines = append(lines, "")
		case entryLoading:
			lines = append(lines, gutter(h.spinner.View()+gameMetaStyle.Render(" Loading games…"), false, false))
		}
		if i == h.cursor {
			selStart, selCount = start, len(lines)-start
		}
	}
	lines = windowLines(lines, selStart, selCount, bodyH)

	// Fixed 8-col left margin, vertically centered — avoids horizontal jitter
	// as row widths change (loading → games).
	menu := lipgloss.NewStyle().MarginLeft(8).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	body := lipgloss.Place(w, bodyH, lipgloss.Left, lipgloss.Center, menu)
	if status != "" {
		return lipgloss.JoinVertical(lipgloss.Left, body, status)
	}
	return body
}

// windowLines clips lines to height h, scrolling so the selected entry (the span
// [selStart, selStart+selCount)) stays visible and roughly centered. Returns the
// lines unchanged when they already fit.
func windowLines(lines []string, selStart, selCount, h int) []string {
	if h <= 0 || len(lines) <= h {
		return lines
	}
	top := selStart + selCount/2 - h/2 // center the cursor's entry
	if max := len(lines) - h; top > max {
		top = max
	}
	if top < 0 {
		top = 0
	}
	return lines[top : top+h]
}

// Login status bar: validating, or logged-in with refresh/logout hints.
func (h homeModel) statusBar(w int) string {
	// A pending confirm takes over the status line.
	if h.confirm.active() {
		return reconnectStyle.Width(w).Align(lipgloss.Center).Render("⚠ " + h.confirm.prompt)
	}
	switch {
	case h.sessionExpired:
		return errorStyle.Width(w).Align(lipgloss.Center).Bold(true).Render("Session expired — sign in again")
	case h.authPending:
		return dimStyle.Width(w).Align(lipgloss.Center).Render("Logging in …")
	case h.authed:
		refresh := dimStyle.Width(w).Align(lipgloss.Center).Render("r to refresh, ? for help")
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
	return gutter(lipgloss.JoinVertical(lipgloss.Left, line1, line2), selected, g.yourTurn())
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
		return selectedItemStyle.Render(">") + "  " + selectedItemStyle.Render(label)
	}
	return itemStyle.Render("   " + label)
}

// Prefixes a block with the 3-col left gutter: a selection caret, a gap, then
// the your-turn marker immediately left of the content. Continuation lines align.
func gutter(block string, selected, yourTurn bool) string {
	sel := " "
	if selected {
		sel = selectedItemStyle.Render(">")
	}
	turn := " "
	if yourTurn {
		turn = turnMarkerStyle.Render("▸")
	}
	lines := strings.Split(block, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = sel + " " + turn + lines[i]
		} else {
			lines[i] = "   " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}
