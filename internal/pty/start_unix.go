//go:build linux || darwin

package pty

import (
	"os"
	"os/exec"
	"syscall"

	creackpty "github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// startInPTY starts cmd inside a pseudo-terminal sized rows x cols and
// returns the master file. Callers read REPL output from it and write
// keyboard input to it. Closing it sends EOF to the child.
//
// Before exec, the slave's termios is patched to clear ICRNL — the
// cooked-mode flag that rewrites input CR to NL. claude-code's TUI
// commits on CR ('\r', 0x0D); if the slave were left in default cooked
// mode at exec time, any CR byte the driver wrote before the child
// finished its own tcsetattr(raw) would be silently translated to NL at
// the kernel's line discipline, leaving the input uncommitted. Clearing
// ICRNL on the slave before exec closes that window structurally.
func startInPTY(cmd *exec.Cmd, rows, cols uint16) (*os.File, error) {
	ptmx, tty, err := creackpty.Open()
	if err != nil {
		return nil, err
	}
	// tty is the slave fd; only the parent needs it briefly to set the
	// child's std{in,out,err} and patch termios. Close it after Start.
	defer func() { _ = tty.Close() }()

	if err := creackpty.Setsize(ptmx, &creackpty.Winsize{Rows: rows, Cols: cols}); err != nil {
		_ = ptmx.Close()
		return nil, err
	}

	if err := clearICRNL(int(tty.Fd())); err != nil {
		_ = ptmx.Close()
		return nil, err
	}

	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Setctty = true

	if err := cmd.Start(); err != nil {
		_ = ptmx.Close()
		return nil, err
	}
	return ptmx, nil
}

// clearICRNL disables ICRNL on fd's termios so input CR is delivered
// unmodified rather than being rewritten to NL by the line discipline.
// Other flags are preserved.
func clearICRNL(fd int) error {
	t, err := unix.IoctlGetTermios(fd, tcGet)
	if err != nil {
		return err
	}
	t.Iflag &^= unix.ICRNL
	return unix.IoctlSetTermios(fd, tcSet, t)
}
