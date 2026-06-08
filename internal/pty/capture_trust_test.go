//go:build capturetrust

package pty

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	creackpty "github.com/creack/pty"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/spawndir"
)

// TestCaptureTrustDialog is the fixture-refresh tool for testdata/trust-dialog.raw.
// It spawns a REAL claude in a fresh (untrusted) spawn dir directly in a PTY,
// reads the raw pre-ANSI-strip stream (including the folder-trust dialog) for a
// few seconds, kills claude, and writes the captured bytes to the fixture.
// trust_test.go pins detection to those real bytes. Run deliberately to refresh
// after a claude version bump:
//
//	go test -tags=capturetrust ./internal/pty/ -run TestCaptureTrustDialog -v
func TestCaptureTrustDialog(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not on PATH")
	}
	sd, err := spawndir.Materialize("", spawndir.Spec{})
	if err != nil {
		t.Fatalf("spawndir: %v", err)
	}
	t.Cleanup(func() { _ = sd.Cleanup() })

	cmd := exec.Command("claude")
	cmd.Dir = sd.Path
	cmd.Env = stripBillingEnv(os.Environ()) // subscription auth, no API key

	ptmx, err := creackpty.Start(cmd)
	if err != nil {
		t.Fatalf("pty start claude: %v", err)
	}

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, ptmx); close(done) }()

	time.Sleep(9 * time.Second) // let claude render the trust dialog
	_ = cmd.Process.Kill()
	_ = ptmx.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	_, _ = cmd.Process.Wait()

	if buf.Len() == 0 {
		t.Fatal("captured 0 bytes — claude produced no PTY output")
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}
	if err := os.WriteFile("testdata/trust-dialog.raw", buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Logf("captured %d bytes to testdata/trust-dialog.raw", buf.Len())
}
