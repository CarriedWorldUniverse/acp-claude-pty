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
// Stdin is read line-by-line; stdout emits OSC busy/idle titles per turn.
// Exit code 0.
package main

import (
	"fmt"
	"os"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/mockclaude"
)

func main() {
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
		os.Exit(1)
	}
}
