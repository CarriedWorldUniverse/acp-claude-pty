package pty

import "time"

// Event is one observation emitted by the driver between Send and
// prompt-return. The concrete type encodes the kind of observation; the
// channel preserves wire-order so consumers can switch on type without
// reconstructing a sequence from parallel streams.
//
// All Events carry the wall-clock time they were observed. Implementations
// embed eventBase to satisfy the interface and supply At().
type Event interface {
	// At reports when the event was observed.
	At() time.Time
	// isEvent is an unexported marker. Defined on eventBase only; this
	// closes the Event interface to internal types and lets the compiler
	// catch missing cases in type switches when paired with linters.
	isEvent()
}

type eventBase struct {
	at time.Time
}

func (e eventBase) At() time.Time { return e.at }
func (e eventBase) isEvent()      {}

func newBase(at time.Time) eventBase {
	if at.IsZero() {
		at = time.Now()
	}
	return eventBase{at: at}
}

// LineEvent is one line of ANSI-stripped output from the REPL. The line
// does not include a trailing newline. Empty lines are emitted as Line == "".
type LineEvent struct {
	eventBase
	Line string
}

// NewLineEvent constructs a LineEvent stamped with the supplied time, or
// time.Now() if at is zero.
func NewLineEvent(at time.Time, line string) LineEvent {
	return LineEvent{eventBase: newBase(at), Line: line}
}

// CompactStart fires when claude begins a /compact operation. While a
// CompactStart is in flight (i.e. before the matching CompactEnd), the
// driver suppresses its idle-silence backstop so the long-running compact
// does not raise a spurious hang error. PromptTimeout still applies.
type CompactStart struct {
	eventBase
}

// NewCompactStart constructs a CompactStart stamped with at (or Now if zero).
func NewCompactStart(at time.Time) CompactStart {
	return CompactStart{eventBase: newBase(at)}
}

// CompactEnd fires when /compact has completed.
//
// CompactEnd does NOT carry the compact summary text: per plumb's probe-5
// findings, claude's TUI rolls the summary into the new conversation
// context invisibly — it isn't surfaced as a parseable line in the PTY
// stream. Callers that want the summary read it from the session JSONL;
// see CompactSummaryAvailable.
type CompactEnd struct {
	eventBase
}

// NewCompactEnd constructs a CompactEnd.
func NewCompactEnd(at time.Time) CompactEnd {
	return CompactEnd{eventBase: newBase(at)}
}

// CompactSummaryAvailable fires when the post-/compact session JSONL has
// been observed to contain the compact summary record. Path is the
// absolute path to the JSONL file the caller can read to extract the
// summary text. The driver does not parse the JSONL itself.
//
// Emission of this event is best-effort: if the JSONL location cannot be
// determined or the file does not become available within a reasonable
// window after CompactEnd, the event is omitted.
type CompactSummaryAvailable struct {
	eventBase
	Path string
}

// NewCompactSummaryAvailable constructs a CompactSummaryAvailable.
func NewCompactSummaryAvailable(at time.Time, path string) CompactSummaryAvailable {
	return CompactSummaryAvailable{eventBase: newBase(at), Path: path}
}

// Cleared fires when /clear has completed. Detected via the title-bar text
// reverting from a previous-conversation summary back to the literal
// default "Claude Code" string (per plumb's probe-5).
type Cleared struct {
	eventBase
}

// NewCleared constructs a Cleared.
func NewCleared(at time.Time) Cleared {
	return Cleared{eventBase: newBase(at)}
}

// ModelChanged fires when claude reports a model switch (typically after a
// /model command). Model is the identifier claude printed.
type ModelChanged struct {
	eventBase
	Model string
}

// NewModelChanged constructs a ModelChanged.
func NewModelChanged(at time.Time, model string) ModelChanged {
	return ModelChanged{eventBase: newBase(at), Model: model}
}

// EffortChanged fires when claude reports a reasoning-effort switch
// (typically after a /effort command). Level is the identifier claude
// printed (e.g. "low", "medium", "high").
type EffortChanged struct {
	eventBase
	Level string
}

// NewEffortChanged constructs an EffortChanged.
func NewEffortChanged(at time.Time, level string) EffortChanged {
	return EffortChanged{eventBase: newBase(at), Level: level}
}

// SessionExiting fires when claude is about to exit cleanly (typically
// after a /exit command), before the PTY EOF. Reason is free-form text the
// driver does not interpret; callers may surface it in logs. The matching
// PTY EOF maps to ErrGracefulEOF; an EOF without a preceding SessionExiting
// maps to ErrCrash.
type SessionExiting struct {
	eventBase
	Reason string
}

// NewSessionExiting constructs a SessionExiting.
func NewSessionExiting(at time.Time, reason string) SessionExiting {
	return SessionExiting{eventBase: newBase(at), Reason: reason}
}
