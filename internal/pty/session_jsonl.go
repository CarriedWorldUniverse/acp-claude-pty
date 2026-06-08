package pty

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Package-internal: source the turn's assistant text from claude's session
// transcript (the structured JSONL it writes per turn) rather than scraping
// the human TUI. The TUI stream stays for turn-boundary detection; the JSONL
// is the truth for CONTENT — drift-proof and free of spinner/status chrome.

// claudeProjectDir maps a spawn cwd to claude's transcript directory:
// ~/.claude/projects/<abs-cwd with every non-alphanumeric rune replaced by
// '-'>. Verified against claude 2.1.168 (e.g. "/Users/jacinta/shadow" ->
// "-Users-jacinta-shadow"; "/private/var/folders/6_/x" ->
// "-private-var-folders-6--x"). Returns ("", false) if the home or cwd is
// unavailable.
func claudeProjectDir(cwd string) (string, bool) {
	if cwd == "" {
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", false
	}
	// claude records the symlink-resolved path (e.g. macOS /var -> /private/var,
	// so a /var/folders temp dir is stored under -private-var-folders-...).
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	enc := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, abs)
	return filepath.Join(home, ".claude", "projects", enc), true
}

// findSessionFile returns the newest .jsonl transcript in the cwd's claude
// project dir, polling until timeout for it to appear (claude writes it on
// session init, shortly after spawn). Returns "" if none shows up — the caller
// falls back to the raw TUI stream.
func findSessionFile(cwd string, timeout time.Duration) string {
	dir, ok := claudeProjectDir(cwd)
	if !ok {
		return ""
	}
	deadline := time.Now().Add(timeout)
	for {
		if f := newestJSONL(dir); f != "" {
			return f
		}
		if !time.Now().Before(deadline) {
			return ""
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// newestJSONL returns the path of the most-recently-modified .jsonl in dir,
// or "" if the dir is unreadable or empty.
func newestJSONL(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newest string
	var newestT time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestT) {
			newest = filepath.Join(dir, e.Name())
			newestT = info.ModTime()
		}
	}
	return newest
}

// transcriptLine is the subset of a claude session JSONL record we read.
type transcriptLine struct {
	Type    string `json:"type"`
	Message struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// readAssistantTextSince reads complete JSONL lines from file starting at
// offset, returns the concatenated assistant text-block content appended since
// (skipping thinking/tool_use blocks and non-assistant records), and the new
// offset advanced past the last COMPLETE line. A trailing partial line (claude
// mid-write) is left unconsumed so the next read picks it up whole.
func readAssistantTextSince(file string, offset int64) (text string, newOffset int64, err error) {
	f, err := os.Open(file)
	if err != nil {
		return "", offset, err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", offset, err
	}
	var b strings.Builder
	newOffset = offset
	r := bufio.NewReader(f)
	for {
		line, rerr := r.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			newOffset += int64(len(line))
			appendAssistantText(&b, line)
			if rerr == nil {
				continue
			}
		}
		// partial (no trailing newline) or read error: stop without
		// consuming the incomplete tail.
		break
	}
	return b.String(), newOffset, nil
}

// appendAssistantText parses one JSONL record and appends its assistant
// text-block content to b (newline-separated), ignoring everything else.
func appendAssistantText(b *strings.Builder, line []byte) {
	var tl transcriptLine
	if err := json.Unmarshal(line, &tl); err != nil {
		return
	}
	if tl.Type != "assistant" || tl.Message.Role != "assistant" {
		return
	}
	for _, c := range tl.Message.Content {
		if c.Type == "text" && c.Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(c.Text)
		}
	}
}
