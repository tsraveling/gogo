package main

import (
	"fmt"
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
	games       []gameModel // open game tabs, parallel to tabs
	tabs        []tabRef    // persisted open-tab refs, parallel to games
	active      int         // 0 = home tab; 1..n = games[active-1]
	auth        ogsAuthModel
	showAuth    bool // modal open, captures all input
	setup       setupModel
	showSetup   bool // setup modal open, captures all input
	showHelp    bool // help modal open; any key closes it
	ogs         ogsModel
	authPending bool // stored login present, validating at launch
	polling     bool // the 20s overview poll loop is running

	events chan gameEvent // backend snapshots, drained by waitForGameEvent
}

func newModel() model {
	m := model{
		home:   newHomeModel(),
		auth:   newOGSAuthModel(),
		setup:  newSetupModel(),
		events: make(chan gameEvent, 16),
	}
	// Restore the chosen board theme.
	if name := loadThemePref(); name != "" {
		setThemeByName(name)
	}
	// Restore open tabs; their game models are built once the game list loads.
	if tabs, err := loadTabs(); err == nil {
		m.tabs = tabs
	}
	// Local (hotseat) games list independently of OGS auth.
	if lg, err := loadLocalGames(); err == nil {
		m.home.setLocalGames(lg)
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

// @region tabs:manage

// Switches to the game's tab, opening (and persisting) a new one if needed.
// Single live socket: focusing a tab dials it, leaving one disconnects it.
func (m *model) openGame(g game) tea.Cmd {
	for i, t := range m.tabs {
		if t.GameID == g.id {
			return m.switchTo(i + 1) // already open: focus it (redials if its socket dropped)
		}
	}
	// Local games (negative id) reload from disk — the saved position is
	// authoritative, and the home copy may be stale.
	var b backend
	source := "ogs"
	if g.id < 0 {
		fresh, lb, ok := openLocalGame(g.id)
		if !ok {
			return nil // record gone; nothing to open
		}
		source, g, b = "local", fresh, lb
	} else {
		b = &ogsBackend{gameID: g.id, whiteID: g.white.id, ogs: m.ogs}
	}
	m.blurActive() // disconnect the tab we're leaving before focusing the new one
	m.tabs = append(m.tabs, tabRef{Source: source, GameID: g.id})
	gm := newGameModel(len(m.games), g, b)
	gm.setSize(m.width, m.height)
	spin := gm.beginConnect()
	m.games = append(m.games, gm)
	_ = saveTabs(m.tabs)
	m.active = len(m.games)
	return tea.Batch(connectBackendCmd(b, g.id, m.events), spin)
}

// switchTo moves focus to tab index next (0 = home), tearing down the socket of
// the tab being left and dialing the one being entered if its socket dropped.
func (m *model) switchTo(next int) tea.Cmd {
	if next == m.active {
		return nil
	}
	m.blurActive()
	m.active = next
	return m.connectActive()
}

// blurActive disconnects the focused OGS socket (an intentional close) and
// cancels any reconnect ladder. Local backends are unaffected.
func (m *model) blurActive() {
	if m.active > 0 && m.active-1 < len(m.games) {
		gm := &m.games[m.active-1]
		gm.cancelReconnect()
		gm.backend.Disconnect()
	}
}

// connectActive dials the focused tab's backend if its socket isn't alive. A
// failed OGS dial enters the reconnect ladder (handled in backendConnectedMsg).
// No-op on home or an already-live/local backend.
func (m *model) connectActive() tea.Cmd {
	if m.active == 0 || m.active-1 >= len(m.games) {
		return nil
	}
	gm := &m.games[m.active-1]
	if gm.backend.isAlive() {
		return nil
	}
	spin := gm.beginConnect()
	return tea.Batch(spin, connectBackendCmd(gm.backend, gm.game.id, m.events))
}

// Closes the game tab at games-index i (active-1), reindexing the rest. A
// finished local game is deleted outright (no scoring/archive yet).
func (m *model) closeTab(i int) {
	ref := m.tabs[i]
	finished := m.games[i].game.state.finished()
	m.games[i].cancelReconnect()
	m.games[i].backend.Disconnect()
	m.games = append(m.games[:i], m.games[i+1:]...)
	m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
	for j := range m.games {
		m.games[j].idx = j
	}
	_ = saveTabs(m.tabs)
	if ref.Source == "local" && finished {
		_ = deleteLocalGame(ref.GameID)
		if lg, err := loadLocalGames(); err == nil {
			m.home.setLocalGames(lg)
		}
	}
	if m.active > len(m.games) {
		m.active = len(m.games)
	}
}

// Finds an open game by id; nil if none (id 0 is never a match).
func (m *model) gameByID(id int64) *gameModel {
	if id == 0 {
		return nil
	}
	for i := range m.games {
		if m.games[i].game.id == id {
			return &m.games[i]
		}
	}
	return nil
}

// Rebuilds game models for restored tabs once the game list is known. Launch
// lands on home and dials nothing (single live socket): OGS tabs connect only
// when focused. Local backends have no socket, so they're connected here (their
// Connect just emits the in-memory position, wiring the submit path). Drops tabs
// whose game is no longer active. No-op once tabs are already open.
func (m *model) restoreTabs(games []game) tea.Cmd {
	if len(m.games) > 0 {
		return nil
	}
	byID := make(map[int64]game, len(games))
	for _, g := range games {
		byID[g.id] = g
	}
	var kept []tabRef
	var cmds []tea.Cmd
	for _, t := range m.tabs {
		var g game
		var b backend
		if t.Source == "local" {
			lg, lb, ok := openLocalGame(t.GameID)
			if !ok {
				continue // record gone
			}
			g, b = lg, lb
		} else {
			og, ok := byID[t.GameID]
			if !ok {
				continue // no longer an active OGS game (or logged out)
			}
			g, b = og, &ogsBackend{gameID: og.id, whiteID: og.white.id, ogs: m.ogs}
		}
		kept = append(kept, t)
		gm := newGameModel(len(m.games), g, b)
		gm.setSize(m.width, m.height)
		if t.Source == "local" {
			cmds = append(cmds, connectBackendCmd(b, g.id, m.events))
		}
		m.games = append(m.games, gm)
	}
	m.tabs = kept
	_ = saveTabs(m.tabs)
	return tea.Batch(cmds...)
}

// Home tab plus one per game.
func (m model) tabCount() int {
	return len(m.games) + 1
}

// startPoll begins the always-on 20s overview poll, once. Idempotent so repeated
// logins don't stack tick loops.
func (m *model) startPoll() tea.Cmd {
	if m.polling {
		return nil
	}
	m.polling = true
	return pollTickCmd()
}

// applyPolledGames refreshes the home list and each open OGS tab's turn/phase from
// a poll. The active board keeps its socket-fed grid; only the "whose turn" meta
// is updated (the poll carries no board).
func (m *model) applyPolledGames(games []game) {
	m.home.setGames(games)
	byID := make(map[int64]game, len(games))
	for _, g := range games {
		byID[g.id] = g
	}
	for i := range m.games {
		gm := &m.games[i]
		if gm.game.id <= 0 {
			continue // local
		}
		if pg, ok := byID[gm.game.id]; ok {
			gm.game.you = pg.you
			gm.game.state.playerToMove = pg.state.playerToMove
			gm.game.state.phase = pg.state.phase
		}
	}
}

// hiddenTurnCount is the number of your-turn OGS games not open as a tab — shown
// as a yellow count beside the home icon.
func (m *model) hiddenTurnCount() int {
	open := make(map[int64]bool, len(m.games))
	for i := range m.games {
		open[m.games[i].game.id] = true
	}
	n := 0
	for _, g := range m.home.games {
		if g.yourTurn() && !open[g.id] {
			n++
		}
	}
	return n
}

// authKilled handles a dead session (a reconnect's errSessionExpired, or a poll
// 401 whose refresh failed): prune OGS tabs, keep local, land on home with a red
// banner, and clear the stored token so next launch doesn't retry a dead session.
func (m *model) authKilled() tea.Cmd {
	var keptGames []gameModel
	var keptTabs []tabRef
	for i := range m.games {
		if m.tabs[i].Source == "local" {
			keptGames = append(keptGames, m.games[i])
			keptTabs = append(keptTabs, m.tabs[i])
			continue
		}
		m.games[i].cancelReconnect()
		m.games[i].backend.Disconnect()
	}
	m.games = keptGames
	m.tabs = keptTabs
	for j := range m.games {
		m.games[j].idx = j
	}
	_ = saveTabs(m.tabs)
	m.active = 0
	_ = m.ogs.clear()
	m.ogs = ogsModel{}
	m.home.setGames(nil)
	m.home.setAuthed(false, "")
	m.home.sessionExpired = true
	// Leave m.polling set: the next (now unauthenticated) tick stops the loop and
	// clears the flag. Clearing it here could race a re-login into two loops.
	return nil
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

// Home selected a game: focus its tab, opening one if not already present.
type openGameMsg struct {
	game game
}

// Confirmed deletion of a local game from the home list.
type deleteLocalGameMsg struct {
	id int64
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
	// waitForGameEvent runs for the app's lifetime, draining socket snapshots.
	if m.authPending {
		return tea.Batch(validateStoredAuth, m.home.spinner.Tick, waitForGameEvent(m.events))
	}
	return tea.Batch(validateStoredAuth, waitForGameEvent(m.events))
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
		for i := range m.games {
			m.games[i].setSize(m.width, m.height) // re-lay each tab's chat
		}
	case authLoadedMsg:
		m.authPending = false
		m.home.setAuthPending(false)
		if msg.ok {
			m.ogs = msg.ogs
			m.home.setAuthed(true, m.ogs.Username)
			return m, tea.Batch(fetchGamesCmd(m.ogs), m.home.startLoading(), m.startPoll())
		}
		// Logged out / no stored auth: still restore local (hotseat) tabs.
		return m, m.restoreTabs(nil)
	case gamesLoadedMsg:
		m.home.setGames(msg.games)
		return m, m.restoreTabs(msg.games)
	case openGameMsg:
		return m, m.openGame(msg.game)
	case deleteLocalGameMsg:
		// Close an open tab for this game (if any), then remove it from disk and
		// refresh the home list.
		for i, t := range m.tabs {
			if t.GameID == msg.id {
				m.closeTab(i)
				break
			}
		}
		_ = deleteLocalGame(msg.id)
		if lg, err := loadLocalGames(); err == nil {
			m.home.setLocalGames(lg)
		}
		return m, nil
	case gameEvent:
		if msg.dropped {
			// Only the focused tab holds a socket, so only its drop starts the
			// ladder; a stale drop for a since-blurred tab is ignored.
			if m.active > 0 && m.games[m.active-1].game.id == msg.gameID {
				return m, tea.Batch(m.startReconnect(&m.games[m.active-1]), waitForGameEvent(m.events))
			}
			return m, waitForGameEvent(m.events)
		}
		if msg.chat != nil {
			if gm := m.gameByID(msg.gameID); gm != nil {
				gm.addChat(*msg.chat)
			}
			return m, waitForGameEvent(m.events)
		}
		if gm := m.gameByID(msg.gameID); gm != nil {
			gm.applySnapshot(msg.state)
		}
		return m, waitForGameEvent(m.events) // keep listening
	case moveResultMsg:
		if gm := m.gameByID(msg.gameID); gm != nil {
			return m, gm.applyMoveResult(msg.err)
		}
		return m, nil
	case chatSentMsg:
		// The optimistic line is already shown; the server echo confirms it.
		// Errors are non-fatal for now (see gameModel.chatSentMsg).
		return m, nil
	case backendConnectedMsg:
		if msg.err != nil {
			if gm := m.gameByID(msg.gameID); gm != nil {
				gm.connecting = false
				// A failed OGS switch-in dial enters the reconnect ladder; local
				// backends don't fail, so a rare local error just shows connectErr.
				if _, ok := gm.backend.(*ogsBackend); ok {
					return m, m.startReconnect(gm)
				}
				gm.connectErr = true
			}
		}
		return m, nil
	case reconnectTickMsg:
		gm := m.gameByID(msg.gameID)
		if gm == nil || msg.gen != gm.reconnectGen || !gm.reconnecting {
			return m, nil // tab closed, ladder canceled, or already recovered
		}
		return m, reconnectAttemptCmd(gm.backend, msg.gameID, msg.rung, msg.gen)
	case reconnectResultMsg:
		gm := m.gameByID(msg.gameID)
		if gm == nil || msg.gen != gm.reconnectGen {
			return m, nil // stale attempt
		}
		if msg.err == errSessionExpired {
			return m, m.authKilled()
		}
		if msg.err == nil {
			// Success: the emitted snapshot clears the reconnecting state; clear
			// here too in case this result lands before it.
			gm.reconnecting = false
			gm.disconnected = false
			gm.reconnectRung = 0
			return m, nil
		}
		next := msg.rung + 1
		if msg.rung < 0 || next >= len(reconnectDelays) {
			gm.reconnecting = false // manual attempt failed, or the ladder is exhausted
			gm.disconnected = true
			return m, nil
		}
		gm.reconnectRung = next
		return m, scheduleReconnect(msg.gameID, next, gm.reconnectGen)
	case pollTickMsg:
		if !m.ogs.authenticated() {
			m.polling = false // logged out / session killed: stop the loop
			return m, nil
		}
		if m.showAuth {
			return m, pollTickCmd() // paused while the login modal is open
		}
		return m, tea.Batch(pollGamesCmd(m.ogs), pollTickCmd())
	case pollResultMsg:
		if msg.authDead {
			return m, m.authKilled()
		}
		if msg.ogs != nil {
			m.ogs = *msg.ogs // adopt refreshed tokens
			for i := range m.games {
				if ob, ok := m.games[i].backend.(*ogsBackend); ok {
					ob.setAuth(m.ogs)
				}
			}
		}
		if msg.err != nil {
			return m, nil // transient; the next tick retries
		}
		m.applyPolledGames(msg.games)
		return m, nil
	case navErrorExpiredMsg:
		if msg.game >= 0 && msg.game < len(m.games) {
			var cmd tea.Cmd
			m.games[msg.game], cmd = m.games[msg.game].Update(msg)
			return m, cmd
		}
		return m, nil
	case submitOKExpiredMsg:
		if msg.game >= 0 && msg.game < len(m.games) {
			var cmd tea.Cmd
			m.games[msg.game], cmd = m.games[msg.game].Update(msg)
			return m, cmd
		}
		return m, nil
	case openSetupMsg:
		m.setup.reset()
		m.showSetup = true
		return m, nil
	case closeSetupMsg:
		m.showSetup = false
		return m, nil
	case localGameCreatedMsg:
		m.showSetup = false
		if msg.err != nil {
			return m, nil // silently drop; MVP has no create-error surface
		}
		// Refresh the home list so the new game shows, then open its tab.
		if lg, err := loadLocalGames(); err == nil {
			m.home.setLocalGames(lg)
		}
		return m, m.openGame(msg.game)
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
			return m, tea.Batch(cmd, fetchGamesCmd(m.ogs), m.home.startLoading(), m.startPoll())
		}
		return m, cmd
	case tea.KeyMsg:
		// Help is the top-most modal: any key dismisses it and is consumed.
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		// Modal captures all input; tabs and quit keys are disabled.
		if m.showAuth {
			var cmd tea.Cmd
			m.auth, cmd = m.auth.Update(msg)
			return m, cmd
		}
		if m.showSetup {
			var cmd tea.Cmd
			m.setup, cmd = m.setup.Update(msg)
			return m, cmd
		}
		// A game's "Go to" prompt captures all input the same way.
		if m.active > 0 && m.games[m.active-1].capturingInput() {
			var cmd tea.Cmd
			m.games[m.active-1], cmd = m.games[m.active-1].Update(msg)
			return m, cmd
		}
		// The home delete-confirm captures all input (incl. esc/q) until resolved.
		if m.active == 0 && m.home.confirm.active() {
			var cmd tea.Cmd
			m.home, cmd = m.home.Update(msg)
			return m, cmd
		}
		// ?: open the help modal for the active view. Placed after the input-
		// capture guards so it types normally into chat / prompts.
		if msg.String() == "?" {
			m.showHelp = true
			return m, nil
		}
		// Q (shift+q) logs out when authenticated; guarded key to avoid misfires.
		if msg.String() == "Q" && m.ogs.authenticated() {
			_ = m.ogs.clear()
			m.ogs = ogsModel{}
			m.home.setGames(nil)
			m.home.setAuthed(false, "")
			return m, nil
		}
		// X closes the focused game tab (home tab can't close). We then connect
		// whatever tab we land on (single live socket).
		if msg.String() == "X" && m.active > 0 {
			m.closeTab(m.active - 1)
			return m, m.connectActive()
		}
		// r: on a given-up game tab, retry the connection once; on home, refetch
		// the game list. Otherwise falls through to the active tab.
		if msg.String() == "r" {
			if m.active > 0 {
				if gm := &m.games[m.active-1]; gm.disconnected {
					return m, m.manualReconnect(gm)
				}
			} else if m.ogs.authenticated() {
				return m, tea.Batch(fetchGamesCmd(m.ogs), m.home.startLoading())
			}
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			for i := range m.games {
				m.games[i].backend.Disconnect() // release every open game's backend on quit
			}
			return m, tea.Quit
		case "tab", "]":
			return m, m.switchTo((m.active + 1) % m.tabCount())
		case "shift+tab", "[":
			return m, m.switchTo((m.active - 1 + m.tabCount()) % m.tabCount())
		}
		// Delegate remaining keys to the active tab.
		var cmd tea.Cmd
		if m.active == 0 {
			m.home, cmd = m.home.Update(msg)
		} else {
			m.games[m.active-1], cmd = m.games[m.active-1].Update(msg)
		}
		return m, cmd
	default:
		// Non-key messages (spinner ticks, cursor blinks) fan out to the home
		// tab and the focused game; each ignores ticks that aren't its own.
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.home, cmd = m.home.Update(msg)
		cmds = append(cmds, cmd)
		if m.active > 0 {
			m.games[m.active-1], cmd = m.games[m.active-1].Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// Longest game name shown on a tab; longer names are truncated with an ellipsis.
const maxTabNameLen = 12

// Trims s to maxTabNameLen runes, appending "…" when clipped.
func truncTabName(s string) string {
	r := []rune(s)
	if len(r) <= maxTabNameLen {
		return s
	}
	return string(r[:maxTabNameLen-1]) + "…"
}

// Top tab bar: home tab first, then one per game. A yellow ▸ prefixes a game
// tab when it's your turn; a yellow count beside the home icon counts your-turn
// games not open as tabs. The home cell is pinned; game tabs scroll so the
// active one stays visible when they overflow the terminal width.
func (m model) renderTabs() string {
	homeStyle := tabStyle
	if m.active == 0 {
		homeStyle = activeTabStyle
	}
	pinned := []string{homeStyle.Render(homeIcon)}
	if n := m.hiddenTurnCount(); n > 0 {
		pinned = append(pinned, turnMarkerStyle.Render(fmt.Sprintf(" %d", n)))
	}

	gameCells := make([]string, len(m.games))
	for i := range m.games {
		g := &m.games[i]
		style := tabStyle
		if m.active == i+1 {
			style = activeTabStyle
		}
		name := truncTabName(g.game.name)
		if g.game.yourTurn() {
			// Split the tab padding so the yellow ▸ abuts the name with nothing
			// between them: arrow takes the left pad, name keeps the right pad.
			arrow := style.PaddingRight(0).Bold(true).Foreground(lipgloss.Color("11"))
			nameStyle := style.PaddingLeft(0)
			gameCells[i] = lipgloss.JoinHorizontal(lipgloss.Top,
				arrow.Render("▸"), nameStyle.Render(name))
			continue
		}
		gameCells[i] = style.Render(name)
	}

	cells := append(pinned, m.scrollTabs(gameCells)...)
	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

// Windows game-tab cells to those that fit the remaining width, always keeping
// the active tab in view. Prepends/appends a dim ‹ / › when tabs are clipped.
func (m model) scrollTabs(gameCells []string) []string {
	if len(gameCells) == 0 {
		return gameCells
	}
	pinnedW := lipgloss.Width(homeIcon) + 4 // home cell padding
	if n := m.hiddenTurnCount(); n > 0 {
		pinnedW += lipgloss.Width(fmt.Sprintf(" %d", n))
	}
	avail := m.width - pinnedW
	const indW = 1 // width of a ‹ or › marker

	widths := make([]int, len(gameCells))
	total := 0
	for i, c := range gameCells {
		widths[i] = lipgloss.Width(c)
		total += widths[i]
	}
	if total <= avail {
		return gameCells // everything fits; no scrolling
	}

	// Active game index (m.active==0 means home is selected; window from start).
	ag := 0
	if m.active > 0 {
		ag = m.active - 1
	}

	lo, hi := ag, ag
	used := widths[ag]
	// Grow right, then left; reserve room for the markers we'll add.
	grow := func() bool {
		room := avail
		if lo > 0 {
			room -= indW
		}
		if hi < len(gameCells)-1 {
			room -= indW
		}
		if hi+1 < len(gameCells) && used+widths[hi+1] <= room {
			hi++
			used += widths[hi]
			return true
		}
		if lo-1 >= 0 && used+widths[lo-1] <= room {
			lo--
			used += widths[lo]
			return true
		}
		return false
	}
	for grow() {
	}

	marker := lipgloss.NewStyle().Foreground(dimColor)
	windowed := append([]string{}, gameCells[lo:hi+1]...)
	if lo > 0 {
		windowed = append([]string{marker.Render("‹")}, windowed...)
	}
	if hi < len(gameCells)-1 {
		windowed = append(windowed, marker.Render("›"))
	}
	return windowed
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
	if m.showSetup {
		return m.setup.View(m.width, m.height)
	}
	if m.showHelp {
		rows := homeHelpRows
		if m.active > 0 {
			rows = gameHelpRows
		}
		return helpView(rows, m.width, m.height)
	}

	tabs := m.renderTabs()
	bodyH := m.height - lipgloss.Height(tabs) - 1 // 1 blank row below the tab bar

	// Login status bar now lives in the home tab (home.View), not here.
	var body string
	if m.active == 0 {
		body = m.home.View(m.width, bodyH)
	} else {
		body = m.games[m.active-1].View(m.width, bodyH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, tabs, "", body)
}
