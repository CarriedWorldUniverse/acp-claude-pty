// Package mockclaude provides a fake claude-code CLI for deterministic
// driver tests.
//
// The mock reproduces the byte-level behaviours the real claude TUI exhibits
// that the driver depends on:
//
//   - OSC title-bar emissions with leading state glyph: braille block for
//     "busy", U+2733 (✳) for "idle". Busy→idle transition signals prompt
//     return.
//   - Cooked-for-Ns confirmation line (Tier 2 detector hint).
//   - Slash-command effect lines (/clear, /compact, /model, /effort, /exit).
//   - Scriptable exit modes: clean, sigterm-with-resume-hint, model-invalid.
//
// The mock reads keyboard-style input from stdin (terminated by '\r', CR,
// matching real claude-code's raw-mode TTY behaviour — see
// pty.Driver.Send) and writes response bytes to stdout. '\r\n' is
// tolerated; bare '\n' does not commit a turn, mirroring the real binary.
// It is intentionally a Go library so the same in-memory script can be
// driven by both the integration tests (via a PTY) and the parser/replay
// tests (via a bytes buffer).
package mockclaude

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// Script is the deterministic playbook the mock follows. A Script is keyed
// off the input line claude receives; on each match, the listed Responses
// are emitted in order with the listed inter-response Delays.
//
// Unmatched input lines invoke the Default response. If Default is empty
// the mock emits a generic warm-send response.
type Script struct {
	// Init bytes emitted once before any input is read. Typically the
	// initial idle title + welcome banner.
	Init []byte

	// Matches is consulted in order; the first prefix-match wins.
	Matches []Match

	// Default is used when no Match applies.
	Default []byte

	// Cook indicates the busy-time the mock simulates between Init and
	// idle-title emission per turn. The driver should observe busy titles
	// during this window.
	Cook time.Duration

	// Exit, if set, causes Run to return after Matches consumed. The
	// caller is then responsible for translating to the real process exit
	// shape (mock as library leaves the OS-level signal to the caller).
	Exit ExitMode
}

// Match associates an input prefix with a deterministic response.
type Match struct {
	InputPrefix string
	Response    []byte
}

// ExitMode controls what bytes the mock writes when its script is exhausted.
type ExitMode int

const (
	// ExitClean: emit no extra bytes, return.
	ExitClean ExitMode = iota
	// ExitSIGTERM: emit the resume-hint tail bytes (literal "Resume this
	// session with: ...") and return. The OS-level exit code shaping is
	// the caller's responsibility.
	ExitSIGTERM
	// ExitModelInvalid: emit a model-not-available line on the first
	// turn instead of a normal response.
	ExitModelInvalid
)

// SIGTERMResumeHint is the literal byte pattern claude prints to its TTY
// when it receives SIGTERM, before exiting with code 143. The driver's
// SIGTERM-detection logic matches against this prefix.
const SIGTERMResumeHint = "\nResume this session with:\n\nclaude --resume "

// Run drives the mock against an input stream (typically the PTY slave's
// stdin) and an output writer (the PTY slave's stdout). It returns when
// stdin is exhausted, when the script's Exit fires, or when err != nil.
//
// Run is intentionally synchronous and blocking. Callers that need
// concurrent reads/writes (e.g. a real PTY) should run it on a goroutine.
func Run(in io.Reader, out io.Writer, script Script) error {
	if _, err := out.Write(script.Init); err != nil {
		return err
	}

	reader := bufio.NewScanner(in)
	reader.Buffer(make([]byte, 0, 64<<10), 1<<20)
	// Mirror real claude-code: commit on '\r' (CR, 0x0D), not '\n'.
	// '\r\n' is tolerated — the trailing LF is stripped from the next read.
	reader.Split(scanCR)

	firstTurn := true
	for reader.Scan() {
		input := strings.TrimPrefix(reader.Text(), "\n")
		if input == "" {
			continue
		}

		// Model-invalid on first turn replaces the normal response.
		if firstTurn && script.Exit == ExitModelInvalid {
			if _, err := out.Write([]byte("Error: model 'invalid-model-name' is not available\n")); err != nil {
				return err
			}
			firstTurn = false
			continue
		}
		firstTurn = false

		response := matchResponse(script, input)
		if err := emitTurn(out, response, script.Cook); err != nil {
			return err
		}
	}
	if err := reader.Err(); err != nil && err != io.EOF {
		return err
	}

	if script.Exit == ExitSIGTERM {
		if _, err := io.WriteString(out, SIGTERMResumeHint+"abc-123-uuid\n"); err != nil {
			return err
		}
	}
	return nil
}

// scanCR is a bufio.SplitFunc that splits on '\r' (CR, 0x0D). Matches
// real claude-code, which runs the TTY in raw mode and recognises CR —
// not LF — as Enter. A leading '\n' on the returned token (from a prior
// '\r\n' sequence) is left for the caller to strip.
//
// Uncommitted bytes at EOF (no trailing CR) are discarded — mirroring
// the real binary, where a process exit on an unsubmitted input does
// not retroactively commit it.
func scanCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), nil, nil
	}
	return 0, nil, nil
}

func matchResponse(s Script, input string) []byte {
	for _, m := range s.Matches {
		if strings.HasPrefix(input, m.InputPrefix) {
			return m.Response
		}
	}
	if s.Default != nil {
		return s.Default
	}
	return []byte(fmt.Sprintf("echo: %s\n", input))
}

// emitTurn writes a busy-title sequence (cook duration), the response
// bytes, then an idle-title sequence. Busy/idle titles use the OSC
// signature plumb's detector notes identified.
func emitTurn(out io.Writer, response []byte, cook time.Duration) error {
	if _, err := out.Write(BusyTitle("…")); err != nil {
		return err
	}
	if cook > 0 {
		time.Sleep(cook)
	}
	if _, err := out.Write(response); err != nil {
		return err
	}
	if _, err := out.Write(IdleTitle("done")); err != nil {
		return err
	}
	return nil
}

// BusyTitle returns an OSC title-bar sequence with a leading braille glyph
// (U+2802) and the supplied trailing label. The driver's title detector
// classifies titles whose leading rune lies in the braille block (U+2800–
// U+28FF) as busy.
func BusyTitle(label string) []byte {
	return []byte("\x1b]0;⠂ " + label + "\x07")
}

// IdleTitle returns an OSC title-bar sequence with the leading idle glyph
// (U+2733, ✳) and the supplied trailing label.
func IdleTitle(label string) []byte {
	return []byte("\x1b]0;✳ " + label + "\x07")
}
