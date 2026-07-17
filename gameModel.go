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
	idx         int     // index into the parent's games slice, for routed messages
	game        game    // core game state this tab tracks
	backend     backend // where this game's authority lives (ogs/local/gnugo)
	board       boardModel
	info        infoModel
	chat        chatModel
	navMode     bool // "Go to" prompt is open, capturing input
	navErr      bool // last entry was invalid; flashing
	navGoto     textinput.Model
	spinner     spinner.Model
	connecting  bool   // awaiting the first gamedata snapshot
	connectErr  bool   // the socket failed to connect
	committing   bool   // a move submission is in flight
	reconnecting bool   // yellow: the reconnect ladder is running (OGS only)
	disconnected bool   // red: the ladder gave up; manual retry only (OGS only)
	reconnectRung int   // current rung of the reconnect ladder
	reconnectGen  int   // bumped to invalidate in-flight ladder ticks/results
	moveErr      string // last rejected move, shown in the control area
	submitOK    bool   // brief ✓ after a confirmed remote submit
	passConfirm bool   // pass-confirm box is open, capturing input
	fastMode    bool   // space plays immediately, skipping the ghost step
}

// How long the green ✓ shows after a confirmed OGS submit.
const submitOKTTL = 900 * time.Millisecond

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
		idx:      idx,
		game:     g,
		backend:  b,
		board:    newBoardModel(g.width, g.height),
		info:     newInfoModel(),
		chat:     newChatModel(),
		navGoto:  ti,
		spinner:  sp,
		fastMode: g.you == empty, // hotseat: no fixed side, default to fast play
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

// Applies a live snapshot: feeds the board and updates turn/phase state. A
// snapshot means the socket is healthy, so it clears every connect/reconnect
// state and invalidates any pending ladder ticks.
func (g *gameModel) applySnapshot(st boardState) {
	g.board.setState(st.grid)
	g.game.state = st
	g.connecting = false
	g.connectErr = false
	g.reconnecting = false
	g.disconnected = false
	g.reconnectGen++
	g.reconnectRung = 0
}

// cancelReconnect stops any running ladder and clears the reconnect/give-up
// states — called when a tab is blurred or closed (an intentional disconnect).
func (g *gameModel) cancelReconnect() {
	g.reconnectGen++
	g.reconnecting = false
	g.disconnected = false
	g.reconnectRung = 0
}

// playBlockReason returns a non-empty message when the socket state forbids
// playing (OGS reconnecting/given-up); empty when a move may proceed.
func (g gameModel) playBlockReason() string {
	if g.disconnected {
		return "reconnect to play"
	}
	if g.reconnecting {
		return "Reconnecting…"
	}
	return ""
}

// Result of a move submission, routed back to its game tab. The new board
// arrives separately as a snapshot via the events channel.
type moveResultMsg struct {
	gameID int64
	err    error
}

// Submits a move off the UI goroutine.
func submitMoveCmd(b backend, gameID int64, m move) tea.Cmd {
	return func() tea.Msg {
		return moveResultMsg{gameID: gameID, err: b.SubmitMove(m)}
	}
}

// Clears the in-flight flag; on success drops the ghost, on reject keeps it for
// retry and surfaces the reason. Remote (OGS) submits flash a brief ✓.
func (g *gameModel) applyMoveResult(err error) tea.Cmd {
	g.committing = false
	g.reconnecting = false
	if err != nil {
		g.moveErr = moveErrText(err)
		return nil
	}
	g.board.ClearGhost()
	g.moveErr = ""
	if !g.backend.Instant() {
		g.submitOK = true
		return submitOKCmd(g.idx)
	}
	return nil
}

// Dismisses the ✓ badge on a specific game.
type submitOKExpiredMsg struct{ game int }

func submitOKCmd(idx int) tea.Cmd {
	return tea.Tick(submitOKTTL, func(time.Time) tea.Msg { return submitOKExpiredMsg{game: idx} })
}

