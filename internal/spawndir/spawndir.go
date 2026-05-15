// Package spawndir materializes the directory tree that the claude CLI sees
// as its working directory on launch.
//
// Inputs come from the caller (typically over ACP). The caller is the source
// of truth for what claude sees — this package writes bytes to disk and
// returns paths.
package spawndir

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Spec describes the contents to materialize into a spawn directory.
//
// All fields are optional. A zero Spec produces an empty directory.
type Spec struct {
	// Files is a map of relative path -> file contents. Paths must be
	// relative, must not escape the spawn dir (no "..", no absolute paths),
	// and use forward slashes.
	Files map[string][]byte

	// MCPServers, if non-nil, is JSON-marshaled and written to .mcp.json
	// at the spawn-dir root. The value is written verbatim under the
	// "mcpServers" key; callers pass the inner map.
	MCPServers map[string]any

	// CLAUDEMd, if non-empty, is written to CLAUDE.md at the spawn-dir root.
	CLAUDEMd string

	// Settings, if non-nil, is JSON-marshaled and written to
	// .claude/settings.json. The value is written verbatim as the file body.
	Settings map[string]any
}

// Dir is a materialized spawn directory on disk.
type Dir struct {
	// Path is the absolute path to the spawn directory.
	Path string

	// ownedTemp is true when Materialize created the directory itself and
	// is therefore responsible for removing it on Cleanup.
	ownedTemp bool
}

// Materialize writes spec into dir. If dir is empty, a fresh temp directory
// is created and owned by the returned *Dir (Cleanup will remove it). If
// dir is non-empty, it must already exist; Cleanup is a no-op in that case.
func Materialize(dir string, spec Spec) (*Dir, error) {
	d := &Dir{}
	if dir == "" {
		tmp, err := os.MkdirTemp("", "acp-claude-pty-*")
		if err != nil {
			return nil, fmt.Errorf("spawndir: create temp: %w", err)
		}
		d.Path = tmp
		d.ownedTemp = true
	} else {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("spawndir: abs path: %w", err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("spawndir: stat %s: %w", abs, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("spawndir: %s is not a directory", abs)
		}
		d.Path = abs
	}

	if err := d.write(spec); err != nil {
		// Best-effort cleanup of an owned temp dir on failure.
		if d.ownedTemp {
			_ = os.RemoveAll(d.Path)
		}
		return nil, err
	}
	return d, nil
}

func (d *Dir) write(spec Spec) error {
	for rel, body := range spec.Files {
		if err := writeRel(d.Path, rel, body); err != nil {
			return err
		}
	}
	if spec.CLAUDEMd != "" {
		if err := writeRel(d.Path, "CLAUDE.md", []byte(spec.CLAUDEMd)); err != nil {
			return err
		}
	}
	if spec.MCPServers != nil {
		body, err := json.MarshalIndent(map[string]any{"mcpServers": spec.MCPServers}, "", "  ")
		if err != nil {
			return fmt.Errorf("spawndir: marshal .mcp.json: %w", err)
		}
		if err := writeRel(d.Path, ".mcp.json", body); err != nil {
			return err
		}
	}
	if spec.Settings != nil {
		body, err := json.MarshalIndent(spec.Settings, "", "  ")
		if err != nil {
			return fmt.Errorf("spawndir: marshal settings.json: %w", err)
		}
		if err := writeRel(d.Path, ".claude/settings.json", body); err != nil {
			return err
		}
	}
	return nil
}

// Cleanup removes the spawn directory if it was created by Materialize.
// For caller-supplied directories it is a no-op.
func (d *Dir) Cleanup() error {
	if d == nil || !d.ownedTemp {
		return nil
	}
	return os.RemoveAll(d.Path)
}

// writeRel writes body to root/rel, creating parent directories as needed.
// rel must be a clean relative path that stays within root.
func writeRel(root, rel string, body []byte) error {
	if err := validateRel(rel); err != nil {
		return err
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("spawndir: mkdir for %s: %w", rel, err)
	}
	if err := os.WriteFile(full, body, 0o644); err != nil {
		return fmt.Errorf("spawndir: write %s: %w", rel, err)
	}
	return nil
}

func validateRel(rel string) error {
	if rel == "" {
		return errors.New("spawndir: empty relative path")
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return fmt.Errorf("spawndir: absolute path not allowed: %s", rel)
	}
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("spawndir: path escapes root: %s", rel)
	}
	return nil
}
