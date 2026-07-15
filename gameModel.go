package main

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// How long the "Invalid position" flash stays up before dismissing.
const navErrorTTL = 800 * time.Millisecond

// A single active game: board column + meta column.
type gameModel struct {
	idx     int // index into the parent's games slice, for routed messages
	name    string
	board   boardModel
	info    infoModel
	chat    chatModel
	navMode bool // "Go to" prompt is open, capturing input
	navErr  bool // last entry was invalid; flashing
	navGoto textinput.Model
}

func newGameModel(idx int, name string, w, h int) gameModel {
	ti := textinput.New()
	ti.Prompt = "Go to > "
	ti.Placeholder = "A1"
	ti.CharLimit = 4
	ti.Width = 4
	return gameModel{
		idx:     idx,
		name:    name,
		board:   newBoardModel(w, h),
		info:    newInfoModel(),
		chat:    newChatModel(),
		navGoto: ti,
	}
}

// Dismisses the invalid-position flash on a specific game.
type navErrorExpiredMsg struct{ game int }

func navErrorCmd(idx int) tea.Cmd {
	return tea.Tick(navErrorTTL, func(time.Time) tea.Msg { return navErrorExpiredMsg{game: idx} })
}

// True while the "Go to" prompt should swallow keys (tabs/quit disabled).
func (g gameModel) capturingInput() bool { return g.navMode }

// @region board:navigation

func (g gameModel) Update(msg tea.Msg) (gameModel, tea.Cmd) {
	switch msg := msg.(type) {
	case navErrorExpiredMsg:
		g.navErr = false
		return g, nil
	case tea.KeyMsg:
		if g.navMode {
			return g.updateNav(msg)
		}
		switch msg.String() {
		case "g":
			g.navMode = true
			g.navErr = false
			g.navGoto.Reset()
			return g, g.navGoto.Focus()
		case "up", "k":
			g.board.MoveCursor(0, -1)
		case "down", "j":
			g.board.MoveCursor(0, 1)
		case "left", "h":
			g.board.MoveCursor(-1, 0)
		case "right", "l":
			g.board.MoveCursor(1, 0)
		}
		return g, nil
	default:
		// Async textinput messages (cursor blink) only matter while navigating.
		if g.navMode {
			var cmd tea.Cmd
			g.navGoto, cmd = g.navGoto.Update(msg)
			return g, cmd
		}
	}
	return g, nil
}

// Handles keys while the "Go to" prompt is open.
func (g gameModel) updateNav(msg tea.KeyMsg) (gameModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if x, y, ok := g.board.parsePosition(g.navGoto.Value()); ok {
			g.board.SetCursor(x, y)
			g.closeNav()
			return g, nil
		}
		g.navErr = true
		return g, navErrorCmd(g.idx)
	case "esc":
		g.closeNav()
		return g, nil
	}
	var cmd tea.Cmd
	g.navGoto, cmd = g.navGoto.Update(msg)
	return g, cmd
}

func (g *gameModel) closeNav() {
	g.navMode = false
	g.navErr = false
	g.navGoto.Blur()
	g.navGoto.Reset()
}

// @region board:view-model

func (g gameModel) View(termW, termH int) string {
	boardW := g.board.renderWidth()

	// Board column: board on top, 2-row control below.
	boardCol := lipgloss.JoinVertical(lipgloss.Left, g.board.View(), g.controlView(boardW))
	colHeight := lipgloss.Height(boardCol)

	// Meta column fills remaining width; info 6 rows, chat gets the rest.
	metaW := max(termW-boardW, 0)
	infoH := 6
	chatH := max(colHeight-infoH, 0)
	metaCol := lipgloss.JoinVertical(
		lipgloss.Left,
		g.info.View(metaW, infoH),
		g.chat.View(metaW, chatH),
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, boardCol, metaCol)
}

// Two-row board control: the "Go to" prompt (with error flash) or a key hint.
func (g gameModel) controlView(w int) string {
	box := lipgloss.NewStyle().Width(w).Height(2)
	if g.navMode {
		top := ""
		if g.navErr {
			top = errorStyle.Render("Invalid position")
		}
		return box.Render(lipgloss.JoinVertical(lipgloss.Left, top, g.navGoto.View()))
	}
	return box.Render(lipgloss.JoinVertical(lipgloss.Left, "", dimStyle.Render("g: go to")))
}
