package pty

import (
	"bytes"
	"unicode/utf8"
)

// titlePromptDetector implements PromptDetector by parsing OSC title-bar
// sequences (\x1b]0;<title>\x07 or \x1b]0;<title>\x1b\\) out of the raw
// stream and classifying the leading rune of each title:
//
//   - leading rune in the braille block (U+2800–U+28FF): busy/thinking
//   - leading rune U+2733 (✳): idle/done
//
// A turn is considered complete on the first busy→idle transition observed
// after Reset. Idle titles seen before any busy title are ignored — they
// correspond to the steady-state pre-input prompt and would otherwise fire
// before claude has even begun the turn.
//
// Operates on the raw (pre-ANSI-strip) byte stream because the ANSI stripper
// removes OSC sequences. The detector is byte-stream-incremental: it tolerates
// the OSC bytes being split across multiple Feed calls.
type titlePromptDetector struct {
	buf      []byte
	sawBusy  bool
	finished bool
}

// NewTitlePromptDetector returns a PromptDetector implementing the
// busy→idle OSC-title transition rule from plumb's probe-3 findings.
func NewTitlePromptDetector() PromptDetector {
	return &titlePromptDetector{}
}

func (d *titlePromptDetector) Reset() {
	d.buf = d.buf[:0]
	d.sawBusy = false
	d.finished = false
}

// Feed consumes a chunk of raw output bytes. It returns true when a
// busy→idle transition has been observed in the byte stream since Reset.
func (d *titlePromptDetector) Feed(chunk []byte) bool {
	if d.finished {
		return true
	}
	d.buf = append(d.buf, chunk...)

	for {
		start := bytes.Index(d.buf, []byte("\x1b]0;"))
		if start < 0 {
			// No partial OSC could span; drop everything before the last ESC
			// (if any) to bound buffer growth.
			if idx := bytes.LastIndexByte(d.buf, 0x1b); idx > 0 {
				d.buf = d.buf[idx:]
			} else if len(d.buf) > 4 {
				d.buf = d.buf[len(d.buf)-1:]
			}
			return false
		}
		bodyStart := start + len("\x1b]0;")

		end, endLen := findOSCTerminator(d.buf[bodyStart:])
		if end < 0 {
			// Partial OSC; keep buffered for next Feed.
			d.buf = d.buf[start:]
			return false
		}

		title := d.buf[bodyStart : bodyStart+end]
		d.classify(title)
		d.buf = d.buf[bodyStart+end+endLen:]

		if d.finished {
			d.buf = d.buf[:0]
			return true
		}
	}
}

func (d *titlePromptDetector) classify(title []byte) {
	r, _ := utf8.DecodeRune(title)
	if r == utf8.RuneError {
		return
	}
	switch {
	case isBrailleRune(r):
		d.sawBusy = true
	case r == '✳':
		if d.sawBusy {
			d.finished = true
		}
	}
}

func isBrailleRune(r rune) bool { return r >= 0x2800 && r <= 0x28FF }

// findOSCTerminator returns (index, terminator-length) of the first OSC
// terminator in b: either BEL (\x07, 1 byte) or ST (\x1b\\, 2 bytes).
// Returns (-1, 0) if no terminator is present in b.
func findOSCTerminator(b []byte) (int, int) {
	bel := bytes.IndexByte(b, 0x07)
	st := bytes.Index(b, []byte{0x1b, '\\'})
	switch {
	case bel < 0 && st < 0:
		return -1, 0
	case bel < 0:
		return st, 2
	case st < 0:
		return bel, 1
	case bel < st:
		return bel, 1
	default:
		return st, 2
	}
}
