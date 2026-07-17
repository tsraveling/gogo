package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// First (home) tab label: Nerd Font house glyph, or plain unicode when GOGO_ASCII set.
var homeIcon = func() string {
	if os.Getenv("GOGO_ASCII") != "" {
		return "⌂"
	}
	return ""
}()

type model struct {
	width       int
	height      int
	home        homeModel
	games       []gameModel // open game tabs, parallel to tabs
	tabs        []tabRef    // persisted open-tab refs, parallel to games
	active      int         // 0 = home tab; 1..n = games[active-1]
	auth        ogsAuthModel
	showAuth    bool // modal open, captures all input
	setup       setupModel
	showSetup   bool // setup modal open, captures all input
	ogs         ogsModel
	authPending bool // stored login present, validating at launch

	events chan gameEvent // backend snapshots, drained by waitForGameEvent
}

func newModel() model {
	m := model{
		home:   newHomeModel(),
		auth:   newOGSAuthModel(),
		setup:  newSetupModel(),
		events: make(chan gameEvent, 16),
	}
	// Restore open tabs; their game models are built once the game list loads.
	if tabs, err := loadTabs(); err == nil {
		m.tabs = tabs
	}
	// Local (hotseat) games list independently of OGS auth.
	if lg, err := loadLocalGames(); err == nil {
		m.home.setLocalGames(lg)
	}
	// Stored login: validate on launch, hide sign-in until it resolves,
	// show the games loading indicator (tick started in Init).
	if o, err := loadOGS(); err == nil && o.authenticated() {
		m.authPending = true
		m.home.loading = true
		m.home.setAuthPending(true)
	}
	return m
}

// @region tabs:manage

// Switches to the game's tab, opening (and persisting) a new one if needed.
// Returns a cmd that connects the new tab's backend (nil when already open).
func (m *model) openGame(g game) tea.Cmd {
	for i, t := range m.tabs {
		if t.GameID == g.id {
			m.active = i + 1
			return nil
		}
	}
	// Local games (negative id) reload from disk — the saved position is
	// authoritative, and the home copy may be stale.
	var b backend
	source := "ogs"
	if g.id < 0 {
		fresh, lb, ok := openLocalGame(g.id)
		if !ok {
			return nil // record gone; nothing to open
		}
		source, g, b = "local", fresh, lb
	} else {
		b = &ogsBackend{gameID: g.id, whiteID: g.white.id, ogs: m.ogs}
	}
	m.tabs = append(m.tabs, tabRef{Source: source, GameID: g.id})
	gm := newGameModel(len(m.games), g, b)
	spin := gm.beginConnect()
	m.games = append(m.games, gm)
	_ = saveTabs(m.tabs)
	m.active = len(m.games)
	return tea.Batch(connectBackendCmd(b, g.id, m.events), spin)
}

// Closes the game tab at games-index i (active-1), reindexing the rest. A
// finished local game is deleted outright (no scoring/archive yet).
func (m *model) closeTab(i int) {
	ref := m.tabs[i]
	finished := m.games[i].game.state.finished()
	m.games[i].backend.Disconnect()
	m.games = append(m.games[:i], m.games[i+1:]...)
	m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
	for j := range m.games {
		m.games[j].idx = j
	}
	_ = saveTabs(m.tabs)
	if ref.Source == "local" && finished {
		_ = deleteLocalGame(ref.GameID)
		if lg, err := loadLocalGames(); err == nil {
			m.home.setLocalGames(lg)
		}
	}
	if m.active > len(m.games) {
		m.active = len(m.games)
	}
}

// Finds an open game by id; nil if none (id 0 is never a match).
func (m *model) gameByID(id int64) *gameModel {
	if id == 0 {
		return nil
	}
	for i := range m.games {
		if m.games[i].game.id == id {
			return &m.games[i]
		}
	}
	return nil
}

// Rebuilds game models for restored tabs once the game list is known, connecting
// each tab's backend (per-tab lifetime). Drops tabs whose game is no longer
// active. No-op once tabs are already open. Returns a cmd that dials the backends.
func (m *model) restoreTabs(games []game) tea.Cmd {
	if len(m.games) > 0 {
		return nil
	}
	byID := make(map[int64]game, len(games))
	for _, g := range games {
		byID[g.id] = g
	}
	var kept []tabRef
	var cmds []tea.Cmd
	for _, t := range m.tabs {
		var g game
		var b backend
		if t.Source == "local" {
			lg, lb, ok := openLocalGame(t.GameID)
			if !ok {
				continue // record gone
			}
			g, b = lg, lb
		} else {
			og, ok := byID[t.GameID]
			if !ok {
				continue // no longer an active OGS game (or logged out)
			}
			g, b = og, &ogsBackend{gameID: og.id, whiteID: og.white.id, ogs: m.ogs}
		}
		kept = append(kept, t)
		gm := newGameModel(len(m.games), g, b)
		cmds = append(cmds, gm.beginConnect(), connectBackendCmd(b, g.id, m.events))
		m.games = append(m.games, gm)
	}
	m.tabs = kept
	_ = saveTabs(m.tabs)
	return tea.Batch(cmds...)
}

