package pty

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"
)

// readChunk is one observation from the PTY read goroutine.
//
// Either bytes or err is set. err == io.EOF signals the PTY closed
// (process exited or fd was closed); other errors are forwarded verbatim.
type readChunk struct {
	at    time.Time
	bytes []byte
	err   error
}

// startReadLoop spawns a goroutine that reads from r until EOF and pushes
// readChunks onto the returned channel. The channel is closed when the
// reader returns. The goroutine takes no locks; callers consume the channel
// from a single goroutine.
func startReadLoop(r io.Reader) <-chan readChunk {
	ch := make(chan readChunk, 16)
	go func() {
		defer close(ch)
		buf := make([]byte, 4<<10)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				ch <- readChunk{at: time.Now(), bytes: chunk}
			}
			if err != nil {
				ch <- readChunk{at: time.Now(), err: err}
				return
			}
		}
	}()
	return ch
}

// turnLoop is the per-Send state machine that consumes raw PTY bytes,
// emits LineEvents (ANSI-stripped, line-buffered), feeds the prompt
// detector, enforces hang/prompt timeouts, and closes the event channel
// when the prompt returns or an error condition fires.
//
// turnLoop does not hold the driver's send mutex; the caller is responsible
// for serialization. turnLoop does not own the read channel; the same
// channel is reused across Sends (the PTY read loop runs for the lifetime
// of the driver, not the turn).
type turnLoop struct {
	chunks    <-chan readChunk
	detector  PromptDetector
	effects   *slashEffectParser // nil-safe; if nil, no effect events emitted
	out       chan<- Event
	stdoutLog io.Writer

	promptTimeout time.Duration
	hangTimeout   time.Duration

	lineBuf bytes.Buffer
}

// runTurn drains chunks until the detector fires, the per-Send prompt
// timeout expires, the hang timeout expires, ctx is cancelled, or EOF is
// observed. It returns a *DriverError (or nil on prompt-return).
//
// runTurn closes nothing — the caller (Send) closes the event channel.
func (l *turnLoop) runTurn(ctx context.Context) error {
	promptDeadline := time.NewTimer(l.promptTimeout)
	defer promptDeadline.Stop()
	hangTimer := time.NewTimer(l.hangTimeout)
	defer hangTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return &DriverError{Kind: ErrPromptTimeout, Detail: "context cancelled", Cause: ctx.Err()}

		case <-promptDeadline.C:
			return &DriverError{Kind: ErrPromptTimeout, Detail: "prompt did not return before deadline"}

		case <-hangTimer.C:
			return &DriverError{Kind: ErrHang, Detail: "no output before hang deadline"}

		case chunk, ok := <-l.chunks:
			if !ok {
				return &DriverError{Kind: ErrGracefulEOF, Detail: "pty read channel closed"}
			}
			if chunk.err != nil {
				if errors.Is(chunk.err, io.EOF) {
					return &DriverError{Kind: ErrGracefulEOF, Cause: chunk.err}
				}
				return &DriverError{Kind: ErrCrash, Detail: "pty read error", Cause: chunk.err}
			}

			if l.stdoutLog != nil {
				_, _ = l.stdoutLog.Write(chunk.bytes)
			}

			// Reset the hang timer on any output.
			if !hangTimer.Stop() {
				select {
				case <-hangTimer.C:
				default:
				}
			}
			hangTimer.Reset(l.hangTimeout)

			done := l.detector.Feed(chunk.bytes)
			if l.effects != nil {
				l.effects.Feed(chunk.at, chunk.bytes)
			}
			l.emitLines(chunk.at, chunk.bytes)
			if done {
				l.flushPartialLine(chunk.at)
				return nil
			}
		}
	}
}

// emitLines ANSI-strips chunk, accumulates bytes into the partial-line
// buffer, and emits a LineEvent for each complete \n-terminated line.
func (l *turnLoop) emitLines(at time.Time, chunk []byte) {
	stripped := StripANSI(chunk)
	for _, b := range stripped {
		if b == '\n' {
			line := l.lineBuf.String()
			line = trimCR(line)
			l.out <- NewLineEvent(at, line)
			l.lineBuf.Reset()
			continue
		}
		l.lineBuf.WriteByte(b)
	}
}

// flushPartialLine emits any buffered partial-line as a LineEvent. Called
// once the detector fires so callers don't lose the final, un-newlined
// segment of a turn.
func (l *turnLoop) flushPartialLine(at time.Time) {
	if l.lineBuf.Len() == 0 {
		return
	}
	line := trimCR(l.lineBuf.String())
	l.out <- NewLineEvent(at, line)
	l.lineBuf.Reset()
}

func trimCR(s string) string {
	if n := len(s); n > 0 && s[n-1] == '\r' {
		return s[:n-1]
	}
	return s
}
