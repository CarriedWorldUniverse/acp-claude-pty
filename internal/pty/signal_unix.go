//go:build !windows

package pty

import (
	"os"
	"syscall"
)

// sendSIGTERM delivers SIGTERM to p. Errors are returned to the caller for
// best-effort logging; callers should still wait for process exit.
func sendSIGTERM(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
