package pty

import "strings"

// billingEnvKeys are the environment variables that route the Claude Code CLI
// onto API-metered billing (an API key, a custom gateway/auth token, or a
// cloud provider) instead of the interactive Pro/Max subscription.
//
// acp-claude-pty exists to hold an INTERACTIVE subscription session, so these
// are stripped from the spawned child's environment by default
// (Options.PreserveAPIEnv opts back in).
//
// Why this matters: from 2026-06-15 Anthropic bills Agent-SDK / headless
// (`claude -p`) usage against a separate API-rate credit pool; interactive
// sessions stay on the subscription — but only if the CLI actually authenticates
// via the subscription OAuth. An inherited ANTHROPIC_API_KEY (or a Bedrock/Vertex/
// gateway override) silently flips even an interactive session onto API billing,
// defeating the entire point of this adapter. Stripping them structurally
// guarantees the subscription path.
var billingEnvKeys = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_BASE_URL",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX",
}

// stripBillingEnv returns env (a KEY=VALUE slice, os.Environ() shape) with every
// billingEnvKeys entry removed. The order of the remaining entries is preserved
// and the input slice is not mutated.
func stripBillingEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if isBillingEnvKey(key) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func isBillingEnvKey(key string) bool {
	for _, k := range billingEnvKeys {
		if key == k {
			return true
		}
	}
	return false
}
