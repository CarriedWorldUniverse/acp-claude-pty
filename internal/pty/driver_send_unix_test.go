//go:build !windows

package pty

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildMockClaude compiles cmd/mockclaude into a t.TempDir and returns the
// absolute path to the resulting binary.
func buildMockClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "mockclaude")

	pkg := "github.com/CarriedWorldUniverse/acp-claude-pty/cmd/mockclaude"
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build mockclaude: %v\n%s", err, out)
	}
	return bin
}

func TestSend_EndToEnd_AgainstMockClaude(t *testing.T) {
	bin := buildMockClaude(t)

	d, err := New(Options{
		Command:       bin,
		Cwd:           t.TempDir(),
		Env:           append(os.Environ(), "MOCKCLAUDE_DEFAULT=hello back\n"),
		PromptTimeout: 3 * time.Second,
		HangTimeout:   1 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	turn, err := d.Send(ctx, "say hi")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var lines []string
	for ev := range turn.Events {
		if l, ok := ev.(LineEvent); ok {
			lines = append(lines, l.Line)
		}
	}
	if err := turn.Err(); err != nil {
		t.Fatalf("Turn.Err: %v", err)
	}

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "hello back") {
		t.Errorf("expected 'hello back' in output, got:\n%s", joined)
	}
}

func TestSend_BeforeStart_ReturnsCrash(t *testing.T) {
	d, err := New(Options{Command: "/usr/bin/true", Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = d.Send(context.Background(), "x")
	var de *DriverError
	if !errors.As(err, &de) {
		t.Fatalf("expected *DriverError, got %T: %v", err, err)
	}
	if de.Kind != ErrCrash {
		t.Errorf("Kind = %v, want ErrCrash", de.Kind)
	}
}

func TestRestart_PreservesSendability(t *testing.T) {
	bin := buildMockClaude(t)

	d, err := New(Options{
		Command:       bin,
		Cwd:           t.TempDir(),
		Env:           append(os.Environ(), "MOCKCLAUDE_DEFAULT=after-restart\n"),
		PromptTimeout: 3 * time.Second,
		HangTimeout:   1 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	if err := d.Restart(ctx); err != nil {
		t.Fatalf("Restart: %v", err)
	}

	turn, err := d.Send(ctx, "ping")
	if err != nil {
		t.Fatalf("Send post-Restart: %v", err)
	}
	var sawResponse bool
	for ev := range turn.Events {
		if l, ok := ev.(LineEvent); ok && strings.Contains(l.Line, "after-restart") {
			sawResponse = true
		}
	}
	if err := turn.Err(); err != nil {
		t.Fatalf("Turn.Err post-Restart: %v", err)
	}
	if !sawResponse {
		t.Error("expected 'after-restart' response after Restart")
	}
}

func TestSend_TwoSequentialTurns(t *testing.T) {
	bin := buildMockClaude(t)

	d, err := New(Options{
		Command:       bin,
		Cwd:           t.TempDir(),
		Env:           append(os.Environ(), "MOCKCLAUDE_DEFAULT=response\n"),
		PromptTimeout: 3 * time.Second,
		HangTimeout:   1 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	for i := 0; i < 2; i++ {
		turn, err := d.Send(ctx, "msg")
		if err != nil {
			t.Fatalf("Send #%d: %v", i, err)
		}
		var sawResponse bool
		for ev := range turn.Events {
			if l, ok := ev.(LineEvent); ok && strings.Contains(l.Line, "response") {
				sawResponse = true
			}
		}
		if err := turn.Err(); err != nil {
			t.Fatalf("Turn.Err #%d: %v", i, err)
		}
		if !sawResponse {
			t.Errorf("turn #%d: missing 'response' in output", i)
		}
	}
}
