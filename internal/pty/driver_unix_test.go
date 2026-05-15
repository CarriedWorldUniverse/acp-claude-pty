//go:build linux || darwin

package pty

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestStartStop_TrueBinary spawns the trivial /bin/true via the PTY backend
// and stops it. This verifies the spawn/wait/teardown plumbing without
// asserting anything about output (which is empty for /bin/true).
func TestStartStop_TrueBinary(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Options{Command: "/usr/bin/true", Cwd: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := d.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Second Stop is a no-op.
	if err := d.Stop(ctx); err != nil {
		t.Fatalf("Stop (second): %v", err)
	}
}

// TestStart_RejectsDoubleStart verifies the started-guard.
func TestStart_RejectsDoubleStart(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Options{Command: "/bin/sh", Args: []string{"-c", "sleep 5"}, Cwd: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = d.Stop(context.Background()) }()

	if err := d.Start(ctx); err == nil {
		t.Fatal("second Start should fail")
	}
}

// TestStart_BadCommandReturnsCrash verifies the typed-error mapping on
// spawn failure (non-existent binary).
func TestStart_BadCommandReturnsCrash(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Options{Command: "/no/such/binary/anvil-test", Cwd: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = d.Start(ctx)
	if err == nil {
		t.Fatal("Start should fail for non-existent binary")
	}
	var de *DriverError
	if !errors.As(err, &de) {
		t.Fatalf("expected *DriverError, got %T: %v", err, err)
	}
	if de.Kind != ErrCrash {
		t.Errorf("Kind = %v, want ErrCrash", de.Kind)
	}
}
