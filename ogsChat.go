package main

import "encoding/json"

// @region ogs:chat

// Game-chat decoding for the realtime socket. OGS delivers each line via a
// game/<id>/chat event as {channel, line}. On connect (with chat enabled) the
// server replays the full existing log as a burst of these events, so history
// needs no separate REST fetch. See goban ServerToClient.ts (GameChatMessage).

// game/<id>/chat payload.
type ogsChatPayload struct {
	Channel string      `json:"channel"`
	Line    ogsChatLine `json:"line"`
}

// One chat line. body is a plain string, or a typed object (analysis/review)
// for a posted variation — captured by isVariation, not decoded further yet.
type ogsChatLine struct {
	ChatID     string      `json:"chat_id"`
	Body       ogsChatBody `json:"body"`
	Date       int64       `json:"date"`
	MoveNumber int         `json:"move_number"`
	Channel    string      `json:"channel"`
	PlayerID   int64       `json:"player_id"`
	Username   string      `json:"username"`
}

// A chat body: either a string or a {type: ...} object (posted variation).
type ogsChatBody struct {
	text      string
	variation bool
}

func (b *ogsChatBody) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		b.text = s
		return nil
	}
	// Typed body (analysis/review/translated). Show the name if present,
	// otherwise a generic marker; the full variation isn't replayed yet.
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	b.variation = true
	b.text = obj.Name
	if b.text == "" {
		b.text = "◆ variation"
	}
	return nil
}

// chatMessage converts a wire line to the core model. The top-level channel
// wins (spectator posts to main come back tagged there); the line's own channel
// is the fallback.
func (p ogsChatPayload) chatMessage() chatMessage {
	ch := p.Channel
	if ch == "" {
		ch = p.Line.Channel
	}
	return chatMessage{
		id:          p.Line.ChatID,
		playerID:    p.Line.PlayerID,
		username:    p.Line.Username,
		body:        p.Line.Body.text,
		channel:     normalizeChannel(ch),
		moveNumber:  p.Line.MoveNumber,
		date:        p.Line.Date,
		isVariation: p.Line.Body.variation,
	}
}

// normalizeChannel folds OGS wire channels into our three; spectator lines read
// as main, and mod/hidden variants fall back to main too.
func normalizeChannel(c string) chatChannel {
	switch c {
	case "malkovich":
		return chatMalkovich
	case "personal":
		return chatPersonal
	default:
		return chatMain
	}
}
