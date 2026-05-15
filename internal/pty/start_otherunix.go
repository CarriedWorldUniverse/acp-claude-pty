//go:build !windows && !linux && !darwin

package pty

import (
	"errors"
	"os"
	"os/exec"
)

// On unix platforms other than linux/darwin we don't yet ship the termios
// patch (clearing ICRNL on the slave before exec) that the driver relies
// on to keep CR keystrokes from being translated to LF. Rather than spawn
// a child without that patch — which would reintroduce the race fixed in
// start_unix.go — we return ErrUnsupportedPlatform here. Adding support
// is a matter of writing a termios_<goos>.go pair for tcGet/tcSet.
var ErrUnsupportedPlatform = errors.New("pty: platform not supported in this build (only linux/darwin/windows-via-ConPTY-v2)")

func startInPTY(cmd *exec.Cmd, rows, cols uint16) (*os.File, error) {
	_, _, _ = cmd, rows, cols
	return nil, ErrUnsupportedPlatform
}
