//go:build integration

package pty

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/spawndir"
)

// TestLive_RealClaude_Turn drives the REAL claude CLI through the PTY driver
// end-to-end: spawn → one prompt → prompt-return → captured output. This is
// the live-binary integration test from NEX-84's acceptance — it exercises the
// title-bar detector against real claude's OSC sequence, which mockclaude only
// approximates (the brittle frontier the prior JS proxy died on).
//
// Gated behind -tags=integration + a claude on PATH, so default CI is unaffected.
//
//	go test -tags=integration ./internal/pty/ -run TestLive_RealClaude_Turn -v
func TestLive_RealClaude_Turn(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not on PATH; skipping live integration test")
	}

	sd, err := spawndir.Materialize("", spawndir.Spec{})
	if err != nil {
		t.Fatalf("spawndir.Materialize: %v", err)
	}
	t.Cleanup(func() { _ = sd.Cleanup() })

	d, err := New(Options{
		Command: "claude",
		Cwd:     sd.Path,
		// A fresh spawn dir triggers claude's folder-trust dialog, which wedges
		// the first turn (no session flag skips it; the prompt Send can't
		// answer the menu). AcceptWorkspaceTrust auto-accepts it at Start.
		AcceptWorkspaceTrust: true,
		PromptTimeout:        90 * time.Second,
		HangTimeout:          30 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start (spawn real claude in PTY): %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	turn, err := d.Send(ctx, "Reply with exactly the single word: PONG")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var out strings.Builder
	var assistant string
	for ev := range turn.Events {
		switch e := ev.(type) {
		case LineEvent:
			out.WriteString(e.Line)
			out.WriteByte('\n')
		case AssistantMessage:
			assistant = e.Text
		}
	}
	t.Logf("=== raw TUI lines ===\n%s========================", out.String())
	t.Logf("=== clean assistant text (JSONL) ===\n%s\n====================================", assistant)

	if err := turn.Err(); err != nil {
		t.Fatalf("turn terminated with error: %v\noutput so far:\n%s", err, out.String())
	}

	// The JSONL-sourced AssistantMessage is the clean content channel: it must
	// carry the answer and none of the TUI chrome the raw line stream has.
	if strings.TrimSpace(assistant) == "" {
		t.Fatal("no AssistantMessage emitted — JSONL-sourced turn text missing")
	}
	if !strings.Contains(strings.ToUpper(assistant), "PONG") {
		t.Errorf("clean assistant text missing PONG: %q", assistant)
	}
	for _, chrome := range []string{"Pondering", "esc to interrupt", "[cleared]", "tokens)", "running sp hook"} {
		if strings.Contains(assistant, chrome) {
			t.Errorf("clean assistant text leaked TUI chrome %q: %q", chrome, assistant)
		}
	}
}
