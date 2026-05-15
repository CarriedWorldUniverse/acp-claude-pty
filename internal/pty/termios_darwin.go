//go:build darwin

package pty

import "golang.org/x/sys/unix"

const (
	tcGet = unix.TIOCGETA
	tcSet = unix.TIOCSETA
)
