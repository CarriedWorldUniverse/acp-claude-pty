// Command acp-claude-pty drives the Claude Code CLI inside a PTY and
// exposes the surface over the Agent Client Protocol (ACP) on stdio.
//
// Usage:
//
//	acp-claude-pty --cwd <spawn-dir> [--command <claude-path>] [--log <path>]
//
// The binary spawns one persistent claude REPL in the supplied spawn
// directory and serves ACP over stdin/stdout until the peer disconnects.
// Spawn-directory contents (CLAUDE.md, .mcp.json, .claude/settings.json,
// SpawnFiles) are the caller's responsibility — see
// internal/spawndir for the materialization helper if needed.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/acpserver"
	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/pty"
	"github.com/CarriedWorldUniverse/acp-claude-pty/internal/version"
)

func main() {
	cwd := flag.String("cwd", "", "spawn directory for claude (required)")
	command := flag.String("command", "claude", "claude binary to launch")
	logPath := flag.String("log", "", "path to write a stdout-log copy of every PTY byte (optional)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("acp-claude-pty %s\n", version.Version)
		return
	}

	if *cwd == "" {
		fmt.Fprintln(os.Stderr, "acp-claude-pty: --cwd is required")
		os.Exit(2)
	}

	opts := pty.Options{
		Command: *command,
		Cwd:     *cwd,
	}
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "acp-claude-pty: open --log:", err)
			os.Exit(1)
		}
		defer f.Close()
		opts.StdoutLog = f
	}

	drv, err := pty.New(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "acp-claude-pty:", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := acpserver.New(drv)
	if err := srv.Serve(ctx, os.Stdout, os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "acp-claude-pty:", err)
		os.Exit(1)
	}
}
