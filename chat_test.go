package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Server replays are deduped by chat_id; a pending optimistic line is upgraded.
func TestChatDedupAndUpgrade(t *testing.T) {
	c := newChatModel()

	// Optimistic local send, no id yet.
	c.addMessage(chatMessage{playerID: 1, body: "hi", channel: chatMain, pending: true})
	if len(c.messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(c.messages))
	}

	// Server echo of the same line upgrades in place (no duplicate).
	c.addMessage(chatMessage{id: "1.100", playerID: 1, body: "hi", channel: chatMain})
	if len(c.messages) != 1 {
		t.Fatalf("echo should upgrade, got %d messages", len(c.messages))
	}
	if c.messages[0].pending || c.messages[0].id != "1.100" {
		t.Fatalf("optimistic line not upgraded: %+v", c.messages[0])
	}

	// A replay of the same id is dropped.
	c.addMessage(chatMessage{id: "1.100", playerID: 1, body: "hi", channel: chatMain})
	if len(c.messages) != 1 {
		t.Fatalf("replay should dedup, got %d", len(c.messages))
	}
}

// Turn dividers appear when consecutive lines are anchored to different moves.
func TestChatTurnHeaders(t *testing.T) {
	c := newChatModel()
	c.black = player{id: 1, name: "B"}
	c.addMessage(chatMessage{id: "a", playerID: 1, body: "one", channel: chatMain, moveNumber: 10})
	c.addMessage(chatMessage{id: "b", playerID: 1, body: "two", channel: chatMain, moveNumber: 10})
	c.addMessage(chatMessage{id: "c", playerID: 1, body: "three", channel: chatMain, moveNumber: 12})

	out := c.renderLog(60)
	if strings.Count(out, "Turn 10") != 1 {
		t.Fatalf("want one Turn 10 header:\n%s", out)
	}
	if !strings.Contains(out, "Turn 12") {
		t.Fatalf("want Turn 12 header:\n%s", out)
	}
}

// Out-of-order arrivals (OGS replays aren't strictly chronological) render by
// date; a still-pending optimistic line sorts last.
func TestChatSortByDate(t *testing.T) {
	c := newChatModel()
	c.addMessage(chatMessage{id: "c", body: "third", date: 300, moveNumber: 3})
	c.addMessage(chatMessage{id: "a", body: "first", date: 100, moveNumber: 1})
	c.addMessage(chatMessage{id: "b", body: "second", date: 200, moveNumber: 2})
	c.addMessage(chatMessage{body: "typing", date: 999, moveNumber: 3, pending: true})

	got := []string{}
	for _, m := range c.messages {
		got = append(got, m.body)
	}
	want := []string{"first", "second", "third", "typing"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// A string body decodes as text; a typed body flags a variation.
func TestChatBodyDecode(t *testing.T) {
	var strPayload ogsChatPayload
	if err := json.Unmarshal([]byte(`{"channel":"main","line":{"chat_id":"x","body":"hello","player_id":7,"move_number":3}}`), &strPayload); err != nil {
		t.Fatal(err)
	}
	m := strPayload.chatMessage()
	if m.body != "hello" || m.isVariation || m.channel != chatMain {
		t.Fatalf("string body decode wrong: %+v", m)
	}

	var varPayload ogsChatPayload
	if err := json.Unmarshal([]byte(`{"channel":"malkovich","line":{"chat_id":"y","body":{"type":"analysis","name":"joseki"},"player_id":7}}`), &varPayload); err != nil {
		t.Fatal(err)
	}
	vm := varPayload.chatMessage()
	if !vm.isVariation || vm.body != "joseki" || vm.channel != chatMalkovich {
		t.Fatalf("variation body decode wrong: %+v", vm)
	}
}

// Spectator/unknown channels fold to main.
func TestChatChannelNormalize(t *testing.T) {
	cases := map[string]chatChannel{
		"main":      chatMain,
		"spectator": chatMain,
		"shadowban": chatMain,
		"malkovich": chatMalkovich,
		"personal":  chatPersonal,
	}
	for in, want := range cases {
		if got := normalizeChannel(in); got != want {
			t.Errorf("normalizeChannel(%q) = %q, want %q", in, got, want)
		}
	}
}
