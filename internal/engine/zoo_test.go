package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// loadZoo resolves each catalog entry to the directory holding it by walking for
// reactor.json manifests, because ids use underscores while folders use hyphens
// and benign controls live one level deeper. Guessing the folder from the id is
// exactly the bug this test pins down.
func TestLoadZooResolvesDirsFromManifestsNotIDs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "notes-mcp", "reactor.json"), `{"id":"art_notes_mcp"}`)
	writeFile(t, filepath.Join(root, "benign", "echo-mcp", "reactor.json"), `{"id":"art_echo_mcp"}`)
	writeFile(t, filepath.Join(root, "index.json"), `[
	  {"id":"art_notes_mcp","kind":"mcp_server","name":"notes-mcp","source":"node server.mjs",
	   "label":"rug_pull","family":"supply-chain","expect":["rug_pull","context_exfil"],
	   "static_blind":true,"lead":true},
	  {"id":"art_echo_mcp","kind":"mcp_server","name":"echo-mcp","source":"node server.mjs","family":"benign"},
	  {"id":"art_dropper_zip","kind":"zip","name":"dropper","source":"node index.mjs",
	   "install":"npm install","dir":"dropper-zip"},
	  {"kind":"mcp_server","name":"Live Pin @acme/x","source":"npx -y @acme/x","live":true}
	]`)

	zoo, err := loadZoo(filepath.Join(root, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(zoo) != 4 {
		t.Fatalf("loaded %d artifacts, want 4", len(zoo))
	}
	byID := map[string]int{}
	for i, a := range zoo {
		byID[a.ID] = i
	}

	// Discovered one level deep, under a hyphenated folder.
	if got := zoo[byID["art_notes_mcp"]].Env["_dir"]; got != filepath.Join(root, "notes-mcp") {
		t.Errorf("art_notes_mcp _dir = %q", got)
	}
	// Discovered two levels deep, under benign/.
	if got := zoo[byID["art_echo_mcp"]].Env["_dir"]; got != filepath.Join(root, "benign", "echo-mcp") {
		t.Errorf("art_echo_mcp _dir = %q", got)
	}
	// An explicit "dir" wins even with no manifest on disk.
	dropper := zoo[byID["art_dropper_zip"]]
	if got := dropper.Env["_dir"]; got != filepath.Join(root, "dropper-zip") {
		t.Errorf("art_dropper_zip _dir = %q", got)
	}
	if dropper.Env["_install"] != "npm install" {
		t.Errorf("install step lost: %q", dropper.Env["_install"])
	}
	// An entry with no id gets one derived from its name, and a live npx pin has
	// no directory at all — it runs straight from `source`.
	live := zoo[3]
	if live.ID != "art_live_pin__acme_x" {
		t.Errorf("derived id = %q", live.ID)
	}
	if _, ok := live.Env["_dir"]; ok {
		t.Errorf("live pin should have no _dir, got %q", live.Env["_dir"])
	}
	if live.Env["_live"] != "1" {
		t.Errorf("live flag lost: %v", live.Env)
	}

	// Ground truth and eval metadata ride along in Env, never in the analyst view.
	notes := zoo[byID["art_notes_mcp"]]
	if notes.Label != "rug_pull" || notes.Env["_family"] != "supply-chain" {
		t.Errorf("ground truth lost: label=%q family=%q", notes.Label, notes.Env["_family"])
	}
	if notes.Env["_expect"] != "rug_pull,context_exfil" || notes.Env["_static_blind"] != "1" || notes.Env["_lead"] != "1" {
		t.Errorf("eval metadata wrong: %v", notes.Env)
	}
	if zoo[byID["art_echo_mcp"]].Env["_static_blind"] != "" {
		t.Error("static_blind should be absent when false, not empty-string-set")
	}
}

func TestLoadZooMissingOrMalformed(t *testing.T) {
	if _, err := loadZoo(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Error("missing catalog should error")
	}
	p := filepath.Join(t.TempDir(), "index.json")
	writeFile(t, p, `{"not":"an array"}`)
	if _, err := loadZoo(p); err == nil {
		t.Error("malformed catalog should error")
	}
}

// discoverManifests must not be derailed by a manifest with no id, or by
// unreadable JSON somewhere in the tree — the rest of the zoo still loads.
func TestDiscoverManifestsSkipsUnusableEntries(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "good", "reactor.json"), `{"id":"art_good"}`)
	writeFile(t, filepath.Join(root, "noid", "reactor.json"), `{"name":"no id here"}`)
	writeFile(t, filepath.Join(root, "broken", "reactor.json"), `{{{`)

	dirs := discoverManifests(root)
	want := map[string]string{"art_good": filepath.Join(root, "good")}
	if !reflect.DeepEqual(dirs, want) {
		t.Fatalf("discoverManifests = %v, want %v", dirs, want)
	}
}

// splitFields turns an artifact's `source` string into argv. Quoting matters:
// a server launched as `node "my server.mjs"` must stay one argument.
func TestSplitFields(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"node server.mjs", []string{"node", "server.mjs"}},
		{"  npx   -y   @acme/notes-mcp  ", []string{"npx", "-y", "@acme/notes-mcp"}},
		{`node "my server.mjs"`, []string{"node", "my server.mjs"}},
		{`sh -c "node a.mjs && node b.mjs"`, []string{"sh", "-c", "node a.mjs && node b.mjs"}},
		{"", nil},
	}
	for _, c := range cases {
		if got := splitFields(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitFields(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// sanitize derives an artifact id from a name. The id ends up in evidence ids
// and in chamber directory names, so it must collapse to a lowercase,
// separator-free token — anything that could traverse a path is dropped, not
// escaped.
func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"notes-mcp":         "notes_mcp",
		"@acme/Notes MCP":   "acme_notes_mcp",
		"Dropper (zip)":     "dropper_zip",
		"...":               "",
		"Server v1.2":       "server_v1_2",
		"/../../etc/passwd": "etc_passwd",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	// Whatever it produces must never be able to walk out of a chamber directory.
	for _, in := range []string{"a/b", "..", "../..", "x\\y", "a b", "~/.ssh/id_rsa"} {
		got := sanitize(in)
		if strings.ContainsAny(got, `/\`) || got == ".." || got == "." {
			t.Errorf("sanitize(%q) = %q is not a safe single path element", in, got)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
