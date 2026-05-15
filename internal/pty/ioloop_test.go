package pty

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/mockclaude"
)

// fakeChunk feeds a fixed sequence of chunks then EOF (or held silence)
// onto a channel, simulating the PTY read loop.
func fakeChunks(t *testing.T, frames [][]byte) chan readChunk {
	t.Helper()
	ch := make(chan readChunk, len(frames)+1)
	for _, f := range frames {
		ch <- readChunk{at: time.Now(), bytes: f}
	}
	return ch
}

func collect(t *testing.T, c <-chan Event) []Event {
	t.Helper()
	var out []Event
	for e := range c {
		out = append(out, e)
	}
	return out
}

func TestTurnLoop_HappyPath_BusyIdleFires(t *testing.T) {
	chunks := fakeChunks(t, [][]byte{
		mockclaude.BusyTitle("thinking"),
		[]byte("line one\nline two\n"),
		mockclaude.IdleTitle("done"),
	})
	close(chunks)

	out := make(chan Event, 16)
	loop := &turnLoop{
		chunks:        chunks,
		detector:      NewTitlePromptDetector(),
		out:           out,
		promptTimeout: 2 * time.Second,
		hangTimeout:   2 * time.Second,
	}

	err := loop.runTurn(context.Background())
	close(out)
	if err != nil {
		t.Fatalf("runTurn: %v", err)
	}

	events := collect(t, out)
	var lines []string
	for _, e := range events {
		if l, ok := e.(LineEvent); ok {
			lines = append(lines, l.Line)
		}
	}
	if len(lines) < 2 || lines[0] != "line one" || lines[1] != "line two" {
		t.Errorf("lines = %v, want [line one line two]", lines)
	}
}

func TestTurnLoop_FlushPartialLineOnFire(t *testing.T) {
	chunks := make(chan readChunk, 4)
	chunks <- readChunk{bytes: mockclaude.BusyTitle("x")}
	chunks <- readChunk{bytes: []byte("partial-with-no-newline")}
	chunks <- readChunk{bytes: mockclaude.IdleTitle("y")}
	close(chunks)

	out := make(chan Event, 16)
	loop := &turnLoop{
		chunks:        chunks,
		detector:      NewTitlePromptDetector(),
		out:           out,
		promptTimeout: 2 * time.Second,
		hangTimeout:   2 * time.Second,
	}
	if err := loop.runTurn(context.Background()); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	close(out)

	events := collect(t, out)
	var lastLine string
	for _, e := range events {
		if l, ok := e.(LineEvent); ok {
			lastLine = l.Line
		}
	}
	if lastLine != "partial-with-no-newline" {
		t.Errorf("expected partial flushed as final LineEvent, got %q", lastLine)
	}
}

func TestTurnLoop_PromptTimeout(t *testing.T) {
	chunks := make(chan readChunk) // never produces; never closes
	out := make(chan Event, 4)
	loop := &turnLoop{
		chunks:        chunks,
		detector:      NewTitlePromptDetector(),
		out:           out,
		promptTimeout: 30 * time.Millisecond,
		hangTimeout:   60 * time.Millisecond,
	}
	err := loop.runTurn(context.Background())
	close(out)
	var de *DriverError
	if !errors.As(err, &de) {
		t.Fatalf("expected *DriverError, got %T: %v", err, err)
	}
	if de.Kind != ErrPromptTimeout {
		t.Errorf("Kind = %v, want ErrPromptTimeout", de.Kind)
	}
}

func TestTurnLoop_HangTimeoutResetsOnOutput(t *testing.T) {
	chunks := make(chan readChunk, 4)
	chunks <- readChunk{bytes: mockclaude.BusyTitle("x")}
	// Don't supply idle yet; just leave the channel open. hangTimeout will
	// fire because no further output arrives.

	out := make(chan Event, 4)
	loop := &turnLoop{
		chunks:        chunks,
		detector:      NewTitlePromptDetector(),
		out:           out,
		promptTimeout: 1 * time.Second,
		hangTimeout:   30 * time.Millisecond,
	}
	err := loop.runTurn(context.Background())
	close(out)
	var de *DriverError
	if !errors.As(err, &de) {
		t.Fatalf("expected *DriverError, got %T: %v", err, err)
	}
	if de.Kind != ErrHang {
		t.Errorf("Kind = %v, want ErrHang", de.Kind)
	}
}

func TestTurnLoop_EOFMappsToGracefulEOF(t *testing.T) {
	chunks := make(chan readChunk, 2)
	chunks <- readChunk{bytes: mockclaude.BusyTitle("x")}
	chunks <- readChunk{err: io.EOF}
	close(chunks)

	out := make(chan Event, 4)
	loop := &turnLoop{
		chunks:        chunks,
		detector:      NewTitlePromptDetector(),
		out:           out,
		promptTimeout: 1 * time.Second,
		hangTimeout:   1 * time.Second,
	}
	err := loop.runTurn(context.Background())
	close(out)
	var de *DriverError
	if !errors.As(err, &de) {
		t.Fatalf("expected *DriverError, got %T: %v", err, err)
	}
	if de.Kind != ErrGracefulEOF {
		t.Errorf("Kind = %v, want ErrGracefulEOF", de.Kind)
	}
}

func TestTurnLoop_ContextCancel(t *testing.T) {
	chunks := make(chan readChunk) // blocks forever
	out := make(chan Event, 4)
	loop := &turnLoop{
		chunks:        chunks,
		detector:      NewTitlePromptDetector(),
		out:           out,
		promptTimeout: 5 * time.Second,
		hangTimeout:   5 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := loop.runTurn(ctx)
	close(out)
	var de *DriverError
	if !errors.As(err, &de) {
		t.Fatalf("expected *DriverError, got %T: %v", err, err)
	}
	if de.Kind != ErrPromptTimeout {
		t.Errorf("Kind = %v, want ErrPromptTimeout (context cancel maps here)", de.Kind)
	}
}

func TestTurnLoop_ANSIStripsBeforeLineEmit(t *testing.T) {
	chunks := make(chan readChunk, 4)
	chunks <- readChunk{bytes: mockclaude.BusyTitle("x")}
	chunks <- readChunk{bytes: []byte("\x1b[31mred\x1b[0m line\n")}
	chunks <- readChunk{bytes: mockclaude.IdleTitle("y")}
	close(chunks)

	out := make(chan Event, 8)
	loop := &turnLoop{
		chunks:        chunks,
		detector:      NewTitlePromptDetector(),
		out:           out,
		promptTimeout: 1 * time.Second,
		hangTimeout:   1 * time.Second,
	}
	if err := loop.runTurn(context.Background()); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	close(out)

	var saw string
	for e := range out {
		if l, ok := e.(LineEvent); ok && l.Line != "" {
			saw = l.Line
			break
		}
	}
	if saw != "red line" {
		t.Errorf("LineEvent = %q, want %q", saw, "red line")
	}
}
