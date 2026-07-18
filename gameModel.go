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
	termW       int    // last known terminal size, for chat layout
	termH       int
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
	chat := newChatModel()
	chat.setContext(g, b != nil && b.SupportsChat())
	return gameModel{
		idx:      idx,
		game:     g,
		backend:  b,
		board:    newBoardModel(g.width, g.height),
		info:     newInfoModel(),
		chat:     chat,
		navGoto:  ti,
		spinner:  sp,
		fastMode: g.you == empty, // hotseat: no fixed side, default to fast play
	}
}

// setSize records the terminal size and re-lays the chat viewport.
func (g *gameModel) setSize(termW, termH int) {
	g.termW = termW
	g.termH = termH
	g.syncChat()
}

// Rows the tab bar + its trailing blank consume above the body (see model.View).
const tabOverhead = 2

// chatDims computes the chat panel's width/height. The meta column fills the
// body height rather than tracking the shorter board column, so chat space no
// longer shrinks with the board.
func (g gameModel) chatDims() (int, int) {
	const leftMargin = 2
	const colGap = 3
	const infoH = 3
	metaW := max(g.termW-leftMargin-g.board.renderWidth()-colGap, 0)
	bodyH := g.termH - tabOverhead
	return metaW, max(bodyH-infoH, 1)
}

// syncChat re-lays the chat viewport for the current size.
func (g *gameModel) syncChat() {
	w, h := g.chatDims()
	g.chat.refresh(w, h)
}

// addChat appends an incoming line and re-lays the log.
func (g *gameModel) addChat(m chatMessage) {
	g.chat.addMessage(m)
	g.syncChat()
}

// myPlayerID / myName identify the local user for outgoing (optimistic) chat,
// derived from the side they play. Zero/empty in a hotseat game (no chat).
func (g gameModel) myPlayerID() int64 {
	switch g.game.you {
	case black:
		return g.game.black.id
	case white:
		return g.game.white.id
	}
	return 0
}

func (g gameModel) myName() string {
	switch g.game.you {
	case black:
		return g.game.black.name
	case white:
		return g.game.white.name
	}
	return ""
}

// sendChat adds the line optimistically and dispatches it to the backend; the
// server echo (with a chat_id) upgrades the optimistic entry.
func (g *gameModel) sendChat(text string) tea.Cmd {
	m := chatMessage{
		playerID:   g.myPlayerID(),
		username:   g.myName(),
		body:       text,
		channel:    g.chat.mode,
		moveNumber: g.game.state.moveNumber,
		date:       time.Now().Unix(),
		pending:    true,
	}
	g.chat.addMessage(m)
	g.syncChat()
	return sendChatCmd(g.backend, g.game.id, m)
}

// Result of a chat send, routed by gameID. Errors are non-fatal (the optimistic
// line already shows); wired for future surfacing.
type chatSentMsg struct {
	gameID int64
	err    error
}

func sendChatCmd(b backend, gameID int64, m chatMessage) tea.Cmd {
	return func() tea.Msg {
		return chatSentMsg{gameID: gameID, err: b.SendChat(m)}
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
	// Capture marks: points that held a stone before and are empty now (play
	// phase only — stone-removal scoring also clears points). Diffing works for
	// every backend without backend-specific capture reporting.
	if st.phase == phasePlay {
		g.board.setCaptures(capturedPoints(g.game.state.grid, st.grid))
	} else {
		g.board.setCaptures(nil)
	}
	g.board.setState(st.grid)
	g.board.setTrail(st.trail())
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

// True while a modal-like prompt should swallow keys (tabs/quit disabled). A
// focused chat composer captures too — tab cycles its mode, keys type.
func (g gameModel) capturingInput() bool { return g.navMode || g.passConfirm || g.chat.focused }

// @region game:input

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
		if g.chat.focused {
			return g.updateChat(msg)
		}
		if g.committing {
			return g, nil // board frozen while a move is in flight
		}
		switch msg.String() {
		case "-":
			g.chat.scrollUp()
			return g, nil
		case "=":
			g.chat.scrollDown()
			return g, nil
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
			// A pending stone commits; otherwise enter jumps into the chat composer.
			if g.board.ghostActive {
				return g.commitMove()
			}
			if g.chat.canChat {
				return g, g.chat.focus()
			}
			return g, nil
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
		// Async textinput/textarea messages (cursor blink) go to whichever input
		// is live.
		if g.navMode {
			var cmd tea.Cmd
			g.navGoto, cmd = g.navGoto.Update(msg)
			return g, cmd
		}
		if g.chat.focused {
			var cmd tea.Cmd
			g.chat, cmd = g.chat.Update(msg)
			return g, cmd
		}
	}
	return g, nil
}

// updateChat handles keys while the chat composer is focused. Enter sends (empty
// = no-op, stays focused); esc clears and unfocuses; tab cycles the channel.
func (g gameModel) updateChat(msg tea.KeyMsg) (gameModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		g.chat.blur()
		return g, nil
	case "tab":
		g.chat.cycleMode()
		return g, nil
	case "enter":
		text := strings.TrimSpace(g.chat.ta.Value())
		if text == "" {
			return g, nil // nothing typed: stay focused, do nothing
		}
		cmd := g.sendChat(text)
		g.chat.ta.Reset()
		return g, cmd // stays focused for the next line
	}
	var cmd tea.Cmd
	g.chat, cmd = g.chat.Update(msg)
	return g, cmd
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

// @region game:view

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
	// Layout: 2-col left margin, board, 3-col gap, then the meta column.
	const leftMargin = 2
	const colGap = 3
	const infoH = 3
	metaW, chatH := g.chatDims()
	g.chat.refresh(metaW, chatH) // size the viewport to the current width (copy)
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
