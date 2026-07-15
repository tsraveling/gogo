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
	games       []gameModel
	active      int // 0 = home tab; 1..n = games[active-1]
	auth        ogsAuthModel
	showAuth    bool // modal open, captures all input
	ogs         ogsModel
	authPending bool // stored login present, validating at launch
}

func newModel() model {
	m := model{
		home: newHomeModel(),
		games: []gameModel{
			newGameModel("9x9", 9, 9),
			newGameModel("13x13", 13, 13),
			newGameModel("19x19", 19, 19),
		},
		auth: newOGSAuthModel(),
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
		// X logs out when authenticated.
		if msg.String() == "X" && m.ogs.authenticated() {
			_ = m.ogs.clear()
			m.ogs = ogsModel{}
			m.home.setGames(nil)
			m.home.setAuthed(false, "")
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
		if m.active == 0 {
			var cmd tea.Cmd
			m.home, cmd = m.home.Update(msg)
			return m, cmd
		}
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
		labels = append(labels, g.name)
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
	bodyH := m.height - lipgloss.Height(tabs)

	// Login status bar now lives in the home tab (home.View), not here.
	var body string
	if m.active == 0 {
		body = m.home.View(m.width, bodyH)
	} else {
		body = m.games[m.active-1].View(m.width, bodyH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, tabs, body)
}
