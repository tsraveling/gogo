package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// @region chat:model

// The game chat: a scrolling log above a fixed two-row composer. Messages arrive
// via the backend (emitChat → gameEvent) and are deduped by id. Sending is OGS-
// only (canChat); local games render the log area with a disabled composer.
type chatModel struct {
	vp       viewport.Model
	ta       textarea.Model
	messages []chatMessage
	seen     map[string]bool // chat_ids already in the log (dedup replays)

	focused     bool
	mode        chatChannel // composer channel; tab cycles
	canChat     bool        // backend supports sending
	scrollStick bool        // jump to bottom on next refresh (new message)

	// Player identity, for name-tag coloring.
	black player
	white player
}

func newChatModel() chatModel {
	ta := textarea.New()
	ta.Placeholder = "Hit ENTER to chat"
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 400
	ta.SetHeight(2)
	// Enter is intercepted for send; keep the textarea single-purpose.
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(0, 0)
	return chatModel{
		vp:   vp,
		ta:   ta,
		seen: map[string]bool{},
		mode: chatMain,
	}
}

// setContext adopts the game's players and whether chat can be sent.
func (c *chatModel) setContext(g game, canChat bool) {
	c.black = g.black
	c.white = g.white
	c.canChat = canChat
}

// addMessage merges an incoming line, deduping server replays by chat_id and
// upgrading a matching optimistic (pending) line to its confirmed form. The log
// is kept in chronological order (by date); optimistic lines with no server
// timestamp yet sort last.
func (c *chatModel) addMessage(m chatMessage) {
	if m.id != "" && c.seen[m.id] {
		return
	}
	if m.id != "" {
		for i := range c.messages {
			om := &c.messages[i]
			if om.pending && om.playerID == m.playerID && om.body == m.body && om.channel == m.channel {
				*om = m
				c.seen[m.id] = true
				c.sortMessages()
				c.scrollStick = true
				return
			}
		}
		c.seen[m.id] = true
	}
	c.messages = append(c.messages, m)
	c.sortMessages()
	c.scrollStick = true
}

// sortMessages orders the log chronologically by date; pending (un-echoed) lines
// have no server timestamp so they're held at the end.
func (c *chatModel) sortMessages() {
	sort.SliceStable(c.messages, func(i, j int) bool {
		a, b := c.messages[i], c.messages[j]
		if a.pending != b.pending {
			return !a.pending // confirmed lines before pending ones
		}
		return a.date < b.date
	})
}

// cycleMode advances the composer channel (main → malkovich → personal → main).
func (c *chatModel) cycleMode() {
	for i, ch := range chatModes {
		if ch == c.mode {
			c.mode = chatModes[(i+1)%len(chatModes)]
			return
		}
	}
	c.mode = chatMain
}

func (c *chatModel) focus() tea.Cmd {
	c.focused = true
	return c.ta.Focus()
}

func (c *chatModel) blur() {
	c.focused = false
	c.ta.Blur()
	c.ta.Reset()
}

func (c *chatModel) scrollUp()   { c.vp.ScrollUp(1) }
func (c *chatModel) scrollDown() { c.vp.ScrollDown(1) }

// Update feeds async messages (textarea cursor blink) while focused.
func (c chatModel) Update(msg tea.Msg) (chatModel, tea.Cmd) {
	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(msg)
	return c, cmd
}

// composerHeight is the fixed bottom block: 2-row textarea + 1 mode row.
const composerHeight = 3

// chatTopMargin is the blank row between the info box and the chat log.
const chatTopMargin = 1

// refresh recomputes the viewport size/content for the given panel dimensions,
// applying a pending scroll-to-bottom. Called from the update path so the value-
// receiver View can stay a pure render.
func (c *chatModel) refresh(w, h int) {
	contentW := max(w-2, 1) // 2-space right margin
	vpH := max(h-chatTopMargin-composerHeight, 1)
	c.vp.Width = contentW
	c.vp.Height = vpH
	c.vp.SetContent(c.renderLog(contentW))
	if c.scrollStick {
		c.vp.GotoBottom()
		c.scrollStick = false
	}
	c.ta.SetWidth(contentW)
}

// renderLog builds the scrollback: turn dividers between move groups, then each
// line as a colored name tag plus body.
func (c chatModel) renderLog(width int) string {
	if len(c.messages) == 0 {
		return dimStyle.Render("No messages yet.")
	}
	var b strings.Builder
	prevTurn := -1
	for i, m := range c.messages {
		if m.moveNumber != prevTurn {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(chatTurnStyle.Render(fmt.Sprintf("--- Turn %d ---", m.moveNumber)))
			b.WriteByte('\n')
			prevTurn = m.moveNumber
		}
		b.WriteString(c.renderLine(m, width))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func (c chatModel) renderLine(m chatMessage, width int) string {
	body := channelStyle(m.channel).Render(m.body)
	if m.isVariation {
		body = chatVariationStyle.Render(m.body)
	}
	line := c.nameTag(m) + " " + body
	return lipgloss.NewStyle().Width(width).Render(line)
}

// nameTag renders the author as a black/white bar (matching the info panel), or
// a dim label for a spectator.
func (c chatModel) nameTag(m chatMessage) string {
	name := m.username
	if name == "" {
		name = "?"
	}
	switch m.playerID {
	case c.black.id:
		return infoBlackStyle.Render(" " + name + " ")
	case c.white.id:
		return infoWhiteStyle.Render(" " + name + " ")
	default:
		return chatSpectatorStyle.Render(name)
	}
}

// channelStyle maps a channel to its body/label color.
func channelStyle(ch chatChannel) lipgloss.Style {
	switch ch {
	case chatMalkovich:
		return chatMalkovichStyle
	case chatPersonal:
		return chatPersonalStyle
	default:
		return chatMainStyle
	}
}

// @region chat:view

func (c chatModel) View(w, h int) string {
	log := c.vp.View()
	var composer string
	if c.canChat {
		composer = lipgloss.JoinVertical(lipgloss.Left, c.ta.View(), c.modeRow())
	} else {
		composer = lipgloss.NewStyle().Height(composerHeight).Render(
			dimStyle.Render("Chat unavailable for this game."))
	}
	return lipgloss.JoinVertical(lipgloss.Left, "", log, composer)
}

// modeRow shows the three channels, the active one emphasized in its color.
func (c chatModel) modeRow() string {
	cells := make([]string, 0, len(chatModes))
	for _, ch := range chatModes {
		label := string(ch)
		if ch == c.mode {
			cells = append(cells, channelStyle(ch).Bold(true).Underline(true).Render(label))
		} else {
			cells = append(cells, chatModeIdleStyle.Render(label))
		}
	}
	prefix := chatModeIdleStyle.Render("⇥ ")
	return prefix + strings.Join(cells, chatModeIdleStyle.Render("  "))
}
