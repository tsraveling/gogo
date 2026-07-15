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
	ogs         ogsModel
	authPending bool // stored login present, validating at launch
}

func newModel() model {
	m := model{
		home: newHomeModel(),
		auth: newOGSAuthModel(),
	}
	// Restore open tabs; their game models are built once the game list loads.
	if tabs, err := loadTabs(); err == nil {
		m.tabs = tabs
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
func (m *model) openGame(g game) {
	for i, t := range m.tabs {
		if t.Source == "ogs" && t.GameID == g.id {
			m.active = i + 1
			return
		}
	}
	m.tabs = append(m.tabs, tabRef{Source: "ogs", GameID: g.id})
	m.games = append(m.games, newGameModel(len(m.games), g))
	_ = saveTabs(m.tabs)
	m.active = len(m.games)
}

// Closes the game tab at games-index i (active-1), reindexing the rest.
func (m *model) closeTab(i int) {
	m.games = append(m.games[:i], m.games[i+1:]...)
	m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
	for j := range m.games {
		m.games[j].idx = j
	}
	_ = saveTabs(m.tabs)
	if m.active > len(m.games) {
		m.active = len(m.games)
	}
}

// Rebuilds game models for restored tabs once the game list is known. Drops
// tabs whose game is no longer active. No-op once tabs are already open.
func (m *model) restoreTabs(games []game) {
	if len(m.games) > 0 {
		return
	}
	byID := make(map[int64]game, len(games))
	for _, g := range games {
		byID[g.id] = g
	}
	var kept []tabRef
	for _, t := range m.tabs {
		g, ok := byID[t.GameID]
		if !ok {
			continue
		}
		kept = append(kept, t)
		m.games = append(m.games, newGameModel(len(m.games), g))
	}
	m.tabs = kept
	_ = saveTabs(m.tabs)
}

// Home tab plus one per game.
func (m model) tabCount() int {
	return len(m.games) + 1
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
	if m.authPending {
		return tea.Batch(validateStoredAuth, m.home.spinner.Tick)
	}
	return validateStoredAuth
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
		return m, nil
	case gamesLoadedMsg:
		m.home.setGames(msg.games)
		m.restoreTabs(msg.games)
		return m, nil
	case openGameMsg:
		m.openGame(msg.game)
		return m, nil
	case navErrorExpiredMsg:
		if msg.game >= 0 && msg.game < len(m.games) {
			var cmd tea.Cmd
			m.games[msg.game], cmd = m.games[msg.game].Update(msg)
			return m, cmd
		}
		return m, nil
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
			return m, nil
		}
		// r refetches the game list when authenticated.
		if msg.String() == "r" && m.ogs.authenticated() {
			return m, tea.Batch(fetchGamesCmd(m.ogs), m.home.startLoading())
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "tab", "]":
			m.active = (m.active + 1) % m.tabCount()
			return m, nil
		case "shift+tab", "[":
			m.active = (m.active - 1 + m.tabCount()) % m.tabCount()
			return m, nil
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
		// Non-key messages (e.g. spinner ticks) go to the home tab.
		var cmd tea.Cmd
		m.home, cmd = m.home.Update(msg)
		return m, cmd
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
