//go:build !windows

package pty

import (
	"os"
	"os/exec"

	creackpty "github.com/creack/pty"
)

// startInPTY starts cmd inside a pseudo-terminal sized rows x cols and
// returns the master file. Callers read REPL output from it and write
// keyboard input to it. Closing it sends EOF to the child.
func startInPTY(cmd *exec.Cmd, rows, cols uint16) (*os.File, error) {
	return creackpty.StartWithSize(cmd, &creackpty.Winsize{Rows: rows, Cols: cols})
}
