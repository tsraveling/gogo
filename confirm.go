package main

import tea "github.com/charmbracelet/bubbletea"

// @region home:confirm

type confirmKind int

const (
	confirmNone confirmKind = iota
	confirmDeleteGame
)

type confirmResult int

const (
	confirmPending confirmResult = iota
	confirmYes
	confirmNo
)

// Reusable yes/no dialog. Rendering placement stays with the caller; confirm
// only owns the prompt and the key interpretation.
type confirm struct {
	kind   confirmKind
	prompt string
}

func newConfirm(kind confirmKind, prompt string) confirm {
	return confirm{kind: kind, prompt: prompt}
}

// Is a confirm waiting for a response?
func (c confirm) active() bool { return c.kind != confirmNone }

func (c confirm) handle(msg tea.Msg) confirmResult {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "y":
			return confirmYes
		case "n", "esc":
			return confirmNo
		}
	}
	return confirmPending
}
