package pty

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClaudeProjectDir_Encoding(t *testing.T) {
	if runtime.GOOS == "windows" {
		// The driver is macOS/Linux-only (Windows ConPTY is v2); these
		// cases assume unix absolute paths, which filepath.Abs rewrites
		// with a drive letter on Windows.
		t.Skip("encoding test assumes unix paths")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cases := map[string]string{
		"/Users/jacinta/shadow":           "-Users-jacinta-shadow",
		"/private/var/folders/6_/x":       "-private-var-folders-6--x",
		"/Users/j/Source/bridle/provider": "-Users-j-Source-bridle-provider",
	}
	for cwd, wantEnc := range cases {
		got, ok := claudeProjectDir(cwd)
		if !ok {
			t.Fatalf("claudeProjectDir(%q) not ok", cwd)
		}
		want := filepath.Join(home, ".claude", "projects", wantEnc)
		if got != want {
			t.Errorf("claudeProjectDir(%q) = %q, want %q", cwd, got, want)
		}
	}
	if _, ok := claudeProjectDir(""); ok {
		t.Error("empty cwd should not resolve")
	}
}

func TestReadAssistantTextSince(t *testing.T) {
	// A realistic transcript slice: user prompt, an assistant turn with a
	// thinking block + text, a tool_use, then the final text. Only the
	// assistant *text* blocks should come back, in order, chrome-free.
	lines := []string{
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","text":"hmm"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Let me check."}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"bash"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done: PONG"}]}}`,
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "s.jsonl")
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	text, off, err := readAssistantTextSince(file, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "Let me check.\nDone: PONG"
	if text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
	if off != int64(len(body)) {
		t.Errorf("offset = %d, want %d", off, len(body))
	}

	// A second read from the new offset yields nothing (no new lines).
	text2, off2, err := readAssistantTextSince(file, off)
	if err != nil {
		t.Fatal(err)
	}
	if text2 != "" || off2 != off {
		t.Errorf("incremental read = (%q, %d), want (\"\", %d)", text2, off2, off)
	}
}

func TestReadAssistantTextSince_LeavesPartialLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "s.jsonl")
	complete := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}` + "\n"
	partial := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"sec`
	if err := os.WriteFile(file, []byte(complete+partial), 0o644); err != nil {
		t.Fatal(err)
	}
	text, off, err := readAssistantTextSince(file, 0)
	if err != nil {
		t.Fatal(err)
	}
	if text != "first" {
		t.Errorf("text = %q, want %q", text, "first")
	}
	if off != int64(len(complete)) {
		t.Errorf("offset = %d, want %d (partial line not consumed)", off, len(complete))
	}
}
