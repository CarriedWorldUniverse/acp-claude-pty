package pty

import "regexp"

// ansiPattern matches the CSI / OSC / single-char escape sequences that
// claude's REPL emits for styling, cursor moves, and screen clears. This is
// a pragmatic subset — enough to strip terminal control out of the bytes we
// hand to the prompt detector and to ACP consumers.
//
// References:
//   - ECMA-48 CSI: ESC '[' parameters intermediates final
//   - OSC: ESC ']' ... (BEL | ESC '\')
//   - Single-char escapes (e.g. ESC = , ESC > )
var ansiPattern = regexp.MustCompile(
	`\x1b\[[0-?]*[ -/]*[@-~]` + // CSI
		`|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)` + // OSC
		`|\x1b[()][\x20-\x7e]` + // charset designation
		`|\x1b[=>78cDEHMNOPVWXZ\\]`, // single-char / two-char escapes
)

// StripANSI removes ANSI escape sequences from b and returns a new slice.
// Carriage returns are preserved; callers that want line semantics should
// split on '\n'. The function does not modify b.
func StripANSI(b []byte) []byte {
	return ansiPattern.ReplaceAll(b, nil)
}

type ansiStreamState uint8

const (
	ansiNormal ansiStreamState = iota
	ansiEsc
	ansiCSI
	ansiOSC
	ansiOSCEsc
	ansiCharset
)

// ansiStreamStripper removes terminal control sequences from an incremental
// byte stream. It handles the same pragmatic subset as StripANSI, but keeps
// enough state to strip sequences split across reads.
type ansiStreamStripper struct {
	state ansiStreamState
}

func (s *ansiStreamStripper) Strip(chunk []byte) []byte {
	out := make([]byte, 0, len(chunk))
	for _, b := range chunk {
		switch s.state {
		case ansiNormal:
			if b == 0x1b {
				s.state = ansiEsc
				continue
			}
			out = append(out, b)

		case ansiEsc:
			switch b {
			case '[':
				s.state = ansiCSI
			case ']':
				s.state = ansiOSC
			case '(', ')':
				s.state = ansiCharset
			default:
				s.state = ansiNormal
			}

		case ansiCSI:
			if b >= 0x40 && b <= 0x7e {
				s.state = ansiNormal
			}

		case ansiOSC:
			switch b {
			case 0x07:
				s.state = ansiNormal
			case 0x1b:
				s.state = ansiOSCEsc
			}

		case ansiOSCEsc:
			if b == '\\' {
				s.state = ansiNormal
			} else if b != 0x1b {
				s.state = ansiOSC
			}

		case ansiCharset:
			s.state = ansiNormal
		}
	}
	return out
}