// Home tab plus one per game.
func (m model) tabCount() int {
	return len(m.games) + 1
}

// Refreshes the OGS game list when the home tab is focused and authenticated.
// Called on every navigation back to home so the list stays current.
func (m *model) refreshIfHome() tea.Cmd {
	if m.active != 0 || !m.ogs.authenticated() {
		return nil
	}
	return tea.Batch(fetchGamesCmd(m.ogs), m.home.startLoading())
}

// Result of validating persisted auth at launch.
type authLoadedMsg struct {
	ogs ogsModel
	ok  bool
}

// Delivers the user's active games after an auth succeeds.
type gamesLoadedMsg struct {
	games []game
}

// Home selected a game: focus its tab, opening one if not already present.
type openGameMsg struct {
	game game
}

// Fetches active games off the UI goroutine; empty list on error (MVP).
func fetchGamesCmd(o ogsModel) tea.Cmd {
	return func() tea.Msg {
		games, err := fetchActiveGames(o.AccessToken, o.UserID)
		if err != nil {
			return gamesLoadedMsg{}
		}
		return gamesLoadedMsg{games: games}
	}
}

func (m model) Init() tea.Cmd {
	// waitForGameEvent runs for the app's lifetime, draining socket snapshots.
	if m.authPending {
		return tea.Batch(validateStoredAuth, m.home.spinner.Tick, waitForGameEvent(m.events))
	}
	return tea.Batch(validateStoredAuth, waitForGameEvent(m.events))
}

// Refreshes persisted tokens to confirm the login still works; clears a stale one.
func validateStoredAuth() tea.Msg {
	o, err := loadOGS()
	if err != nil || !o.authenticated() {
		return authLoadedMsg{}
	}
	refreshed, err := authenticateRefresh(o.RefreshToken)
	if err != nil {
		_ = o.clear()
		return authLoadedMsg{}
	}
	_ = refreshed.save()
	return authLoadedMsg{ogs: refreshed, ok: true}
}

