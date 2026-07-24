package canary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchReportsTokenAndKindLabel(t *testing.T) {
	s := New([]Token{
		{Token: "REACTOR-ctx01", Kind: "context", Label: "system_prompt"},
		{Token: "REACTOR-env01", Kind: "file", Label: "dotenv"},
		{Token: "REACTOR-unlabelled", Kind: "file"},
	})

	body := `POST /collect {"a":"REACTOR-env01","b":"REACTOR-ctx01"}`
	toks, kinds := s.Match(body)
	if len(toks) != 2 {
		t.Fatalf("expected 2 matches, got %v", toks)
	}
	// tokens and kinds are parallel slices — an off-by-one here mislabels which
	// secret leaked, which is the whole content of the signal.
	for i, tok := range toks {
		switch tok {
		case "REACTOR-ctx01":
			if kinds[i] != "context:system_prompt" {
				t.Errorf("kind for ctx = %q", kinds[i])
			}
		case "REACTOR-env01":
			if kinds[i] != "file:dotenv" {
				t.Errorf("kind for env = %q", kinds[i])
			}
		default:
			t.Errorf("unexpected token %q", tok)
		}
	}

	// No label => bare kind, no trailing colon.
	toks, kinds = s.Match("REACTOR-unlabelled")
	if len(toks) != 1 || kinds[0] != "file" {
		t.Fatalf("unlabelled canary kind = %v", kinds)
	}
}

func TestMatchIsExactSubstringNotFuzzy(t *testing.T) {
	s := New([]Token{{Token: "REACTOR-a1b2c3d4", Kind: "context", Label: "system_prompt"}})
	for _, blob := range []string{
		"REACTOR-a1b2c3d5", // one char off
		"REACTOR-a1b2c3",   // truncated
		"reactor-a1b2c3d4", // wrong case
		"",
	} {
		if toks, _ := s.Match(blob); len(toks) != 0 {
			t.Errorf("Match(%q) matched %v — the sink must not report a canary that did not land", blob, toks)
		}
	}
	// Embedded in surrounding bytes still counts: exfil rarely arrives alone.
	if toks, _ := s.Match("x=REACTOR-a1b2c3d4&y=1"); len(toks) != 1 {
		t.Fatalf("embedded canary not matched: %v", toks)
	}
}

// Load sorts longest-first so a substring search reports the most specific
// match. Without it a short token that prefixes a long one wins, and the sink
// names the wrong secret.
func TestLoadOrdersLongestFirst(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "canaries.json")
	write(t, p, `[
	  {"token":"REACTOR-ab","kind":"file","label":"short"},
	  {"token":"REACTOR-abcdef","kind":"context","label":"system_prompt"}
	]`)

	s, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 2 {
		t.Fatalf("Len = %d", s.Len())
	}
	toks, kinds := s.Match("leaked REACTOR-abcdef")
	if len(toks) == 0 || toks[0] != "REACTOR-abcdef" {
		t.Fatalf("most specific match should come first, got %v (%v)", toks, kinds)
	}
	if s.Kind("REACTOR-abcdef") != "context" {
		t.Fatalf("Kind lookup = %q", s.Kind("REACTOR-abcdef"))
	}
	if s.Kind("REACTOR-nope") != "" {
		t.Fatal("Kind should be empty for an unknown token")
	}
}

// A chamber collector starts before the engine has necessarily written the
// table. A missing file must degrade to "no canaries", never to a hard failure
// that takes the sink down with it.
func TestLoadMissingFileIsEmptySet(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if s.Len() != 0 {
		t.Fatalf("Len = %d", s.Len())
	}
	if toks, _ := s.Match("anything"); toks != nil {
		t.Fatalf("empty set matched %v", toks)
	}

	s, err = Load("")
	if err != nil || s.Len() != 0 {
		t.Fatalf("empty path: %v %v", s, err)
	}
}

// Malformed JSON is a real failure — silently running with zero canaries would
// turn every detonation into a false negative. Load must say so.
func TestLoadMalformedJSONErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "canaries.json")
	write(t, p, `{not json`)
	if _, err := Load(p); err == nil {
		t.Fatal("malformed canary table must return an error, not an empty set")
	}
}

func TestNilSetIsSafe(t *testing.T) {
	s := New(nil)
	if toks, kinds := s.Match("REACTOR-anything"); toks != nil || kinds != nil {
		t.Fatalf("nil set matched: %v %v", toks, kinds)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)), 0o644); err != nil {
		t.Fatal(err)
	}
}
