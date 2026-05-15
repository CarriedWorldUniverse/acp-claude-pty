package pty

import (
	"testing"
	"time"
)

// drainEvents consumes everything currently available on out without
// blocking.
func drainEvents(out chan Event) []Event {
	close(out)
	var evs []Event
	for e := range out {
		evs = append(evs, e)
	}
	return evs
}

func TestSlashEffect_ModelChanged(t *testing.T) {
	out := make(chan Event, 8)
	p := newSlashEffectParser(out)
	p.Feed(time.Now(), []byte("⎿  Set model to Sonnet 4.6 with medium effort · Claude Max\n"))
	evs := drainEvents(out)

	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(evs), evs)
	}
	mc, ok := evs[0].(ModelChanged)
	if !ok {
		t.Fatalf("expected ModelChanged, got %T", evs[0])
	}
	if mc.Model == "" {
		t.Errorf("Model is empty")
	}
}

func TestSlashEffect_EffortChanged(t *testing.T) {
	out := make(chan Event, 8)
	p := newSlashEffectParser(out)
	p.Feed(time.Now(), []byte("⎿  Set effort level to high: Comprehensive implementation with extensive testing and documentation\n"))
	evs := drainEvents(out)

	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(evs), evs)
	}
	ec, ok := evs[0].(EffortChanged)
	if !ok {
		t.Fatalf("expected EffortChanged, got %T", evs[0])
	}
	if ec.Level != "high" {
		t.Errorf("Level = %q, want %q", ec.Level, "high")
	}
}

func TestSlashEffect_CompactStart(t *testing.T) {
	out := make(chan Event, 8)
	p := newSlashEffectParser(out)
	p.Feed(time.Now(), []byte("✳ Compacting conversation…\n"))
	evs := drainEvents(out)

	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if _, ok := evs[0].(CompactStart); !ok {
		t.Fatalf("expected CompactStart, got %T", evs[0])
	}
}

func TestSlashEffect_Cleared_FromOSCTitle(t *testing.T) {
	out := make(chan Event, 8)
	p := newSlashEffectParser(out)
	p.Feed(time.Now(), []byte("\x1b]0;✳ Claude Code\x07"))
	evs := drainEvents(out)

	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if _, ok := evs[0].(Cleared); !ok {
		t.Fatalf("expected Cleared, got %T", evs[0])
	}
}

func TestSlashEffect_Cleared_NotFiredOnConversationSummaryTitle(t *testing.T) {
	out := make(chan Event, 8)
	p := newSlashEffectParser(out)
	p.Feed(time.Now(), []byte("\x1b]0;✳ Previous conversation summary text\x07"))
	evs := drainEvents(out)

	if len(evs) != 0 {
		t.Errorf("expected no events for non-default title, got %+v", evs)
	}
}

func TestSlashEffect_IgnoresUnrelatedLines(t *testing.T) {
	out := make(chan Event, 8)
	p := newSlashEffectParser(out)
	p.Feed(time.Now(), []byte("ordinary output line\n"))
	p.Feed(time.Now(), []byte("⎿  Some other UI continuation\n"))
	evs := drainEvents(out)

	if len(evs) != 0 {
		t.Errorf("expected no events for unrelated content, got %+v", evs)
	}
}

func TestSlashEffect_TitleAcrossSplitChunks(t *testing.T) {
	out := make(chan Event, 8)
	p := newSlashEffectParser(out)
	// Split OSC across two Feed calls.
	full := []byte("\x1b]0;✳ Claude Code\x07")
	mid := len(full) / 2
	p.Feed(time.Now(), full[:mid])
	p.Feed(time.Now(), full[mid:])
	evs := drainEvents(out)

	if len(evs) != 1 {
		t.Fatalf("expected 1 event across split chunks, got %d", len(evs))
	}
	if _, ok := evs[0].(Cleared); !ok {
		t.Fatalf("expected Cleared, got %T", evs[0])
	}
}
