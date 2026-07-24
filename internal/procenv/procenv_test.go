package procenv

import (
	"strings"
	"testing"
)

// SPEC §1.2's containment argument is "the victim holds nothing worth stealing".
// That is only true if the operator's real keys are actually stripped before an
// artifact subprocess is spawned — the local chamber runs on the same machine as
// the host, so a leaked key here is a real key in an attacker's hands.
func TestSanitizeStripsHostCredentials(t *testing.T) {
	mustStrip := []string{
		"XAI_API_KEY=xai-live-abc",
		"GROK_API_KEY=g-1",
		"VICTIM_API_KEY=v-1",
		"OPENAI_API_KEY=sk-live",
		"ANTHROPIC_API_KEY=sk-ant-live",
		"CLAUDE_CODE_TOKEN=t",
		"DAYTONA_API_KEY=dt-1",
		"HF_TOKEN=hf_1",
		"HUGGINGFACE_TOKEN=hf_2",
		"GH_TOKEN=ghp_1",
		"GITHUB_TOKEN=ghp_2",
		"AWS_ACCESS_KEY_ID=AKIA1",
		"AWS_SECRET_ACCESS_KEY=s3cret",
		"AWS_SESSION_TOKEN=st",
		"GOOGLE_API_KEY=g",
		"GEMINI_API_KEY=g",
		"SLACK_BOT_TOKEN=xoxb",
		"STRIPE_SECRET_KEY=sk_live",
		"OPENROUTER_API_KEY=or",
		"TOGETHER_API_KEY=tg",
		"GROQ_API_KEY=gq",
		"FIREWORKS_API_KEY=fw",
		// Case must not be an escape hatch: the check upper-cases the name.
		"xai_api_key=lowercase-leak",
	}
	mustKeep := []string{
		"HOME=/home/agent",
		"PATH=/usr/bin",
		"REACTOR_SESSION=4",
		"REACTOR_SINK_HTTP=http://127.0.0.1:9931",
		"REACTOR_CANARY_CONTEXT=REACTOR-a1b2",
		"NODE_ENV=production",
	}

	got := Sanitize(append(append([]string{}, mustStrip...), mustKeep...))
	joined := strings.Join(got, "\n")

	for _, kv := range mustStrip {
		name := kv[:strings.IndexByte(kv, '=')]
		for _, out := range got {
			if strings.HasPrefix(strings.ToUpper(out), strings.ToUpper(name)+"=") {
				t.Errorf("credential %s survived Sanitize", name)
			}
		}
	}
	for _, kv := range mustKeep {
		if !strings.Contains(joined, kv) {
			t.Errorf("Sanitize dropped a required runtime var: %s", kv)
		}
	}
	if len(got) != len(mustKeep) {
		t.Errorf("Sanitize returned %d vars, want exactly the %d benign ones: %v", len(got), len(mustKeep), got)
	}
}

// Only the *name* decides. A benign variable whose value happens to mention a
// provider must survive, and a variable that merely contains a sensitive
// substring mid-name is not a credential.
func TestSanitizeMatchesNamePrefixOnly(t *testing.T) {
	in := []string{
		"REACTOR_TASK=summarize the openai_api_key handling in this repo",
		"MY_OPENAI_KEY=not-a-real-prefix",
		"OPENAI_API_KEY=sk-live",
	}
	got := Sanitize(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 survivors, got %v", got)
	}
	for _, kv := range got {
		if strings.HasPrefix(kv, "OPENAI_") {
			t.Fatalf("OPENAI_-prefixed var survived: %s", kv)
		}
	}
}

// A malformed entry with no '=' must not panic or be silently reclassified.
func TestSanitizeHandlesMalformedEntries(t *testing.T) {
	got := Sanitize([]string{"NOEQUALS", "XAI_NOEQUALS", "=novalue", "HOME=/h"})
	want := map[string]bool{"NOEQUALS": true, "=novalue": true, "HOME=/h": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, kv := range got {
		if !want[kv] {
			t.Fatalf("unexpected survivor %q in %v", kv, got)
		}
	}
}
