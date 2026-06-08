package pty

import (
	_ "embed"
	"testing"
)

// realTrustDialog is the raw, pre-ANSI-strip PTY bytes captured from a real
// claude (2.1.168) folder-trust dialog in a fresh spawn dir. Refresh it after a
// claude version bump with:
//
//	go test -tags=capturetrust ./internal/pty/ -run TestCaptureTrustDialog -v
//
//go:embed testdata/trust-dialog.raw
var realTrustDialog []byte

func TestTrustDialogVisible(t *testing.T) {
	// Pinned to REAL captured bytes, run through the same StripANSI the driver
	// applies. The TUI renders with cursor-positioning (not literal spaces), so
	// this is the genuine production input — not a hand-written approximation.
	if !trustDialogVisible(StripANSI(realTrustDialog)) {
		t.Errorf("did not detect the real captured trust dialog.\nstripped: %q", StripANSI(realTrustDialog))
	}

	// Spaced form (e.g. a future render) must also match.
	if !trustDialogVisible([]byte("Quick safety check: ... trust this folder?")) {
		t.Error("did not detect the spaced trust dialog")
	}

	// Normal turn output must not false-positive.
	for _, ok := range []string{
		"Here is the answer to your question.",
		"⏺ PONG",
		"Running tests… all green.",
	} {
		if trustDialogVisible([]byte(ok)) {
			t.Errorf("false positive on normal output: %q", ok)
		}
	}
}
