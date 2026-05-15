//go:build linux

package pty

import "golang.org/x/sys/unix"

const (
	tcGet = unix.TCGETS
	tcSet = unix.TCSETS
)
