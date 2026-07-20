package main

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// @region ogs:auth-ui

// Asks the root model to open the auth modal.
type openAuthMsg struct{}

// Asks the root model to dismiss the auth modal.
type closeAuthMsg struct{}

// Outcome of an OGS login attempt.
type authResultMsg struct {
	ogs ogsModel
	err error
}

// Fires after the success banner has been shown.
type welcomeDoneMsg struct{}

// How long the green "Welcome!" shows before the modal closes.
const welcomeDuration = 800 * time.Millisecond

// Modal login form: username + password.
type ogsAuthModel struct {
	username   textinput.Model
	password   textinput.Model
	focus      int  // 0 = username, 1 = password
	submitting bool // request in flight
	success    bool // showing welcome banner
	errText    string
}

func newOGSAuthModel() ogsAuthModel {
	u := textinput.New()
	u.Placeholder = "username"
	u.CharLimit = 64
	u.Width = 32

	p := textinput.New()
	p.Placeholder = "password"
	p.CharLimit = 64
	p.Width = 32
	p.EchoMode = textinput.EchoPassword

	m := ogsAuthModel{username: u, password: p}
	m.setFocus(0)
	return m
}

// Moves the cursor to field i (0=username, 1=password).
func (m *ogsAuthModel) setFocus(i int) {
	m.focus = i
	if i == 0 {
		m.username.Focus()
		m.password.Blur()
	} else {
		m.username.Blur()
		m.password.Focus()
	}
}

// Clears entered text and transient state, returning focus to username.
func (m *ogsAuthModel) reset() {
	m.username.Reset()
	m.password.Reset()
	m.submitting = false
	m.success = false
	m.errText = ""
	m.setFocus(0)
}

// Seeds the username field (e.g. from a prior login).
func (m *ogsAuthModel) prefillUsername(name string) {
	m.username.SetValue(name)
}

// Dispatches the login request as a command.
func (m ogsAuthModel) submit() tea.Cmd {
	user, pass := m.username.Value(), m.password.Value()
	return func() tea.Msg {
		ogs, err := authenticatePassword(user, pass)
		return authResultMsg{ogs: ogs, err: err}
	}
}

func (m ogsAuthModel) Update(msg tea.Msg) (ogsAuthModel, tea.Cmd) {
	switch msg := msg.(type) {
	case authResultMsg:
		m.submitting = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		m.success = true
		m.errText = ""
		return m, tea.Tick(welcomeDuration, func(time.Time) tea.Msg { return welcomeDoneMsg{} })

	case tea.KeyMsg:
		// Ignore input while a request is in flight or on the success banner.
		if m.submitting || m.success {
			return m, nil
		}
		switch msg.String() {
		case "esc":
			m.reset()
			return m, func() tea.Msg { return closeAuthMsg{} }
		case "tab", "shift+tab":
			m.setFocus((m.focus + 1) % 2)
			return m, nil
		case "enter":
			if m.focus == 0 {
				m.setFocus(1)
				return m, nil
			}
			m.errText = ""
			m.submitting = true
			return m, m.submit()
		}
	}

	var cmd tea.Cmd
	if m.focus == 0 {
		m.username, cmd = m.username.Update(msg)
	} else {
		m.password, cmd = m.password.Update(msg)
	}
	return m, cmd
}

func (m ogsAuthModel) View(w, h int) string {
	var status string
	switch {
	case m.success:
		status = successStyle.Render("Welcome!")
	case m.submitting:
		status = dimStyle.Render("Signing in…")
	case m.errText != "":
		status = errorStyle.Render(m.errText)
	default:
		status = dimStyle.Render("tab: switch field · enter: next/submit · esc: cancel")
	}

	form := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Sign in to OGS"),
		"",
		"Username",
		m.username.View(),
		"",
		"Password",
		m.password.View(),
		"",
		status,
	)
	box := modalStyle.Render(form)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
