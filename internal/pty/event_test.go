package pty

import (
	"testing"
	"time"
)

func TestEventInterfaceSatisfaction(t *testing.T) {
	ts := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	events := []Event{
		NewLineEvent(ts, "hello"),
		NewCompactStart(ts),
		NewCompactEnd(ts),
		NewCompactSummaryAvailable(ts, "/tmp/session.jsonl"),
		NewCleared(ts),
		NewModelChanged(ts, "claude-opus-4-7"),
		NewEffortChanged(ts, "high"),
		NewSessionExiting(ts, "user requested /exit"),
	}
	for i, e := range events {
		if !e.At().Equal(ts) {
			t.Errorf("event %d: At() = %v, want %v", i, e.At(), ts)
		}
	}
}

func TestEventTypeSwitch(t *testing.T) {
	var seen []string
	for _, e := range []Event{
		NewLineEvent(time.Time{}, "x"),
		NewCompactStart(time.Time{}),
		NewCompactEnd(time.Time{}),
		NewCompactSummaryAvailable(time.Time{}, "p"),
		NewCleared(time.Time{}),
		NewModelChanged(time.Time{}, "m"),
		NewEffortChanged(time.Time{}, "low"),
		NewSessionExiting(time.Time{}, ""),
	} {
		switch e.(type) {
		case LineEvent:
			seen = append(seen, "line")
		case CompactStart:
			seen = append(seen, "compact-start")
		case CompactEnd:
			seen = append(seen, "compact-end")
		case CompactSummaryAvailable:
			seen = append(seen, "compact-summary-available")
		case Cleared:
			seen = append(seen, "cleared")
		case ModelChanged:
			seen = append(seen, "model-changed")
		case EffortChanged:
			seen = append(seen, "effort-changed")
		case SessionExiting:
			seen = append(seen, "session-exiting")
		default:
			t.Errorf("unknown event type %T", e)
		}
	}
	want := []string{"line", "compact-start", "compact-end", "compact-summary-available", "cleared", "model-changed", "effort-changed", "session-exiting"}
	if len(seen) != len(want) {
		t.Fatalf("seen %v, want %v", seen, want)
	}
	for i := range seen {
		if seen[i] != want[i] {
			t.Errorf("seen[%d] = %q, want %q", i, seen[i], want[i])
		}
	}
}

func TestEventBaseDefaultsToNow(t *testing.T) {
	before := time.Now()
	e := NewLineEvent(time.Time{}, "x")
	after := time.Now()
	if e.At().Before(before) || e.At().After(after) {
		t.Errorf("At() = %v not within [%v, %v]", e.At(), before, after)
	}
}
