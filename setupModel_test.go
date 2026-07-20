package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Guards against the list clipping to fewer rows than its items (pagination /
// delegate-height regressions have bitten this twice).
func TestSetupListShowsAllSizes(t *testing.T) {
	m := newSetupModel()
	rows := 0
	for _, ln := range strings.Split(m.list.View(), "\n") {
		if strings.TrimSpace(ln) != "" {
			rows++
		}
	}
	if rows != 3 {
		t.Fatalf("want 3 size rows visible, got %d:\n%q", rows, m.list.View())
	}
}

// Drives the setup-modal flow at the model level: open modal → pick size →
// create → a local game tab opens and gets focus.
func TestSetupModalCreatesGame(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(model)

	// Open the setup modal; it must render without panicking.
	next, _ = m.Update(openSetupMsg{})
	m = next.(model)
	if !m.showSetup {
		t.Fatalf("setup modal should be open")
	}
	if m.View() == "" {
		t.Fatalf("setup modal rendered empty")
	}

	// Enter on the size list creates the game; run the returned command.
	sm, c := m.setup.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.setup = sm
	if c == nil {
		t.Fatalf("enter should return a create command")
	}
	msg := c()
	created, ok := msg.(localGameCreatedMsg)
	if !ok || created.err != nil {
		t.Fatalf("want localGameCreatedMsg with no error, got %+v", msg)
	}

	// Deliver the creation result: modal closes, tab opens and is focused.
	next, _ = m.Update(created)
	m = next.(model)
	if m.showSetup {
		t.Fatalf("modal should close after create")
	}
	if len(m.games) != 1 {
		t.Fatalf("want 1 open game, got %d", len(m.games))
	}
	if m.active != 1 {
		t.Fatalf("new game tab should be focused, active=%d", m.active)
	}
	if m.games[0].game.id >= 0 {
		t.Fatalf("local game id should be negative, got %d", m.games[0].game.id)
	}

	// The new game tab renders without panicking.
	if m.View() == "" {
		t.Fatalf("game tab rendered empty")
	}
}
