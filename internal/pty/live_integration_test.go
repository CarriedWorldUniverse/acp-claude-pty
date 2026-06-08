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
	for ev := range turn.Events {
		if le, ok := ev.(LineEvent); ok {
			out.WriteString(le.Line)
			out.WriteByte('\n')
		}
	}
	got := out.String()
	t.Logf("=== live turn output ===\n%s========================", got)

	if err := turn.Err(); err != nil {
		t.Fatalf("turn terminated with error: %v\noutput so far:\n%s", err, got)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("turn completed but emitted no output lines")
	}
	if !strings.Contains(strings.ToUpper(got), "PONG") {
		t.Logf("note: output did not contain PONG — claude may have formatted differently, but a turn completed cleanly")
	}
}
