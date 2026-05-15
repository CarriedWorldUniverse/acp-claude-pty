//go:build windows

package pty

import (
	"errors"
	"os"
	"os/exec"
)

// ErrUnsupportedPlatform is returned by Start on platforms without PTY
// support in this build. Windows ConPTY support is tracked as v2.
var ErrUnsupportedPlatform = errors.New("pty: platform not supported in this build (Windows ConPTY is v2)")

func startInPTY(cmd *exec.Cmd, rows, cols uint16) (*os.File, error) {
	_, _, _ = cmd, rows, cols
	return nil, ErrUnsupportedPlatform
}
