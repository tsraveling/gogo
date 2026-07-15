package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// homeIcon labels the first (home) tab: Nerd Font house glyph (nf-fa-home),
// or a plain-unicode house when GOGO_ASCII is set (no Nerd Font detection
// is possible at runtime, so it's an opt-out).
var homeIcon = func() string {
	if os.Getenv("GOGO_ASCII") != "" {
		return "⌂"
	}
	return ""
}()

type model struct {
	width    int
	height   int
	home     homeModel
	games    []gameModel
	active   int // 0 = home tab; 1..n = games[active-1]
	auth        ogsAuthModel
	showAuth    bool // when true, the auth modal is open and captures all input
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
	// A stored login means we'll validate it on launch: show "Logging in …"
	// and keep the sign-in menu entry hidden until the check resolves.
	if o, err := loadOGS(); err == nil && o.authenticated() {
		m.authPending = true
		m.home.setAuthPending(true)
	}
	return m
}

// tabCount is the home tab plus one per game.
func (m model) tabCount() int {
	return len(m.games) + 1
}

// authLoadedMsg reports the result of validating persisted auth at launch.
type authLoadedMsg struct {
	ogs ogsModel
	ok  bool
}

func (m model) Init() tea.Cmd {
	return validateStoredAuth
}

// validateStoredAuth loads persisted tokens and refreshes them to confirm the
// login still works. A stale login is cleared. Runs off the UI goroutine.
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
			m.home.setAuthed(true)
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
		if msg.err == nil {
			m.ogs = msg.ogs
			_ = m.ogs.save()
			m.home.setAuthed(true)
		}
		var cmd tea.Cmd
		m.auth, cmd = m.auth.Update(msg)
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
			m.home.setAuthed(false)
			return m, nil
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
	}
	return m, nil
}

// renderTabs draws the top tab bar: home tab first, then one per game.
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

	// Reserve a bottom row for login status: validating, or logged in.
	var status string
	switch {
	case m.authPending:
		status = dimStyle.Width(m.width).Align(lipgloss.Center).Render("Logging in …")
	case m.ogs.authenticated():
		status = dimStyle.Width(m.width).Align(lipgloss.Center).
			Render("Logged in as " + m.ogs.Username + ". X to logout.")
	}
	if status != "" {
		bodyH -= lipgloss.Height(status)
	}

	var body string
	if m.active == 0 {
		body = m.home.View(m.width, bodyH)
	} else {
		body = m.games[m.active-1].View(m.width, bodyH)
	}

	if status != "" {
		return lipgloss.JoinVertical(lipgloss.Left, tabs, body, status)
	}
	return lipgloss.JoinVertical(lipgloss.Left, tabs, body)
}
