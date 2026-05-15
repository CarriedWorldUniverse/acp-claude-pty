package spawndir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMaterialize_TempDirOwned(t *testing.T) {
	d, err := Materialize("", Spec{
		Files:    map[string][]byte{"hello.txt": []byte("hi")},
		CLAUDEMd: "# claude.md\n",
		MCPServers: map[string]any{
			"echo": map[string]any{"command": "cat"},
		},
		Settings: map[string]any{"theme": "dark"},
	})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	t.Cleanup(func() { _ = d.Cleanup() })

	mustRead := func(rel string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(d.Path, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return b
	}

	if got := string(mustRead("hello.txt")); got != "hi" {
		t.Errorf("hello.txt = %q, want %q", got, "hi")
	}
	if got := string(mustRead("CLAUDE.md")); got != "# claude.md\n" {
		t.Errorf("CLAUDE.md mismatch: %q", got)
	}

	var mcp struct {
		MCPServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(mustRead(".mcp.json"), &mcp); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}
	if mcp.MCPServers["echo"].Command != "cat" {
		t.Errorf(".mcp.json missing echo server: %+v", mcp)
	}

	var settings map[string]any
	if err := json.Unmarshal(mustRead(".claude/settings.json"), &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	if settings["theme"] != "dark" {
		t.Errorf("settings.theme = %v, want dark", settings["theme"])
	}
}

func TestMaterialize_CallerSuppliedDir(t *testing.T) {
	dir := t.TempDir()
	d, err := Materialize(dir, Spec{
		Files: map[string][]byte{"a/b/c.txt": []byte("nested")},
	})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if d.ownedTemp {
		t.Fatal("caller-supplied dir should not be owned")
	}
	if err := d.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a/b/c.txt")); err != nil {
		t.Errorf("caller-supplied dir should survive Cleanup: %v", err)
	}
}

func TestMaterialize_RejectsEscapingPaths(t *testing.T) {
	cases := []string{"../escape", "/abs/path", "a/../../b"}
	for _, rel := range cases {
		t.Run(rel, func(t *testing.T) {
			_, err := Materialize("", Spec{Files: map[string][]byte{rel: []byte("x")}})
			if err == nil {
				t.Fatalf("expected error for %q", rel)
			}
		})
	}
}

func TestMaterialize_NoOpForZeroSpec(t *testing.T) {
	d, err := Materialize("", Spec{})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	t.Cleanup(func() { _ = d.Cleanup() })

	entries, err := os.ReadDir(d.Path)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty dir, got %d entries", len(entries))
	}
}

func TestCleanup_RemovesOwnedTemp(t *testing.T) {
	d, err := Materialize("", Spec{})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	path := d.Path
	if err := d.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected dir removed, stat err = %v", err)
	}
}
