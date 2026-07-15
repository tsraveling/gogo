package main

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// How long the "Invalid position" flash stays up before dismissing.
const navErrorTTL = 800 * time.Millisecond

// A single active game: board column + meta column.
type gameModel struct {
	idx        int     // index into the parent's games slice, for routed messages
	game       game    // core game state this tab tracks
	backend    backend // where this game's authority lives (ogs/local/gnugo)
	board      boardModel
	info       infoModel
	chat       chatModel
	navMode    bool // "Go to" prompt is open, capturing input
	navErr     bool // last entry was invalid; flashing
	navGoto    textinput.Model
	spinner    spinner.Model
	connecting bool // awaiting the first gamedata snapshot
	connectErr bool // the socket failed to connect
}

func newGameModel(idx int, g game, b backend) gameModel {
	ti := textinput.New()
	ti.Prompt = "Go to > "
	ti.Placeholder = "A1"
	ti.CharLimit = 4
	ti.Width = 4
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(primaryColor)
	return gameModel{
		idx:     idx,
		game:    g,
		backend: b,
		board:   newBoardModel(g.width, g.height),
		info:    newInfoModel(),
		chat:    newChatModel(),
		navGoto: ti,
		spinner: sp,
	}
}

// Marks the game as connecting; returns the spinner tick to start it. A game
// that already holds a snapshot refetches silently (no spinner).
func (g *gameModel) beginConnect() tea.Cmd {
	g.connectErr = false
	if g.board.grid != nil {
		g.connecting = false
		return nil
	}
	g.connecting = true
	return g.spinner.Tick
}

// Applies a live snapshot: feeds the board and updates turn/phase state.
func (g *gameModel) applySnapshot(st boardState) {
	g.board.setState(st.grid)
	g.game.state = st
	g.connecting = false
	g.connectErr = false
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
	case spinner.TickMsg:
		if !g.connecting {
			return g, nil
		}
		var cmd tea.Cmd
		g.spinner, cmd = g.spinner.Update(msg)
		return g, cmd
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

	// Board column: the grid + control rows, or a centered loading/error box
	// while the socket connects and before the first snapshot arrives.
	var boardCol string
	switch {
	case g.connectErr:
		boardCol = g.centeredBoardBox(boardW, errorStyle.Render("Connection failed"))
	case g.connecting:
		boardCol = g.centeredBoardBox(boardW, g.spinner.View()+dimStyle.Render(" Loading board…"))
	default:
		boardCol = lipgloss.JoinVertical(lipgloss.Left, g.board.View(), g.controlView(boardW))
	}
	colHeight := lipgloss.Height(boardCol)

	// Layout: 2-col left margin, board, 3-col gap, then the meta column.
	const leftMargin = 2
	const colGap = 3
	metaW := max(termW-leftMargin-boardW-colGap, 0)
	infoH := 6
	chatH := max(colHeight-infoH, 0)
	metaCol := lipgloss.JoinVertical(
		lipgloss.Left,
		g.info.View(g.game, metaW, infoH),
		g.chat.View(metaW, chatH),
	)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		strings.Repeat(" ", leftMargin), boardCol,
		strings.Repeat(" ", colGap), metaCol,
	)
}

// A box the size of the board + control rows, with centered content. Keeps the
// column layout stable between the loading state and the rendered board.
func (g gameModel) centeredBoardBox(w int, content string) string {
	h := g.board.renderHeight() + 2 // board rows + the 2 control rows
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
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
