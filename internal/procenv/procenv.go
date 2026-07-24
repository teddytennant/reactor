// Package procenv keeps host credentials out of the blast radius. Even in the
// local chamber (same machine as the host), the artifact and the MCP server it
// runs must never inherit the operator's cloud keys — a hosted-victim key, a
// Daytona token, real AWS creds. SPEC §1.2's whole argument is that the victim
// holds nothing worth stealing; that only holds if we actually strip the keys
// before spawning anything untrusted.
package procenv

import "strings"

// sensitive prefixes never passed to an artifact or MCP server subprocess.
var sensitive = []string{
	"XAI_", "GROK_", "VICTIM_API", "OPENAI_", "ANTHROPIC_", "CLAUDE_",
	"DAYTONA_", "HF_TOKEN", "HUGGING", "GH_TOKEN", "GITHUB_TOKEN",
	"AWS_ACCESS", "AWS_SECRET", "AWS_SESSION", "GOOGLE_API", "GEMINI",
	"SLACK_", "STRIPE_", "OPENROUTER", "TOGETHER_API", "GROQ_API", "FIREWORKS",
}

// Sanitize returns env with sensitive variables removed. Reactor's own
// REACTOR_* and benign runtime vars (HOME, PATH, sink url…) pass through.
func Sanitize(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if isSensitive(kv) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func isSensitive(kv string) bool {
	name := kv
	if i := strings.IndexByte(kv, '='); i >= 0 {
		name = kv[:i]
	}
	up := strings.ToUpper(name)
	for _, p := range sensitive {
		if strings.HasPrefix(up, p) {
			return true
		}
	}
	return false
}