// The color the local player would place: their fixed side, or the side to move
// in a hotseat game (you == empty).
func (g gameModel) placingColor() stoneColor {
	if g.game.you != empty {
		return g.game.you
	}
	return g.game.state.playerToMove
}

// Whether it's the local player's move. Hotseat (you == empty) is always live —
// either side plays; sided games (OGS) require it to be your turn.
func (g gameModel) myTurn() bool {
	return g.game.you == empty || g.game.yourTurn()
}

// Play is possible only once a live position is loaded, the game is ongoing, and
// it's the local player's move.
func (g gameModel) canPlay() bool {
	return !g.connecting && !g.connectErr && g.board.grid != nil &&
		!g.game.state.finished() && g.myTurn()
}

// Human-readable reason for a rejected move.
func moveErrText(err error) string {
	switch err {
	case errOccupied:
		return "Point occupied"
	case errSuicide:
		return "Suicide — illegal"
	case errKo:
		return "Ko — illegal recapture"
	case errNotYourTurn:
		return "Not your turn"
	case errOffBoard:
		return "Off board"
	case errGameOver:
		return "Game over"
	default:
		return err.Error()
	}
}

// Dismisses the invalid-position flash on a specific game.
type navErrorExpiredMsg struct{ game int }

func navErrorCmd(idx int) tea.Cmd {
	return tea.Tick(navErrorTTL, func(time.Time) tea.Msg { return navErrorExpiredMsg{game: idx} })
}

// True while a modal-like prompt should swallow keys (tabs/quit disabled).
func (g gameModel) capturingInput() bool { return g.navMode || g.passConfirm }

// @region board:navigation

