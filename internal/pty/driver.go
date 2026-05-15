package pty

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Options configures a Driver. Zero values are documented defaults.
type Options struct {
	// Command is the claude binary to launch. Defaults to "claude".
	Command string
	// Args are arguments passed to the command. Defaults to no args
	// (interactive REPL). Callers MUST NOT pass "-p" — the driver requires
	// the interactive REPL.
	Args []string
	// Env is the process environment. If nil, os.Environ() is used.
	Env []string
	// Cwd is the spawn directory the process runs in. Required.
	Cwd string

	// Rows is the PTY row count reported to claude. Defaults to 40.
	// Per operator-resolved dimensions (chat #1128) the parser is
	// size-agnostic; this affects only how diagnostic logs render.
	Rows uint16
	// Cols is the PTY column count reported to claude. Defaults to 80.
	Cols uint16

	// PromptTimeout bounds how long a single Send waits for the prompt to
	// return after the last byte of input. Zero disables the timeout
	// (not recommended). Defaults to 5 minutes if zero.
	PromptTimeout time.Duration
	// HangTimeout bounds how long the driver tolerates total output
	// silence while a Send is in flight, before raising ErrHang. Zero
	// disables. Defaults to 60 seconds if zero.
	HangTimeout time.Duration

	// StopGrace is the window Stop waits between sending SIGTERM and
	// escalating to SIGKILL. Per plumb's probe-4c findings, claude-code
	// prints its resume hint after SIGTERM before exiting with code 143;
	// the grace window must be long enough for that tail to flush.
	// Defaults to 2 seconds.
	StopGrace time.Duration

	// StdoutLog, if non-nil, receives every byte read from the PTY before
	// ANSI stripping. The driver does not close it.
	StdoutLog io.Writer
	// StderrLog, if non-nil, receives stderr-equivalent diagnostics from
	// the driver itself (spawn errors, lifecycle transitions). The driver
	// does not close it.
	StderrLog io.Writer

	// PromptDetector decides when the prompt has returned. If nil, a
	// default detector is used (see DefaultPromptDetector).
	PromptDetector PromptDetector
}

// PromptDetector inspects ANSI-stripped output bytes and reports whether the
// claude REPL prompt has returned (i.e. the turn is complete).
//
// Implementations must be safe for repeated calls within a single Send and
// may keep internal state across calls. Reset is invoked at the start of
// every Send.
type PromptDetector interface {
	Reset()
	Feed(chunk []byte) (done bool)
}

// Driver is the persistent PTY-backed claude REPL.
//
// One Driver wraps exactly one claude process. Concurrent Sends are
// serialized internally; callers may invoke Send from multiple goroutines
// safely but only one turn runs at a time. Start/Stop/Restart are not safe
// to call concurrently with each other; Send is safe to call concurrently
// with itself.
type Driver struct {
	opts Options

	startMu sync.Mutex // guards Start/Stop/Restart against each other
	sendMu  sync.Mutex // serializes Sends

	stateMu sync.RWMutex
	started bool
	cmd     *exec.Cmd
	ptyFile *os.File // master side of the PTY; nil before Start, after Stop
	waitCh  chan error
	chunks  <-chan readChunk // populated in Start; consumed by Send
}

// New constructs a Driver. It does not start the process; call Start.
func New(opts Options) (*Driver, error) {
	if opts.Cwd == "" {
		return nil, errors.New("pty: Options.Cwd is required")
	}
	if opts.Command == "" {
		opts.Command = "claude"
	}
	if opts.Rows == 0 {
		opts.Rows = 40
	}
	if opts.Cols == 0 {
		opts.Cols = 80
	}
	if opts.PromptTimeout == 0 {
		opts.PromptTimeout = 5 * time.Minute
	}
	if opts.HangTimeout == 0 {
		opts.HangTimeout = 60 * time.Second
	}
	if opts.StopGrace == 0 {
		opts.StopGrace = 2 * time.Second
	}
	return &Driver{opts: opts}, nil
}

// Start spawns the claude process inside a PTY and prepares it to receive
// Sends. It returns once the process is running, or an error wrapping a
// *DriverError on failure. The initial prompt-settle wait is the caller's
// concern (the first Send will block until the REPL is ready).
//
// On platforms without PTY support (Windows, pending ConPTY v2), Start
// returns an unwrapped error indicating the platform is unsupported.
func (d *Driver) Start(ctx context.Context) error {
	d.startMu.Lock()
	defer d.startMu.Unlock()
	return d.startLocked(ctx)
}

