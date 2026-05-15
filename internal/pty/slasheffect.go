package pty

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// slashEffectParser detects output-side effects of slash commands and
// emits typed events. It is fed the same chunks as the prompt detector
// but performs its own line accumulation (since it cannot rely on the
// driver's ANSI-stripped line emission, which strips OSC titles that
// /clear detection needs).
//
// The parser tracks two streams:
//
//   - Line stream (post-ANSI-strip): matches the `⎿  Set <thing> to ...`
//     family of command-result lines for /model and /effort.
//
//   - OSC title stream (pre-strip): matches the /clear signature, which
//     is title-text reverting from a conversation summary back to the
//     literal default "Claude Code".
//
// Detected events are pushed onto an out channel in wire-order alongside
// the LineEvents from the driver's main I/O loop. The parser is not
// responsible for /compact start/end detection — that is line-pattern
// driven and is handled inline by the driver (see compact-tracking in
// the I/O loop integration).
type slashEffectParser struct {
	out chan<- Event

	// lineBuf accumulates ANSI-stripped bytes between newlines, so we can
	// inspect complete lines.
	lineBuf strings.Builder

	// titleBuf accumulates raw bytes between OSC start (\x1b]0;) and
	// terminator (BEL or ST), so we can inspect title text.
	titleBuf strings.Builder
	inTitle  bool
}

// newSlashEffectParser returns a parser that emits events to out.
func newSlashEffectParser(out chan<- Event) *slashEffectParser {
	return &slashEffectParser{out: out}
}

// Feed consumes a chunk of raw output bytes. Both line and title detection
// run in lockstep so events are emitted in observation order.
func (p *slashEffectParser) Feed(at time.Time, chunk []byte) {
	for i := 0; i < len(chunk); i++ {
		b := chunk[i]

		// OSC title bookkeeping (raw stream).
		if !p.inTitle && b == 0x1b && i+3 < len(chunk) && chunk[i+1] == ']' && chunk[i+2] == '0' && chunk[i+3] == ';' {
			p.inTitle = true
			p.titleBuf.Reset()
			i += 3
			continue
		}
		if p.inTitle {
			if b == 0x07 {
				p.classifyTitle(at, p.titleBuf.String())
				p.inTitle = false
				continue
			}
			if b == 0x1b && i+1 < len(chunk) && chunk[i+1] == '\\' {
				p.classifyTitle(at, p.titleBuf.String())
				p.inTitle = false
				i++
				continue
			}
			p.titleBuf.WriteByte(b)
			continue
		}
	}

	// Line stream: ANSI-strip the chunk and accumulate lines.
	stripped := StripANSI(chunk)
	for _, b := range stripped {
		if b == '\n' {
			line := strings.TrimRight(p.lineBuf.String(), "\r")
			p.classifyLine(at, line)
			p.lineBuf.Reset()
			continue
		}
		p.lineBuf.WriteByte(b)
	}
}

// continuationPrefix is the UI marker the TUI prefixes every command-
// result line with: U+23BF + two spaces.
const continuationPrefix = "⎿  "

var (
	// "⎿  Set model to <name>" — name may include arbitrary tail text.
	setModelRE = regexp.MustCompile(`^Set model to (.+)$`)
	// "⎿  Set effort level to <level>: <description>" — capture the level.
	setEffortRE = regexp.MustCompile(`^Set effort level to ([^:]+?)(?::.*)?$`)
	// "Compacting conversation" — appears mid-line during /compact.
	compactingRE = regexp.MustCompile(`Compacting conversation`)
)

// classifyLine inspects one fully-buffered, ANSI-stripped line.
func (p *slashEffectParser) classifyLine(at time.Time, line string) {
	trimmed := strings.TrimSpace(line)

	if compactingRE.MatchString(trimmed) {
		p.emit(NewCompactStart(at))
		return
	}

	if rest, ok := stripPrefix(trimmed, continuationPrefix); ok {
		if m := setModelRE.FindStringSubmatch(rest); m != nil {
			p.emit(NewModelChanged(at, strings.TrimSpace(m[1])))
			return
		}
		if m := setEffortRE.FindStringSubmatch(rest); m != nil {
			p.emit(NewEffortChanged(at, strings.TrimSpace(m[1])))
			return
		}
	}
}

// classifyTitle inspects one fully-buffered title body.
//
// /clear signature: title text reverts to the literal "Claude Code" after
// having been set to a conversation-summary title. We don't model the
// before/after; we emit Cleared on any title whose text-content (post-
// leading-glyph) is exactly "Claude Code". The leading glyph is stripped
// because both busy and idle states pass through this default title.
func (p *slashEffectParser) classifyTitle(at time.Time, title string) {
	rest := stripLeadingRune(title)
	rest = strings.TrimSpace(rest)
	if rest == "Claude Code" {
		p.emit(NewCleared(at))
	}
}

func (p *slashEffectParser) emit(ev Event) {
	if p.out == nil {
		return
	}
	select {
	case p.out <- ev:
	default:
		// Slow consumer: drop the event rather than block the I/O loop.
		// Event observability is best-effort; the LineEvents still carry
		// the underlying output.
	}
}

func stripPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

// stripLeadingRune returns s with its first rune removed.
func stripLeadingRune(s string) string {
	if s == "" {
		return ""
	}
	_, n := utf8.DecodeRuneInString(s)
	return s[n:]
}