func (g gameModel) Update(msg tea.Msg) (gameModel, tea.Cmd) {
	switch msg := msg.(type) {
	case navErrorExpiredMsg:
		g.navErr = false
		return g, nil
	case submitOKExpiredMsg:
		g.submitOK = false
		return g, nil
	case spinner.TickMsg:
		if !g.connecting && !g.committing && !g.reconnecting {
			return g, nil
		}
		var cmd tea.Cmd
		g.spinner, cmd = g.spinner.Update(msg)
		return g, cmd
	case tea.KeyMsg:
		if g.navMode {
			return g.updateNav(msg)
		}
		if g.passConfirm {
			return g.updatePassConfirm(msg)
		}
		if g.committing {
			return g, nil // board frozen while a move is in flight
		}
		switch msg.String() {
		case "g":
			g.navMode = true
			g.navErr = false
			g.navGoto.Reset()
			return g, g.navGoto.Focus()
		case "m":
			g.fastMode = !g.fastMode
			g.board.ClearGhost() // drop any pending ghost when switching modes
			return g, nil
		case " ":
			if g.fastMode {
				return g.submitAt(g.board.cursorX, g.board.cursorY)
			}
			g.toggleGhost()
			return g, nil
		case "enter":
			return g.commitMove()
		case "p":
			if reason := g.playBlockReason(); reason != "" {
				g.moveErr = reason
				return g, nil
			}
			if g.canPlay() {
				g.passConfirm = true
			}
			return g, nil
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

// @region board:play

// Places a ghost at the cursor, relocates it, or clears it when it's already
// there. No-op on occupied points or a read-only board.
func (g *gameModel) toggleGhost() {
	if !g.canPlay() {
		return
	}
	x, y := g.board.cursorX, g.board.cursorY
	if g.board.stoneAt(x, y) != empty {
		return
	}
	if g.board.ghostActive && g.board.ghostX == x && g.board.ghostY == y {
		g.board.ClearGhost()
		return
	}
	g.board.SetGhost(x, y, g.placingColor())
}

// Submits the ghosted move; no-op without a ghost.
func (g gameModel) commitMove() (gameModel, tea.Cmd) {
	if !g.board.ghostActive {
		return g, nil
	}
	return g.submitAt(g.board.ghostX, g.board.ghostY)
}

// Submits a stone at a point. No-op on a read-only board or an occupied point.
// A blocked socket state surfaces the reason instead of playing.
func (g gameModel) submitAt(x, y int) (gameModel, tea.Cmd) {
	if reason := g.playBlockReason(); reason != "" {
		g.moveErr = reason
		return g, nil
	}
	if !g.canPlay() || g.board.stoneAt(x, y) != empty {
		return g, nil
	}
	m := move{x: x, y: y, color: g.placingColor()}
	g.committing = true
	g.moveErr = ""
	return g, tea.Batch(submitMoveCmd(g.backend, g.game.id, m), g.spinner.Tick)
}

// Handles keys while the pass-confirm box is open.
func (g gameModel) updatePassConfirm(msg tea.KeyMsg) (gameModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		g.passConfirm = false
		if reason := g.playBlockReason(); reason != "" {
			g.moveErr = reason
			return g, nil
		}
		m := move{x: -1, y: -1, color: g.placingColor()}
		g.committing = true
		g.moveErr = ""
		return g, tea.Batch(submitMoveCmd(g.backend, g.game.id, m), g.spinner.Tick)
	case "esc":
		g.passConfirm = false
	}
	return g, nil
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

// Board control area: pass-confirm box, "Go to" prompt, move-submission status,
// or a key hint, depending on state.
func (g gameModel) controlView(w int) string {
	box := lipgloss.NewStyle().Width(w).Height(2)
	switch {
	case g.passConfirm:
		return passBoxStyle.Render("⚠ Pass?  " + dimStyle.Render("enter: confirm · esc: cancel"))
	case g.navMode:
		top := ""
		if g.navErr {
			top = errorStyle.Render("Invalid position")
		}
		return box.Render(lipgloss.JoinVertical(lipgloss.Left, top, g.navGoto.View()))
	case g.game.state.finished():
		return box.Render(lipgloss.JoinVertical(lipgloss.Left,
			gameOverStyle.Render("⚑ Game over"), dimStyle.Render("(both passed)")))
	case g.disconnected:
		return box.Render(lipgloss.JoinVertical(lipgloss.Left,
			errorStyle.Render("Disconnected"), dimStyle.Render("r to reconnect")))
	case g.reconnecting:
		return box.Render(lipgloss.JoinVertical(lipgloss.Left, "", g.spinner.View()+reconnectStyle.Render(" Reconnecting…")))
	case g.committing:
		return box.Render(lipgloss.JoinVertical(lipgloss.Left, "", g.spinner.View()+dimStyle.Render(" Submitting move…")))
	case g.submitOK:
		return box.Render(lipgloss.JoinVertical(lipgloss.Left, "", successStyle.Render("✓ Move submitted")))
	case g.moveErr != "":
		return box.Render(lipgloss.JoinVertical(lipgloss.Left,
			errorStyle.Render(g.moveErr), dimStyle.Render("space: reposition · enter: retry")))
	case !g.myTurn():
		return box.Render(lipgloss.JoinVertical(lipgloss.Left, "", dimStyle.Render("Waiting for opponent…")))
	case g.board.ghostActive:
		return box.Render(lipgloss.JoinVertical(lipgloss.Left, g.fastTag(), dimStyle.Render("hit enter to submit · p: pass")))
	default:
		hint := "space: place · p: pass · g: go to · m: fast"
		if g.fastMode {
			hint = "space: play · p: pass · g: go to · m: normal"
		}
		return box.Render(lipgloss.JoinVertical(lipgloss.Left, g.fastTag(), dimStyle.Render(hint)))
	}
}

// Green FAST badge when fast mode is on; empty otherwise.
func (g gameModel) fastTag() string {
	if g.fastMode {
		return successStyle.Render("FAST")
	}
	return ""
}
