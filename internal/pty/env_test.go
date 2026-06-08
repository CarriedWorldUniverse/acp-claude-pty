package pty

import (
	"slices"
	"strings"
	"testing"
)

func TestStripBillingEnvRemovesAPIRoutingKeys(t *testing.T) {
	in := []string{
		"HOME=/home/agent",
		"ANTHROPIC_API_KEY=sk-ant-secret",
		"PATH=/usr/bin",
		"CLAUDE_CODE_USE_BEDROCK=1",
		"ANTHROPIC_BASE_URL=https://gateway.example",
		"ANTHROPIC_AUTH_TOKEN=tok",
		"CLAUDE_CODE_USE_VERTEX=1",
		"ANTHROPIC_MODEL=claude-opus-4-7", // model selection, NOT billing — must survive
	}
	got := stripBillingEnv(in)

	for _, kv := range got {
		key, _, _ := strings.Cut(kv, "=")
		if isBillingEnvKey(key) {
			t.Errorf("billing key %q survived strip", key)
		}
	}
	for _, want := range []string{"HOME=/home/agent", "PATH=/usr/bin", "ANTHROPIC_MODEL=claude-opus-4-7"} {
		if !slices.Contains(got, want) {
			t.Errorf("non-billing entry %q was dropped", want)
		}
	}
	// Input must not be mutated.
	if !slices.Contains(in, "ANTHROPIC_API_KEY=sk-ant-secret") {
		t.Error("stripBillingEnv mutated its input slice")
	}
}

func TestNewRejectsPrintFlag(t *testing.T) {
	for _, arg := range []string{"-p", "--print", "-p=hi", "--print=hi"} {
		if _, err := New(Options{Cwd: "/tmp", Args: []string{arg}}); err == nil {
			t.Errorf("New accepted headless flag %q; want error (would be Agent-SDK billed)", arg)
		}
	}
	// A normal interactive invocation is still accepted.
	if _, err := New(Options{Cwd: "/tmp", Args: []string{"--model", "opus"}}); err != nil {
		t.Errorf("New rejected a valid interactive invocation: %v", err)
	}
}
