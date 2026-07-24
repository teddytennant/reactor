package intake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEmpty(t *testing.T) {
	if p := Parse(""); p.Kind != Empty {
		t.Fatalf("empty: %+v", p)
	}
	if p := Parse("   "); p.Kind != Empty {
		t.Fatalf("whitespace: %+v", p)
	}
}

func TestParseRepo(t *testing.T) {
	cases := []struct {
		in, url string
	}{
		{"https://github.com/acme/notes", "https://github.com/acme/notes"},
		{"http://github.com/acme/notes", "http://github.com/acme/notes"},
		{"github.com/acme/notes", "https://github.com/acme/notes"},
		{"www.github.com/acme/notes", "https://github.com/acme/notes"},
		{"acme/notes", "https://github.com/acme/notes"},
		{"acme/notes.git", "https://github.com/acme/notes"},
	}
	for _, c := range cases {
		p := ParseWithRef(c.in, "main")
		if p.Kind != Repo || p.RepoURL != c.url || p.Ref != "main" {
			t.Errorf("Parse(%q) = %+v, want repo %q ref=main", c.in, p, c.url)
		}
	}
}

func TestParseSpec(t *testing.T) {
	p := Parse("npx -y @acme/notes-mcp")
	if p.Kind != Spec || p.SpecCommand != "npx -y @acme/notes-mcp" || p.SpecName != "@acme/notes-mcp" {
		t.Fatalf("runner: %+v", p)
	}
	p = Parse("@acme/notes-mcp")
	if p.Kind != Spec || p.SpecCommand != "npx -y @acme/notes-mcp" || p.SpecName != "@acme/notes-mcp" {
		t.Fatalf("scoped: %+v", p)
	}
	p = Parse("uvx some-tool")
	if p.Kind != Spec || p.SpecName != "some-tool" {
		t.Fatalf("uvx: %+v", p)
	}
	// Bare word that is not art_* becomes an npx package.
	p = Parse("left-pad")
	if p.Kind != Spec || p.SpecCommand != "npx -y left-pad" {
		t.Fatalf("bare package: %+v", p)
	}
}

func TestParseZooID(t *testing.T) {
	p := Parse("art_notes_mcp")
	if p.Kind != ZooID || p.ArtifactID != "art_notes_mcp" {
		t.Fatalf("zoo: %+v", p)
	}
}

func TestParseRefused(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"git@github.com:acme/x.git", "https only"},
		{"ssh://git@github.com/acme/x", "https only"},
		{"git://github.com/acme/x", "https only"},
		{"file:///tmp/repo", "Zip the directory"},
		// Three path segments is not owner/repo and not a runner command.
		{"acme/notes/extra", "Paste an https"},
	}
	for _, c := range cases {
		p := Parse(c.in)
		if p.Kind != Refused || !strings.Contains(p.Message, c.want) {
			t.Errorf("Parse(%q) = %+v, want refused mentioning %q", c.in, p, c.want)
		}
	}
}

func TestParseFileAndDirectory(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "thing.zip")
	if err := os.WriteFile(zipPath, []byte("PK\x03\x04"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Parse(zipPath)
	if p.Kind != File || p.Path == "" {
		t.Fatalf("abs file: %+v", p)
	}
	// Relative path from inside the temp dir.
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	p = Parse("./thing.zip")
	if p.Kind != File || !strings.HasSuffix(p.Path, "thing.zip") {
		t.Fatalf("rel file: %+v", p)
	}
	p = Parse(".")
	if p.Kind != Refused || !strings.Contains(p.Message, "Zip the directory") {
		t.Fatalf("dir: %+v", p)
	}
}

func TestResolvePrefersFileThenZoo(t *testing.T) {
	// Zoo-shaped token with no file on disk.
	p := Resolve("art_notes_mcp", "")
	if p.Kind != ZooID || p.ArtifactID != "art_notes_mcp" {
		t.Fatalf("zoo resolve: %+v", p)
	}
	// Unknown bare word without file → spec via Parse.
	p = Resolve("some-pkg", "")
	if p.Kind != Spec {
		t.Fatalf("bare pkg: %+v", p)
	}
}

func TestParseDoesNotStatBareZooID(t *testing.T) {
	// A same-named file in cwd must not steal art_* unless path-shaped.
	cwd, _ := os.Getwd()
	dir := t.TempDir()
	t.Cleanup(func() { os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("art_notes_mcp", []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Parse("art_notes_mcp")
	if p.Kind != ZooID {
		t.Fatalf("bare zoo id became %+v because a cwd file existed", p)
	}
}
