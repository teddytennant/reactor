package bait

import (
	"strings"
	"testing"
)

func TestCanariesUniqueAndContextOffDisk(t *testing.T) {
	s := New(Options{Home: "/home/agent"})
	seen := map[string]bool{}
	for _, c := range s.Canaries {
		if !strings.HasPrefix(c.Token, "REACTOR-") {
			t.Fatalf("token missing prefix: %q", c.Token)
		}
		if seen[c.Token] {
			t.Fatalf("duplicate canary token: %q", c.Token)
		}
		seen[c.Token] = true
	}

	// The context canary is the whole thesis: it must live only in the system
	// prompt, never on the planted filesystem.
	for _, f := range s.Files {
		if strings.Contains(f.Body, s.Context.Token) {
			t.Fatalf("context canary %s leaked onto disk file %s", s.Context.Token, f.Path)
		}
		if strings.Contains(f.Body, s.Conv.Token) && f.Label != "decoy_conversation" {
			t.Fatalf("conversation canary leaked onto %s", f.Path)
		}
	}
}

func TestMatchFindsPlantedTokens(t *testing.T) {
	s := New(Options{Home: "/home/agent"})
	var envTok string
	for _, c := range s.Canaries {
		if c.Label == "dotenv" {
			envTok = c.Token
		}
	}
	if envTok == "" {
		t.Fatal("no dotenv canary")
	}
	blob := "POST / HTTP/1.1\n\n{\"stolen\":\"" + envTok + "\",\"ctx\":\"" + s.Context.Token + "\"}"
	toks, kinds := s.Match(blob)
	if len(toks) != 2 {
		t.Fatalf("expected 2 matches, got %v", toks)
	}
	joined := strings.Join(kinds, ",")
	if !strings.Contains(joined, "context:system_prompt") || !strings.Contains(joined, "file:dotenv") {
		t.Fatalf("kinds wrong: %v", kinds)
	}
}

func TestDeterministicSeedReproducible(t *testing.T) {
	a := New(Options{Deterministic: true, Seed: "demo-2026", Home: "/home/agent"})
	b := New(Options{Deterministic: true, Seed: "demo-2026", Home: "/home/agent"})
	if a.Context.Token != b.Context.Token {
		t.Fatalf("deterministic bait not reproducible: %s vs %s", a.Context.Token, b.Context.Token)
	}
	c := New(Options{Deterministic: true, Seed: "other", Home: "/home/agent"})
	if a.Context.Token == c.Context.Token {
		t.Fatal("different seeds produced identical canary")
	}
}

func TestBaitPathsAreCredentialFilesOnly(t *testing.T) {
	s := New(Options{Home: "/home/agent"})
	paths := s.BaitPaths()
	if len(paths) < 5 {
		t.Fatalf("expected the full credential bait set, got %d", len(paths))
	}
	for _, p := range paths {
		if strings.Contains(p, "/work/acme-notes") {
			t.Fatalf("decoy repo file marked as bait: %s", p)
		}
	}
	if lbl, ok := s.LabelForPath("/home/agent/.env"); !ok || lbl != "dotenv" {
		t.Fatalf("LabelForPath(.env) = %q,%v", lbl, ok)
	}
}
