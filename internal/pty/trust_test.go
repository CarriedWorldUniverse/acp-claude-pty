package pty

import "testing"

func TestTrustDialogVisible(t *testing.T) {
	// Real claude renders the TUI without literal spaces (cursor positioning,
	// not whitespace), so the detector must match the space-collapsed form —
	// this is the exact shape captured from claude 2.1.168.
	spaceless := []byte("Accessingworkspace:Quicksafetycheck:Isthisaprojectyoucreatedoroneyoutrust?1.Yes,Itrustthisfolder2.No,exit")
	if !trustDialogVisible(spaceless) {
		t.Error("did not detect the space-collapsed trust dialog (real-claude shape)")
	}

	// Spaced form (e.g. from a future render or a fixture) must also match.
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
