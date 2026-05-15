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
