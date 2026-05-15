package acpserver

import (
	"testing"
	"time"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/pty"
	acp "github.com/coder/acp-go-sdk"
)

func TestTextFromBlocks_ConcatenatesTextBlocks(t *testing.T) {
	blocks := []acp.ContentBlock{
		acp.TextBlock("hello"),
		acp.TextBlock("world"),
	}
	got := textFromBlocks(blocks)
	if got != "hello world" {
		t.Errorf("textFromBlocks = %q, want %q", got, "hello world")
	}
}

func TestTextFromBlocks_IgnoresNonText(t *testing.T) {
	blocks := []acp.ContentBlock{
		acp.TextBlock("alpha"),
		acp.ImageBlock("base64data", "image/png"),
		acp.TextBlock("beta"),
	}
	got := textFromBlocks(blocks)
	if got != "alpha beta" {
		t.Errorf("textFromBlocks = %q, want %q (non-text blocks should be skipped)", got, "alpha beta")
	}
}

func TestStopReasonFor_MapsKindsToACP(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want acp.StopReason
	}{
		{"nil", nil, acp.StopReasonEndTurn},
		{"prompt-timeout", &pty.DriverError{Kind: pty.ErrPromptTimeout}, acp.StopReasonCancelled},
		{"hang", &pty.DriverError{Kind: pty.ErrHang}, acp.StopReasonCancelled},
		{"graceful-eof", &pty.DriverError{Kind: pty.ErrGracefulEOF}, acp.StopReasonEndTurn},
		{"aborted-by-sigterm", &pty.DriverError{Kind: pty.ErrAbortedBySIGTERM}, acp.StopReasonEndTurn},
		{"crash", &pty.DriverError{Kind: pty.ErrCrash}, acp.StopReasonCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stopReasonFor(tc.err); got != tc.want {
				t.Errorf("stopReasonFor(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestFormatEffect_AllVariants(t *testing.T) {
	ts := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		ev   pty.Event
		want string
	}{
		{"compact-start", pty.NewCompactStart(ts), "[compact-start]"},
		{"compact-end", pty.NewCompactEnd(ts), "[compact-end]"},
		{"compact-summary-available", pty.NewCompactSummaryAvailable(ts, "/tmp/s.jsonl"), "[compact-summary-available] /tmp/s.jsonl"},
		{"cleared", pty.NewCleared(ts), "[cleared]"},
		{"model-changed", pty.NewModelChanged(ts, "claude-opus-4-7"), "[model-changed] claude-opus-4-7"},
		{"effort-changed", pty.NewEffortChanged(ts, "high"), "[effort-changed] high"},
		{"session-exiting", pty.NewSessionExiting(ts, "user-requested"), "[session-exiting] user-requested"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatEffect(tc.ev); got != tc.want {
				t.Errorf("formatEffect = %q, want %q", got, tc.want)
			}
		})
	}
}
