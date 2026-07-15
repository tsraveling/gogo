package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// @region ogs:auth

// openAuthMsg asks the root model to open the auth modal.
type openAuthMsg struct{}

// closeAuthMsg asks the root model to dismiss the auth modal.
type closeAuthMsg struct{}

// ogsAuthModel is the modal login form: username + password. UI only;
// actual OGS auth is stubbed until the hookup section.
type ogsAuthModel struct {
	username textinput.Model
	password textinput.Model
	focus    int // 0 = username, 1 = password
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

// setFocus moves the cursor to field i (0=username, 1=password).
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

// reset clears entered text and returns focus to username.
func (m *ogsAuthModel) reset() {
	m.username.Reset()
	m.password.Reset()
	m.setFocus(0)
}

// attemptAuth is a stub; auth wiring lands in the hookup section.
func (m ogsAuthModel) attemptAuth() error {
	return nil
}

func (m ogsAuthModel) Update(msg tea.Msg) (ogsAuthModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			// enter on password: attempt (stubbed) auth, close on success.
			if err := m.attemptAuth(); err == nil {
				m.reset()
				return m, func() tea.Msg { return closeAuthMsg{} }
			}
			return m, nil
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
	form := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Sign in to OGS"),
		"",
		"Username",
		m.username.View(),
		"",
		"Password",
		m.password.View(),
		"",
		dimStyle.Render("tab: switch field · enter: next/submit · esc: cancel"),
	)
	box := modalStyle.Render(form)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
