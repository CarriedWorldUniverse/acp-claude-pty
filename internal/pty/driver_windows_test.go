//go:build windows

package pty

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestStart_UnsupportedOnWindows asserts that Start surfaces a clear
// platform-unsupported error until ConPTY support lands (v2).
func TestStart_UnsupportedOnWindows(t *testing.T) {
	dir := t.TempDir()
	d, err := New(Options{Command: "cmd.exe", Cwd: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = d.Start(ctx)
	if err == nil {
		t.Fatal("Start should fail on Windows in this build")
	}
	var de *DriverError
	if !errors.As(err, &de) {
		t.Fatalf("expected *DriverError, got %T: %v", err, err)
	}
	if de.Kind != ErrCrash {
		t.Errorf("Kind = %v, want ErrCrash", de.Kind)
	}
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform in chain, got: %v", err)
	}
}