// @region tabs:input

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case authLoadedMsg:
		m.authPending = false
		m.home.setAuthPending(false)
		if msg.ok {
			m.ogs = msg.ogs
			m.home.setAuthed(true, m.ogs.Username)
			return m, tea.Batch(fetchGamesCmd(m.ogs), m.home.startLoading())
		}
		// Logged out / no stored auth: still restore local (hotseat) tabs.
		return m, m.restoreTabs(nil)
	case gamesLoadedMsg:
		m.home.setGames(msg.games)
		return m, m.restoreTabs(msg.games)
	case openGameMsg:
		return m, m.openGame(msg.game)
	case gameEvent:
		if gm := m.gameByID(msg.gameID); gm != nil {
			if msg.reconnecting {
				gm.reconnecting = true
				return m, tea.Batch(gm.spinner.Tick, waitForGameEvent(m.events))
			}
			gm.applySnapshot(msg.state)
		}
		return m, waitForGameEvent(m.events) // keep listening
	case moveResultMsg:
		if gm := m.gameByID(msg.gameID); gm != nil {
			return m, gm.applyMoveResult(msg.err)
		}
		return m, nil
	case backendConnectedMsg:
		if msg.err != nil {
			if gm := m.gameByID(msg.gameID); gm != nil {
				gm.connecting = false
				gm.connectErr = true
			}
		}
		return m, nil
	case navErrorExpiredMsg:
		if msg.game >= 0 && msg.game < len(m.games) {
			var cmd tea.Cmd
			m.games[msg.game], cmd = m.games[msg.game].Update(msg)
			return m, cmd
		}
		return m, nil
	case submitOKExpiredMsg:
		if msg.game >= 0 && msg.game < len(m.games) {
			var cmd tea.Cmd
			m.games[msg.game], cmd = m.games[msg.game].Update(msg)
			return m, cmd
		}
		return m, nil
	case openSetupMsg:
		m.setup.reset()
		m.showSetup = true
		return m, nil
	case closeSetupMsg:
		m.showSetup = false
		return m, nil
	case localGameCreatedMsg:
		m.showSetup = false
		if msg.err != nil {
			return m, nil // silently drop; MVP has no create-error surface
		}
		// Refresh the home list so the new game shows, then open its tab.
		if lg, err := loadLocalGames(); err == nil {
			m.home.setLocalGames(lg)
		}
		return m, m.openGame(msg.game)
	case openAuthMsg:
		m.auth.reset()
		m.auth.prefillUsername(m.ogs.Username)
		m.showAuth = true
		return m, nil
	case closeAuthMsg:
		m.showAuth = false
		return m, nil
	case welcomeDoneMsg:
		m.showAuth = false
		m.auth.reset()
		return m, nil
	case authResultMsg:
		// Persist a successful login before the auth modal shows its banner.
		var cmd tea.Cmd
		m.auth, cmd = m.auth.Update(msg)
		if msg.err == nil {
			m.ogs = msg.ogs
			_ = m.ogs.save()
			m.home.setAuthed(true, m.ogs.Username)
			return m, tea.Batch(cmd, fetchGamesCmd(m.ogs), m.home.startLoading())
		}
		return m, cmd
	case tea.KeyMsg:
		// Modal captures all input; tabs and quit keys are disabled.
		if m.showAuth {
			var cmd tea.Cmd
			m.auth, cmd = m.auth.Update(msg)
			return m, cmd
		}
		if m.showSetup {
			var cmd tea.Cmd
			m.setup, cmd = m.setup.Update(msg)
			return m, cmd
		}
		// A game's "Go to" prompt captures all input the same way.
		if m.active > 0 && m.games[m.active-1].capturingInput() {
			var cmd tea.Cmd
			m.games[m.active-1], cmd = m.games[m.active-1].Update(msg)
			return m, cmd
		}
		// Q (shift+q) logs out when authenticated; guarded key to avoid misfires.
		if msg.String() == "Q" && m.ogs.authenticated() {
			_ = m.ogs.clear()
			m.ogs = ogsModel{}
			m.home.setGames(nil)
			m.home.setAuthed(false, "")
			return m, nil
		}
		// X closes the focused game tab (home tab can't close).
		if msg.String() == "X" && m.active > 0 {
			m.closeTab(m.active - 1)
			return m, m.refreshIfHome() // refresh if we landed back on home
		}
		// r refetches the game list when authenticated.
		if msg.String() == "r" && m.ogs.authenticated() {
			return m, tea.Batch(fetchGamesCmd(m.ogs), m.home.startLoading())
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			for i := range m.games {
				m.games[i].backend.Disconnect() // release every open game's backend on quit
			}
			return m, tea.Quit
		case "tab", "]":
			m.active = (m.active + 1) % m.tabCount()
			return m, m.refreshIfHome()
		case "shift+tab", "[":
			m.active = (m.active - 1 + m.tabCount()) % m.tabCount()
			return m, m.refreshIfHome()
		}
		// Delegate remaining keys to the active tab.
		var cmd tea.Cmd
		if m.active == 0 {
			m.home, cmd = m.home.Update(msg)
		} else {
			m.games[m.active-1], cmd = m.games[m.active-1].Update(msg)
		}
		return m, cmd
	default:
		// Non-key messages (spinner ticks, cursor blinks) fan out to the home
		// tab and the focused game; each ignores ticks that aren't its own.
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.home, cmd = m.home.Update(msg)
		cmds = append(cmds, cmd)
		if m.active > 0 {
			m.games[m.active-1], cmd = m.games[m.active-1].Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// Top tab bar: home tab first, then one per game.
func (m model) renderTabs() string {
	labels := make([]string, 0, m.tabCount())
	labels = append(labels, homeIcon)
	for _, g := range m.games {
		labels = append(labels, g.game.name)
	}

	tabs := make([]string, len(labels))
	for i, label := range labels {
		if i == m.active {
			tabs[i] = activeTabStyle.Render(label)
		} else {
			tabs[i] = tabStyle.Render(label)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

// @region tabs:render

func (m model) View() string {
	if m.width == 0 {
		return titleStyle.Render("GoGo")
	}

	// Modal takes the full screen; tab bar hidden while open.
	if m.showAuth {
		return m.auth.View(m.width, m.height)
	}
	if m.showSetup {
		return m.setup.View(m.width, m.height)
	}

	tabs := m.renderTabs()
	bodyH := m.height - lipgloss.Height(tabs) - 1 // 1 blank row below the tab bar

	// Login status bar now lives in the home tab (home.View), not here.
	var body string
	if m.active == 0 {
		body = m.home.View(m.width, bodyH)
	} else {
		body = m.games[m.active-1].View(m.width, bodyH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, tabs, "", body)
}
