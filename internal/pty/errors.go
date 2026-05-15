// Package pty drives the Claude Code CLI inside a pseudo-terminal.
//
// The driver holds one persistent REPL across many Send calls. Restart is
// caller-driven (explicit).
package pty

import (
	"errors"
	"fmt"
)

// ErrorKind classifies a driver failure. Stable values; callers (notably the
// ACP server layer) map these onto wire-level error codes.
type ErrorKind int

const (
	// ErrCrash means the underlying claude process exited non-zero or was
	// killed by a signal while a Send was in flight.
	ErrCrash ErrorKind = iota + 1

	// ErrHang means the driver detected an unrecoverable I/O stall — the
	// process is alive but neither producing output nor accepting input.
	ErrHang

	// ErrPromptTimeout means the prompt did not return within the per-Send
	// deadline. The process may still be working; the Send is abandoned.
	ErrPromptTimeout

	// ErrGracefulEOF means claude exited cleanly (zero status) and the PTY
	// closed. Any in-flight Send is reported with this kind.
	ErrGracefulEOF

	// ErrAbortedBySIGTERM means claude received SIGTERM and printed its
	// resume hint ("Resume this session with: claude --resume <uuid>")
	// before exit (exit code 143). This is the cleanest abort path; the
	// session is recoverable via the printed --resume identifier.
	ErrAbortedBySIGTERM

	// ErrKilled means claude was terminated by SIGKILL (term_sig=9) with
	// no graceful tail. On macOS, closing the PTY fd directly delivers a
	// TTY hangup that surfaces as SIGKILL; the driver's Stop path issues
	// SIGTERM-then-grace-then-close to avoid this where possible.
	ErrKilled

	// ErrModelInvalid means claude reported the configured model is not
	// available. This is a first-Send error, not a Start error: claude-code
	// launches its TUI successfully regardless of model validity and only
	// surfaces the failure when a turn is attempted.
	ErrModelInvalid
)

func (k ErrorKind) String() string {
	switch k {
	case ErrCrash:
		return "crash"
	case ErrHang:
		return "hang"
	case ErrPromptTimeout:
		return "prompt-timeout"
	case ErrGracefulEOF:
		return "graceful-eof"
	case ErrAbortedBySIGTERM:
		return "aborted-by-sigterm"
	case ErrKilled:
		return "killed"
	case ErrModelInvalid:
		return "model-invalid"
	default:
		return fmt.Sprintf("unknown(%d)", int(k))
	}
}

// DriverError is the typed error surface for the PTY driver. Every error
// the driver returns to callers wraps one of these so the ACP layer can map
// on Kind without string-matching.
type DriverError struct {
	Kind ErrorKind
	// ExitCode is the underlying process exit code when known (Crash,
	// GracefulEOF); zero otherwise.
	ExitCode int
	// Detail is a human-readable hint. May be empty.
	Detail string
	// Cause is the underlying error, if any.
	Cause error
}

func (e *DriverError) Error() string {
	parts := "pty: " + e.Kind.String()
	if e.Detail != "" {
		parts += ": " + e.Detail
	}
	if e.Cause != nil {
		parts += ": " + e.Cause.Error()
	}
	return parts
}

func (e *DriverError) Unwrap() error { return e.Cause }

// AsDriverError extracts a *DriverError from err, if present.
func AsDriverError(err error) (*DriverError, bool) {
	var de *DriverError
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}
