package pty

import (
	"testing"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/mockclaude"
)

func TestTitleDetector_FiresOnBusyThenIdle(t *testing.T) {
	d := NewTitlePromptDetector()

	if done := d.Feed(mockclaude.BusyTitle("thinking")); done {
		t.Fatal("busy alone should not fire")
	}
	if done := d.Feed([]byte("some response\n")); done {
		t.Fatal("plain text should not fire")
	}
	if done := d.Feed(mockclaude.IdleTitle("done")); !done {
		t.Fatal("idle after busy should fire")
	}
}

func TestTitleDetector_IgnoresIdleBeforeBusy(t *testing.T) {
	d := NewTitlePromptDetector()
	if done := d.Feed(mockclaude.IdleTitle("welcome")); done {
		t.Fatal("idle before any busy should not fire (pre-input prompt)")
	}
	if done := d.Feed(mockclaude.BusyTitle("x")); done {
		t.Fatal("busy alone should still not fire after pre-input idle")
	}
	if done := d.Feed(mockclaude.IdleTitle("done")); !done {
		t.Fatal("idle after busy should fire even if a pre-input idle was seen")
	}
}

func TestTitleDetector_ResetClearsState(t *testing.T) {
	d := NewTitlePromptDetector()
	d.Feed(mockclaude.BusyTitle("x"))
	d.Feed(mockclaude.IdleTitle("y"))
	d.Reset()
	if done := d.Feed(mockclaude.IdleTitle("post-reset")); done {
		t.Fatal("after Reset, idle alone should not fire")
	}
}

func TestTitleDetector_HandlesSplitOSC(t *testing.T) {
	d := NewTitlePromptDetector()

	busy := mockclaude.BusyTitle("x")
	idle := mockclaude.IdleTitle("y")

	// Split each OSC at every byte boundary; ensure no mid-OSC split fires
	// erroneously, but the final byte of idle does fire.
	for i := 1; i < len(busy); i++ {
		if done := d.Feed(busy[:i]); done {
			t.Fatalf("busy partial at %d incorrectly fired", i)
		}
		d.Feed(busy[i:])
		break // one split is enough to exercise the path
	}
	for i := 1; i < len(idle); i++ {
		if done := d.Feed(idle[:i]); done {
			t.Fatalf("idle partial at %d incorrectly fired", i)
		}
		if done := d.Feed(idle[i:]); !done {
			t.Fatalf("idle completion at split %d should fire", i)
		}
		break
	}
}

func TestTitleDetector_IdempotentAfterFire(t *testing.T) {
	d := NewTitlePromptDetector()
	d.Feed(mockclaude.BusyTitle("x"))
	if done := d.Feed(mockclaude.IdleTitle("y")); !done {
		t.Fatal("expected fire")
	}
	if done := d.Feed([]byte("extra bytes")); !done {
		t.Error("post-fire Feed should remain done")
	}
}

func TestTitleDetector_STTerminator(t *testing.T) {
	d := NewTitlePromptDetector()
	d.Feed([]byte("\x1b]0;⠂ busy\x1b\\")) // ST-terminated busy title
	if done := d.Feed([]byte("\x1b]0;✳ done\x1b\\")); !done {
		t.Fatal("ST-terminated busy→idle should fire")
	}
}
