// Command mockclaude is the executable form of internal/mockclaude. It is
// used as a fake claude binary in integration tests where the driver
// spawns a real subprocess inside a PTY.
//
// Behaviour is controlled by environment variables, set by the driver
// tests:
//
//	MOCKCLAUDE_EXIT=clean|sigterm|model-invalid
//	MOCKCLAUDE_INIT=<utf8 string written before any input is read>
//	MOCKCLAUDE_DEFAULT=<utf8 string written for any unmatched input>
//
// Stdin is read CR-terminated; stdout emits OSC busy/idle titles per turn.
// Exit code 0.
//
// Raw-mode contract: when stdin is a TTY (i.e. the driver spawned us via
// PTY), mockclaude puts the terminal into raw mode on startup. This
// mirrors real claude-code, which runs tcsetattr to disable ICRNL so that
// CR keystrokes reach the application as '\r' rather than being
// translated to '\n' by the cooked-mode line discipline. Without this,
// pty.Driver.Send (which writes '\r') would have its CR rewritten to LF
// by the slave-side terminal before mockclaude's stdin reader saw it,
// and the CR-only splitter in internal/mockclaude would hang.
package main

import (
	"fmt"
	"os"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/mockclaude"
	"golang.org/x/term"
)

func main() {
	os.Exit(run())
}

func run() int {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mockclaude: MakeRaw:", err)
			return 1
		}
		defer func() { _ = term.Restore(fd, state) }()
	}

	script := mockclaude.Script{
		Init:    []byte(os.Getenv("MOCKCLAUDE_INIT")),
		Default: []byte(os.Getenv("MOCKCLAUDE_DEFAULT")),
	}
	switch os.Getenv("MOCKCLAUDE_EXIT") {
	case "sigterm":
		script.Exit = mockclaude.ExitSIGTERM
	case "model-invalid":
		script.Exit = mockclaude.ExitModelInvalid
	}

	if err := mockclaude.Run(os.Stdin, os.Stdout, script); err != nil {
		fmt.Fprintln(os.Stderr, "mockclaude:", err)
		return 1
	}
	return 0
}