// startLocked is the body of Start, callable from Restart. The caller
// must hold d.startMu.
func (d *Driver) startLocked(ctx context.Context) error {
	d.stateMu.RLock()
	already := d.started
	d.stateMu.RUnlock()
	if already {
		return errors.New("pty: already started")
	}

	cmd := exec.CommandContext(ctx, d.opts.Command, d.opts.Args...)
	cmd.Dir = d.opts.Cwd
	if d.opts.Env != nil {
		cmd.Env = d.opts.Env
	} else {
		cmd.Env = os.Environ()
	}

	ptyFile, err := startInPTY(cmd, d.opts.Rows, d.opts.Cols)
	if err != nil {
		return &DriverError{Kind: ErrCrash, Detail: "spawn", Cause: err}
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	chunks := startReadLoop(ptyFile)

	d.stateMu.Lock()
	d.cmd = cmd
	d.ptyFile = ptyFile
	d.waitCh = waitCh
	d.chunks = chunks
	d.started = true
	d.stateMu.Unlock()

	return nil
}

// Turn is the handle returned by Send. Callers drain Events until it
// closes, then read Err for the terminating *DriverError (or nil on a
// clean prompt-return).
//
// The Events channel and Err are populated by a single goroutine; reading
// Err before Events is fully drained is not safe.
type Turn struct {
	// Events delivers LineEvents and tagged effect events in wire-order.
	// Closed when the turn terminates (prompt-return, error, EOF, or ctx
	// cancellation).
	Events <-chan Event

	errMu sync.Mutex
	err   error
}

// Err returns the terminating error for the turn. Only safe to call after
// Events has been fully drained (i.e. the channel close has been observed).
func (t *Turn) Err() error {
	t.errMu.Lock()
	defer t.errMu.Unlock()
	return t.err
}

// Send injects input into the REPL and returns a Turn whose Events channel
// closes when the prompt returns or an error condition fires. Callers must
// drain Events before reading Turn.Err.
//
// Sends are serialized: a second Send blocks until the first completes
// (i.e. until the prior Turn.Events channel is closed and drained by the
// driver). Input is written as-is followed by a single '\r' (CR, 0x0D)
// to commit. claude-code's TUI runs the TTY in raw mode and recognises
// CR — not LF — as Enter; sending '\n' leaves the input uncommitted
// (verified by plumb's probe-8: 3/3 wedge on '\n', 3/3 land on '\r' for
// /compact against the real binary). Callers control the slash-command
// surface by including the leading '/' in input; the driver does not
// interpret command text.
//
// Send returns a non-nil error only for pre-flight failures (driver not
// started, PTY write failure). Turn-time errors are delivered via
// Turn.Err after Events closes.
func (d *Driver) Send(ctx context.Context, input string) (*Turn, error) {
	d.sendMu.Lock()

	d.stateMu.RLock()
	started := d.started
	ptyFile := d.ptyFile
	chunks := d.chunks
	d.stateMu.RUnlock()

	if !started {
		d.sendMu.Unlock()
		return nil, &DriverError{Kind: ErrCrash, Detail: "Send before Start"}
	}

	detector := d.opts.PromptDetector
	if detector == nil {
		detector = NewTitlePromptDetector()
	}
	detector.Reset()

	if _, err := ptyFile.Write([]byte(input + "\r")); err != nil {
		d.sendMu.Unlock()
		return nil, &DriverError{Kind: ErrCrash, Detail: "write input to pty", Cause: err}
	}

	out := make(chan Event, 32)
	turn := &Turn{Events: out}

	loop := &turnLoop{
		chunks:        chunks,
		detector:      detector,
		effects:       newSlashEffectParser(out),
		out:           out,
		stdoutLog:     d.opts.StdoutLog,
		promptTimeout: d.opts.PromptTimeout,
		hangTimeout:   d.opts.HangTimeout,
	}

	go func() {
		defer d.sendMu.Unlock()
		defer close(out)
		err := loop.runTurn(ctx)
		turn.errMu.Lock()
		turn.err = err
		turn.errMu.Unlock()
	}()

	return turn, nil
}

// Restart stops and re-starts the underlying process, materializing a fresh
// REPL while preserving the Driver and its Options. Used when callers want
// thread-isolated work. Expected cost is ~3s on macOS/Linux (spawn+init
// per plumb's probe-2 numbers); callers should treat Restart as
// affordable but not free.
func (d *Driver) Restart(ctx context.Context) error {
	d.startMu.Lock()
	defer d.startMu.Unlock()

	if err := d.stopLocked(ctx); err != nil {
		return err
	}
	return d.startLocked(ctx)
}

// Stop terminates the claude process gracefully and releases the PTY.
// Safe to call when already stopped.
//
// Sequence (per plumb's probe-4c findings):
//  1. Send SIGTERM to the process.
//  2. Wait up to Options.StopGrace for the process to exit. During this
//     window claude-code prints its resume-hint tail bytes.
//  3. If still alive, send SIGKILL.
//  4. Close the PTY master fd last, so the resume-hint bytes are readable.
//
// Closing the fd first would deliver a TTY hangup that surfaces as SIGKILL
// on macOS and burns the graceful tail.
func (d *Driver) Stop(ctx context.Context) error {
	d.startMu.Lock()
	defer d.startMu.Unlock()
	return d.stopLocked(ctx)
}

// stopLocked is the body of Stop, callable from Restart. The caller must
// hold d.startMu.
func (d *Driver) stopLocked(ctx context.Context) error {
	d.stateMu.RLock()
	cmd := d.cmd
	ptyFile := d.ptyFile
	waitCh := d.waitCh
	started := d.started
	d.stateMu.RUnlock()

	if !started {
		return nil
	}

	if cmd != nil && cmd.Process != nil {
		_ = sendSIGTERM(cmd.Process)
	}

	grace := d.opts.StopGrace
	if ctxDeadline, ok := ctx.Deadline(); ok {
		if untilCtx := time.Until(ctxDeadline); untilCtx < grace {
			grace = untilCtx
		}
	}

	select {
	case <-waitCh:
	case <-time.After(grace):
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-waitCh
	}

	if ptyFile != nil {
		_ = ptyFile.Close()
	}

	d.stateMu.Lock()
	d.cmd = nil
	d.ptyFile = nil
	d.waitCh = nil
	d.chunks = nil
	d.started = false
	d.stateMu.Unlock()

	return nil
}
