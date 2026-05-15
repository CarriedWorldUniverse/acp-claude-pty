//go:build windows

package pty

import "os"

// sendSIGTERM is a no-op on Windows. Stop on Windows still goes through
// process Kill; PTY support is gated behind ConPTY v2 anyway, so this
// path is unreachable in practice for the v1 driver.
func sendSIGTERM(p *os.Process) error {
	_ = p
	return nil
}
