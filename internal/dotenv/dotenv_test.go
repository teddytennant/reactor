package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsesAndDoesNotOverrideRealEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	body := `
# a comment
XAI_API_KEY=xai-from-file
export DAYTONA_API_KEY=dt-exported
QUOTED_DOUBLE="has spaces"
QUOTED_SINGLE='also spaces'
  PADDED  =  trimmed
EMPTY=
URL=https://api.example.com/v1?a=b=c
ALREADY_SET=from-file

not-a-kv-line
=novalue
`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// A real environment variable always wins, so `FOO=x reactor` overrides .env.
	t.Setenv("ALREADY_SET", "from-env")
	for _, k := range []string{"XAI_API_KEY", "DAYTONA_API_KEY", "QUOTED_DOUBLE", "QUOTED_SINGLE", "PADDED", "EMPTY", "URL"} {
		t.Setenv(k, "") // registers cleanup...
		os.Unsetenv(k)  // ...then clears it so Load sees it as unset
	}

	Load(p)

	want := map[string]string{
		"XAI_API_KEY":     "xai-from-file",
		"DAYTONA_API_KEY": "dt-exported",
		"QUOTED_DOUBLE":   "has spaces",
		"QUOTED_SINGLE":   "also spaces",
		"PADDED":          "trimmed",
		"EMPTY":           "",
		// Only the first '=' separates; the rest belongs to the value.
		"URL":         "https://api.example.com/v1?a=b=c",
		"ALREADY_SET": "from-env",
	}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if _, ok := os.LookupEnv("not-a-kv-line"); ok {
		t.Error("a line with no '=' became a variable")
	}
	if _, ok := os.LookupEnv(""); ok {
		t.Error("a line starting with '=' produced an empty-named variable")
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	// The engine calls Load(".env", "~/.reactor.env") unconditionally; neither
	// existing is the normal case in CI.
	Load(filepath.Join(t.TempDir(), "absent.env"), filepath.Join(t.TempDir(), "also-absent"))
}

func TestLoadLaterFilesDoNotOverrideEarlier(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.env")
	second := filepath.Join(dir, "b.env")
	os.WriteFile(first, []byte("REACTOR_TEST_ORDER=first\n"), 0o600)
	os.WriteFile(second, []byte("REACTOR_TEST_ORDER=second\n"), 0o600)

	t.Setenv("REACTOR_TEST_ORDER", "")
	os.Unsetenv("REACTOR_TEST_ORDER")

	Load(first, second)
	if got := os.Getenv("REACTOR_TEST_ORDER"); got != "first" {
		t.Fatalf("REACTOR_TEST_ORDER = %q, want the first file to win", got)
	}
}
