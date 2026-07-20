package main

import "testing"

func TestSGFCoord(t *testing.T) {
	cases := []struct {
		m    move
		want string
	}{
		{move{x: 0, y: 0}, "aa"},
		{move{x: 2, y: 1}, "cb"},
		{move{x: 8, y: 8}, "ii"},
		{move{x: -1, y: -1}, ".."}, // pass
	}
	for _, c := range cases {
		if got := sgfCoord(c.m); got != c.want {
			t.Errorf("sgfCoord(%v) = %q, want %q", c.m, got, c.want)
		}
	}
}

func TestBackendInstant(t *testing.T) {
	if !(&hotseatBackend{}).Instant() {
		t.Errorf("hotseat should be instant")
	}
	if (&ogsBackend{}).Instant() {
		t.Errorf("ogs should not be instant")
	}
}

// A confirmed remote submit flashes ✓ and schedules its dismissal; an instant
// backend does neither; a reject shows the error instead.
func TestApplyMoveResultSubmitOK(t *testing.T) {
	remote := gameModel{backend: &ogsBackend{}}
	if cmd := remote.applyMoveResult(nil); cmd == nil || !remote.submitOK {
		t.Fatalf("remote success should set submitOK and return a clear cmd")
	}

	instant := gameModel{backend: &hotseatBackend{}}
	if cmd := instant.applyMoveResult(nil); cmd != nil || instant.submitOK {
		t.Fatalf("instant success should not flash ✓")
	}

	rej := gameModel{backend: &ogsBackend{}}
	if cmd := rej.applyMoveResult(errKo); cmd != nil || rej.submitOK || rej.moveErr == "" {
		t.Fatalf("reject should surface an error, not ✓")
	}
}
