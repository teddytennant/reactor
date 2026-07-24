package engine

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---- archive fixtures ----

// entry is one archive member, written by both the zip and the tar builders so
// a security case can be stated once and asserted against both formats.
type entry struct {
	name   string
	body   string
	mode   os.FileMode
	link   string // non-empty: write a symlink entry pointing here
	hard   string // tar only: non-empty writes a hardlink entry
	dir    bool
	device bool // tar only: a character device node
	zeroes int  // body of n zero bytes, for compression-bomb cases
}

func zipBytes(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		switch {
		case e.dir:
			mode |= os.ModeDir
		case e.link != "":
			mode |= os.ModeSymlink
		case e.device:
			mode |= os.ModeDevice | os.ModeCharDevice
		}
		hdr.SetMode(mode)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case e.link != "":
			io.WriteString(w, e.link)
		case e.zeroes > 0:
			w.Write(make([]byte, e.zeroes))
		default:
			io.WriteString(w, e.body)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tarBytes(t *testing.T, entries []entry, gzipped bool) []byte {
	t.Helper()
	var raw bytes.Buffer
	var out io.Writer = &raw
	var gz *gzip.Writer
	if gzipped {
		gz = gzip.NewWriter(&raw)
		out = gz
	}
	tw := tar.NewWriter(out)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Typeflag: tar.TypeReg}
		if e.mode != 0 {
			hdr.Mode = int64(e.mode.Perm())
		}
		body := e.body
		if e.zeroes > 0 {
			body = string(make([]byte, e.zeroes))
		}
		switch {
		case e.dir:
			hdr.Typeflag, hdr.Mode, body = tar.TypeDir, 0o755, ""
		case e.link != "":
			hdr.Typeflag, hdr.Linkname, body = tar.TypeSymlink, e.link, ""
		case e.hard != "":
			hdr.Typeflag, hdr.Linkname, body = tar.TypeLink, e.hard, ""
		case e.device:
			hdr.Typeflag, hdr.Devmajor, hdr.Devminor, body = tar.TypeChar, 1, 3, ""
		}
		hdr.Size = int64(len(body))
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		io.WriteString(tw, body)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if gz != nil {
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return raw.Bytes()
}

func stage(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "archive.bin")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func looseLimits() extractLimits { return extractLimits{MaxBytes: 8 << 20, MaxFiles: 1000} }

// ---- zip slip ----

// Zip-slip is the bug that turns "unpack this archive" into "write anywhere on
// the host as the engine's user". Every one of these names has to be refused,
// in both container formats, and the refusal has to happen before anything is
// written — so the destination is checked for strays afterwards too.
func TestExtractRefusesEntriesThatEscapeTheRoot(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"parent traversal", "../escaped.txt"},
		{"deep traversal", "../../../../etc/cron.d/pwn"},
		{"traversal mid-path", "docs/../../escaped.txt"},
		{"absolute path", "/etc/passwd"},
		{"absolute under home", "/home/agent/.ssh/authorized_keys"},
		{"drive relative", "c:/windows/system32/evil.dll"},
		{"backslash traversal", `..\..\escaped.txt`},
		{"dot only", "."},
		{"dotdot only", ".."},
		{"trailing traversal", "a/b/../../.."},
		{"empty name", ""},
	}
	for _, c := range cases {
		for _, format := range []string{archiveZip, archiveTar, archiveTarGz} {
			ents := []entry{{name: "README.md", body: "fine"}, {name: c.path, body: "pwned"}}
			var data []byte
			switch format {
			case archiveZip:
				data = zipBytes(t, ents)
			case archiveTar:
				data = tarBytes(t, ents, false)
			default:
				data = tarBytes(t, ents, true)
			}
			dest := t.TempDir()
			canary := filepath.Join(filepath.Dir(dest), "escaped.txt")
			_, err := extractArchive(stage(t, data), format, dest, looseLimits())
			if err == nil {
				t.Errorf("%s/%s: %q was extracted instead of refused", format, c.name, c.path)
				continue
			}
			if got := statusOf(err); got != http.StatusBadRequest {
				t.Errorf("%s/%s: status %d, want 400", format, c.name, got)
			}
			if !strings.Contains(err.Error(), "refusing archive") {
				t.Errorf("%s/%s: unhelpful message %q", format, c.name, err)
			}
			if _, err := os.Stat(canary); err == nil {
				t.Fatalf("%s/%s: %q escaped the destination and landed on disk", format, c.name, c.path)
			}
		}
	}
}

// safeEntryPath is the single place the escape decision is made, so the cases
// that must survive it are pinned separately from the archive plumbing.
func TestSafeEntryPathAcceptsOrdinaryNames(t *testing.T) {
	cases := map[string]string{
		"README.md":            "README.md",
		"./README.md":          "README.md",
		"src/server.mjs":       filepath.Join("src", "server.mjs"),
		"a/b/../c.txt":         filepath.Join("a", "c.txt"),
		"repo-main/pkg/x.json": filepath.Join("repo-main", "pkg", "x.json"),
		"..hidden":             "..hidden",
		"a..b/c":               filepath.Join("a..b", "c"),
	}
	for in, want := range cases {
		got, err := safeEntryPath(in)
		if err != nil {
			t.Errorf("safeEntryPath(%q) refused a legitimate name: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("safeEntryPath(%q) = %q, want %q", in, got, want)
		}
	}
	// A NUL is not a filename character anywhere, and it truncates paths in C.
	if _, err := safeEntryPath("ok\x00/../../etc/passwd"); err == nil {
		t.Error("a NUL byte in an entry name was accepted")
	}
	if _, err := safeEntryPath(strings.Repeat("a/", 600)); err == nil {
		t.Error("a 1200-character entry name was accepted")
	}
}

// ---- symlinks and other non-regular entries ----

// A symlink entry is how an archive escapes its root without any entry name
// ever containing "..": unpack `link -> /home/agent/.ssh`, then unpack
// `link/authorized_keys` and the second, innocent-looking write lands outside.
// So nothing that is not a regular file or a directory is ever materialised.
func TestExtractSkipsSymlinksAndOtherNonRegularEntries(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("host secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		entries []entry
	}{
		{"absolute symlink", []entry{{name: "keys", link: secret}, {name: "ok.txt", body: "x"}}},
		{"relative escaping symlink", []entry{{name: "up", link: "../.."}, {name: "ok.txt", body: "x"}}},
		{"symlink then write through it", []entry{
			{name: "out", link: filepath.Dir(secret)},
			{name: "out/planted.txt", body: "pwned"},
			{name: "ok.txt", body: "x"},
		}},
		{"hardlink", []entry{{name: "hard", hard: "ok.txt"}, {name: "ok.txt", body: "x"}}},
		{"device node", []entry{{name: "null", device: true}, {name: "ok.txt", body: "x"}}},
	}
	for _, c := range cases {
		for _, format := range []string{archiveZip, archiveTar} {
			if format == archiveZip && (c.name == "hardlink" || c.name == "device node") {
				continue // zip has no hardlink type; the device case is tar-shaped
			}
			var data []byte
			if format == archiveZip {
				data = zipBytes(t, c.entries)
			} else {
				data = tarBytes(t, c.entries, false)
			}
			dest := t.TempDir()
			st, err := extractArchive(stage(t, data), format, dest, looseLimits())
			if err != nil {
				t.Errorf("%s/%s: %v", format, c.name, err)
				continue
			}
			if st.Skipped == 0 {
				t.Errorf("%s/%s: nothing reported as skipped", format, c.name)
			}
			// The regular entry still lands: a hostile member must not cost the
			// user the rest of their archive.
			if _, err := os.Stat(filepath.Join(dest, "ok.txt")); err != nil {
				t.Errorf("%s/%s: the ordinary entry was lost: %v", format, c.name, err)
			}
			// Nothing under dest may be a link, and the host file is untouched.
			filepath.Walk(dest, func(p string, info os.FileInfo, err error) error {
				if err == nil && info.Mode()&os.ModeSymlink != 0 {
					t.Errorf("%s/%s: materialised a symlink at %s", format, c.name, p)
				}
				return nil
			})
			if b, err := os.ReadFile(secret); err != nil || string(b) != "host secret" {
				t.Fatalf("%s/%s: the host file outside the destination was written through: %q %v",
					format, c.name, b, err)
			}
		}
	}
}

// A hostile archive can also just ask for a setuid binary. Modes are reduced to
// 0644/0755 on the way in.
func TestExtractStripsDangerousModes(t *testing.T) {
	data := zipBytes(t, []entry{
		{name: "suid", body: "x", mode: 0o4755},
		{name: "worldwrite", body: "x", mode: 0o666},
		{name: "run.sh", body: "x", mode: 0o755},
	})
	dest := t.TempDir()
	if _, err := extractArchive(stage(t, data), archiveZip, dest, looseLimits()); err != nil {
		t.Fatal(err)
	}
	want := map[string]os.FileMode{"suid": 0o755, "worldwrite": 0o644, "run.sh": 0o755}
	for name, mode := range want {
		info, err := os.Stat(filepath.Join(dest, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Errorf("%s mode = %o, want %o", name, info.Mode().Perm(), mode)
		}
	}
}

// ---- ceilings ----

// The uncompressed size in a zip header is a claim, not a fact, so the ceiling
// is enforced against bytes actually copied. These archives are all small on
// the wire and enormous once opened — that is the whole trick.
func TestExtractEnforcesDecompressedCeilings(t *testing.T) {
	cases := []struct {
		name    string
		entries []entry
		lim     extractLimits
		want    string
	}{
		{
			name:    "one bomb entry",
			entries: []entry{{name: "bomb", zeroes: 4 << 20}},
			lim:     extractLimits{MaxBytes: 64 << 10, MaxFiles: 100},
			want:    "unpacks to more than",
		},
		{
			name: "many small entries summing past the ceiling",
			entries: func() []entry {
				var out []entry
				for i := 0; i < 40; i++ {
					out = append(out, entry{name: "f" + string(rune('a'+i%26)) + string(rune('a'+i/26)), zeroes: 8 << 10})
				}
				return out
			}(),
			lim:  extractLimits{MaxBytes: 64 << 10, MaxFiles: 1000},
			want: "unpacks to more than",
		},
		{
			name: "too many files",
			entries: func() []entry {
				var out []entry
				for i := 0; i < 50; i++ {
					out = append(out, entry{name: "f" + string(rune('a'+i%26)) + string(rune('a'+i/26)), body: "x"})
				}
				return out
			}(),
			lim:  extractLimits{MaxBytes: 8 << 20, MaxFiles: 10},
			want: "more than 10 files",
		},
	}
	for _, c := range cases {
		for _, format := range []string{archiveZip, archiveTarGz} {
			var data []byte
			if format == archiveZip {
				data = zipBytes(t, c.entries)
			} else {
				data = tarBytes(t, c.entries, true)
			}
			dest := t.TempDir()
			_, err := extractArchive(stage(t, data), format, dest, c.lim)
			if err == nil {
				t.Errorf("%s/%s: extracted without hitting the ceiling", format, c.name)
				continue
			}
			if got := statusOf(err); got != http.StatusRequestEntityTooLarge {
				t.Errorf("%s/%s: status %d, want 413 (%v)", format, c.name, got, err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s/%s: message %q does not mention %q", format, c.name, err, c.want)
			}
			// The point of a ceiling is that the disk survives it.
			if n := dirSize(dest); n > c.lim.MaxBytes+(1<<10) {
				t.Errorf("%s/%s: wrote %d bytes past a %d ceiling", format, c.name, n, c.lim.MaxBytes)
			}
		}
	}
}

// The dry run is what /api/upload validates with, so it must reach the same
// verdict as a real extraction while writing nothing at all.
func TestDryRunMatchesRealExtraction(t *testing.T) {
	good := zipBytes(t, []entry{{name: "pkg/server.mjs", body: "console.log(1)"}, {name: "pkg/package.json", body: "{}"}})
	dry, err := extractArchive(stage(t, good), archiveZip, "", looseLimits())
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	wet, err := extractArchive(stage(t, good), archiveZip, dest, looseLimits())
	if err != nil {
		t.Fatal(err)
	}
	if dry.Files != wet.Files || dry.Bytes != wet.Bytes || !reflect.DeepEqual(dry.Names, wet.Names) {
		t.Fatalf("dry run %+v != real extraction %+v", dry, wet)
	}
	// And the hostile archive is refused at dry-run time, which is the only
	// reason /api/upload can answer 400 instead of failing mid-detonation.
	bad := zipBytes(t, []entry{{name: "../pwn", body: "x"}})
	if _, err := extractArchive(stage(t, bad), archiveZip, "", looseLimits()); err == nil {
		t.Fatal("the dry run accepted a zip-slip archive")
	}
}

// ---- sniffing ----

// The filename and the client's Content-Type are attacker-controlled, so type
// is decided by content. Anything unrecognised is 415 with a message naming
// what would have worked.
func TestSniffArchiveIdentifiesByContentNotName(t *testing.T) {
	cases := []struct {
		name   string
		head   []byte
		want   string
		status int
	}{
		{"zip", zipBytes(t, []entry{{name: "a", body: "b"}}), archiveZip, 0},
		{"empty zip", zipBytes(t, nil), archiveZip, 0},
		{"tar", tarBytes(t, []entry{{name: "a", body: "b"}}, false), archiveTar, 0},
		{"tar.gz", tarBytes(t, []entry{{name: "a", body: "b"}}, true), archiveTarGz, 0},
		{"bzip2", []byte("BZh91AY&SY"), "", http.StatusUnsupportedMediaType},
		{"xz", []byte("\xfd7zXZ\x00\x00\x00"), "", http.StatusUnsupportedMediaType},
		{"zstd", []byte("\x28\xb5\x2f\xfd\x00\x00"), "", http.StatusUnsupportedMediaType},
		{"7z", []byte("7z\xbc\xaf\x27\x1c\x00"), "", http.StatusUnsupportedMediaType},
		{"rar", []byte("Rar!\x1a\x07\x00"), "", http.StatusUnsupportedMediaType},
		{"elf binary", []byte("\x7fELF\x02\x01\x01\x00"), "", http.StatusUnsupportedMediaType},
		{"plain json", []byte(`{"mcpServers":{}}`), "", http.StatusUnsupportedMediaType},
		{"empty", nil, "", http.StatusUnsupportedMediaType},
		// A zip renamed to .tar.gz, or the reverse, is still what it is.
		{"zip bytes whatever the name", []byte("PK\x03\x04rest"), archiveZip, 0},
	}
	for _, c := range cases {
		head := c.head
		if len(head) > 512 {
			head = head[:512]
		}
		got, err := sniffArchive(head)
		if c.want != "" {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			} else if got != c.want {
				t.Errorf("%s: sniffed %q, want %q", c.name, got, c.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: accepted as %q", c.name, got)
			continue
		}
		if s := statusOf(err); s != c.status {
			t.Errorf("%s: status %d, want %d", c.name, s, c.status)
		}
		if !strings.Contains(err.Error(), "zip") {
			t.Errorf("%s: message %q does not say what to upload instead", c.name, err)
		}
	}
}

// A file whose magic says zip but whose body is garbage must fail as an
// unreadable archive, not as a panic or a partial extraction.
func TestExtractRejectsCorruptArchives(t *testing.T) {
	for _, c := range []struct{ kind, body string }{
		{archiveZip, "PK\x03\x04 and then nothing that makes sense at all"},
		{archiveTarGz, "\x1f\x8b\x08\x00 not really gzip"},
		{archiveTar, strings.Repeat("x", 2048)},
	} {
		_, err := extractArchive(stage(t, []byte(c.body)), c.kind, t.TempDir(), looseLimits())
		if err == nil {
			t.Errorf("%s: corrupt archive extracted cleanly", c.kind)
			continue
		}
		if s := statusOf(err); s != http.StatusUnsupportedMediaType && s != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 415 or 400", c.kind, s)
		}
	}
}

// ---- git url policy ----

func TestNormalizeRepoURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // "" means the url must be refused
	}{
		{"github https", "https://github.com/acme/notes-mcp", "https://github.com/acme/notes-mcp"},
		{"with .git suffix", "https://github.com/acme/notes-mcp.git", "https://github.com/acme/notes-mcp.git"},
		{"nested path", "https://gitlab.com/group/sub/proj", "https://gitlab.com/group/sub/proj"},
		{"explicit port", "https://git.example.com:8443/a/b", "https://git.example.com:8443/a/b"},
		{"surrounding space", "  https://github.com/acme/x  ", "https://github.com/acme/x"},
		{"query stripped", "https://github.com/acme/x?upload-pack=evil", "https://github.com/acme/x"},
		{"fragment stripped", "https://github.com/acme/x#frag", "https://github.com/acme/x"},

		{"ssh scp form", "git@github.com:acme/notes-mcp.git", ""},
		{"ssh scheme", "ssh://git@github.com/acme/x", ""},
		{"git protocol", "git://github.com/acme/x", ""},
		{"plain http", "http://github.com/acme/x", ""},
		{"file url", "file:///etc", ""},
		{"ext transport", "ext::sh -c whoami", ""},
		{"embedded credentials", "https://user:token@github.com/acme/x", ""},
		{"no path", "https://github.com", ""},
		{"root path only", "https://github.com/", ""},
		{"no host", "https:///acme/x", ""},
		{"loopback literal", "https://127.0.0.1/x/y", ""},
		{"loopback name", "https://localhost/x/y", ""},
		{"ipv6 loopback", "https://[::1]/x/y", ""},
		{"private range", "https://10.0.0.5/x/y", ""},
		{"link local metadata", "https://169.254.169.254/latest/meta-data", ""},
		{"unspecified", "https://0.0.0.0/x/y", ""},
		{"newline smuggling", "https://github.com/acme/x\nhttps://evil/y", ""},
		{"space smuggling", "https://github.com/acme/x --upload-pack=evil", ""},
		{"leading dash", "--upload-pack=evil", ""},
		{"empty", "", ""},
		{"absurdly long", "https://github.com/" + strings.Repeat("a", 600), ""},
	}
	for _, c := range cases {
		got, err := normalizeRepoURL(c.in, false)
		if c.want == "" {
			if err == nil {
				t.Errorf("%s: %q accepted as %q", c.name, c.in, got)
				continue
			}
			if s := statusOf(err); s != http.StatusBadRequest {
				t.Errorf("%s: status %d, want 400", c.name, s)
			}
			if err.Error() == "" {
				t.Errorf("%s: empty error message", c.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %q refused: %v", c.name, c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: normalised to %q, want %q", c.name, got, c.want)
		}
	}

	// The development escape hatch widens exactly two things and nothing else.
	if _, err := normalizeRepoURL("file:///srv/repo.git", true); err != nil {
		t.Errorf("allowLocal did not permit file://: %v", err)
	}
	if _, err := normalizeRepoURL("https://127.0.0.1/a/b", true); err != nil {
		t.Errorf("allowLocal did not permit a loopback host: %v", err)
	}
	if _, err := normalizeRepoURL("git@github.com:a/b", true); err == nil {
		t.Error("allowLocal must not open the ssh path")
	}
}

func TestSafeGitRef(t *testing.T) {
	ok := []string{"", "main", "v1.2.3", "release/2024-06", "feature_x", "0123456789abcdef0123456789abcdef01234567"}
	for _, r := range ok {
		got, err := safeGitRef(r)
		if err != nil {
			t.Errorf("safeGitRef(%q) refused a legitimate ref: %v", r, err)
		} else if got != r {
			t.Errorf("safeGitRef(%q) = %q", r, got)
		}
	}
	bad := []string{
		"--upload-pack=evil",   // git reads a leading dash as another option
		"-b",                   //
		"main;rm -rf /",        // there is no shell, but the character set stays boring
		"main branch",          //
		"a..b",                 // a range, not a ref
		"../../etc",            //
		"main\nfetch",          //
		"refs/heads/main.lock", //
		"main/",                //
		"/main",                //
		strings.Repeat("a", 300),
	}
	for _, r := range bad {
		if _, err := safeGitRef(r); err == nil {
			t.Errorf("safeGitRef(%q) accepted a hostile ref", r)
		}
	}
}

// The clone argv is the whole security boundary for the git path: no shell
// string is ever built, so what matters is that the flags which disable
// execution are present and that the url can never be read as an option.
func TestGitCloneArgs(t *testing.T) {
	args := gitCloneArgs("https://github.com/acme/x", "main", "/w/src/repo", false)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-c core.hooksPath=/dev/null", // no hook may run
		"--template=",                 // and no template dir arrives holding one
		"-c credential.helper=",       // the host's credentials stay the host's
		"-c protocol.allow=never",     // an insteadOf rewrite cannot switch transport
		"-c protocol.https.allow=always",
		"clone --depth 1 --single-branch --no-tags",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("clone argv is missing %q: %v", want, args)
		}
	}
	if strings.Contains(joined, "--recurse-submodules") {
		t.Error("submodules would let the repository name a second remote to fetch")
	}
	// The url and the destination must be positional, after a bare "--".
	if n := len(args); args[n-3] != "--" || args[n-2] != "https://github.com/acme/x" || args[n-1] != "/w/src/repo" {
		t.Errorf("url and dest are not the last two args after --: %v", args[n-4:])
	}
	// A ref becomes --branch, and only when one was given.
	if !strings.Contains(joined, "--branch main") {
		t.Errorf("ref not passed as --branch: %v", args)
	}
	if strings.Contains(strings.Join(gitCloneArgs("https://h/a/b", "", "/d", false), " "), "--branch") {
		t.Error("--branch passed with no ref")
	}
	// file:// only becomes reachable with the development flag set.
	if strings.Contains(joined, "protocol.file.allow") {
		t.Error("the file transport is enabled by default")
	}
	if !strings.Contains(strings.Join(gitCloneArgs("file:///r", "", "/d", true), " "), "protocol.file.allow=always") {
		t.Error("allowLocal did not enable the file transport")
	}
	// And nothing anywhere in the vector is a shell.
	for _, a := range args {
		if a == "sh" || a == "-c" && false {
			t.Fatalf("a shell appears in the clone argv: %v", args)
		}
	}
}

func TestCloneEnvIsBuiltFromNothing(t *testing.T) {
	env := cloneEnv("/scratch/githome")
	want := map[string]string{
		"HOME":                "/scratch/githome",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_CONFIG_GLOBAL":   "/dev/null",
	}
	got := map[string]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			got[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("clone env %s = %q, want %q", k, got[k], v)
		}
	}
	// Anything the operator happens to have exported must not survive: only PATH
	// is carried across, and only because git needs it to find its helpers.
	for k := range got {
		switch k {
		case "PATH", "HOME", "LC_ALL":
		default:
			if !strings.HasPrefix(k, "GIT_") && !strings.HasPrefix(k, "SSH_") {
				t.Errorf("unexpected variable %q in the clone environment", k)
			}
		}
	}
}

// ---- clone, end to end ----

// A real clone, off a local repository, with the development flag on. This is
// the only way to exercise the size ceiling, the .git removal and the digest
// without a network.
func TestCloneRepoEndToEnd(t *testing.T) {
	git := gitOrSkip(t)
	origin := makeRepo(t, git, map[string]string{
		"server.mjs":   "console.log('hi')",
		"package.json": `{"name":"x"}`,
		"docs/a.md":    "hello",
	})

	e := newTestEngine(t)
	e.cfg.AllowLocalRepos = true
	dest := filepath.Join(t.TempDir(), "src", "repo")
	os.MkdirAll(filepath.Dir(dest), 0o700)

	if err := e.cloneRepo(t.Context(), "file://"+origin, "", dest); err != nil {
		t.Fatal(err)
	}
	// .git carries hooks and packfiles; neither belongs in a chamber.
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		t.Error(".git survived the clone")
	}
	sum, st, err := sealTree(dest, looseLimits())
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 3 {
		t.Errorf("sealed %d files, want 3: %v", st.Files, st.Names)
	}
	if len(sum) != 64 {
		t.Errorf("digest %q is not a sha256", sum)
	}
	// The digest is over the tree's real contents, so a second clone of the same
	// commit matches and a changed byte does not.
	dest2 := filepath.Join(t.TempDir(), "src", "repo")
	os.MkdirAll(filepath.Dir(dest2), 0o700)
	if err := e.cloneRepo(t.Context(), "file://"+origin, "", dest2); err != nil {
		t.Fatal(err)
	}
	sum2, _, _ := sealTree(dest2, looseLimits())
	if sum != sum2 {
		t.Errorf("two clones of one commit hashed differently: %s vs %s", sum, sum2)
	}
	os.WriteFile(filepath.Join(dest2, "server.mjs"), []byte("console.log('changed')"), 0o644)
	sum3, _, _ := sealTree(dest2, looseLimits())
	if sum == sum3 {
		t.Error("changing a file did not change the tree digest")
	}
}

// A clone that would fill the disk is killed while it runs, not audited
// afterwards.
func TestCloneRepoEnforcesSizeCeiling(t *testing.T) {
	git := gitOrSkip(t)
	origin := makeRepo(t, git, map[string]string{"big.bin": strings.Repeat("A", 2<<20)})

	e := newTestEngine(t)
	e.cfg.AllowLocalRepos = true
	e.cfg.MaxCloneBytes = 4 << 10
	dest := filepath.Join(t.TempDir(), "src", "repo")
	os.MkdirAll(filepath.Dir(dest), 0o700)

	// Whether the watchdog kills it mid-flight or the post-clone measurement
	// catches it, an oversize repository never reaches a chamber.
	err := e.cloneRepo(t.Context(), "file://"+origin, "", dest)
	if err == nil {
		t.Fatal("a 2 MiB repository cloned under a 4 KiB ceiling")
	}
	if s := statusOf(err); s != http.StatusRequestEntityTooLarge && s != http.StatusBadRequest {
		t.Errorf("status %d, want 413 (or 400 if git noticed the kill first): %v", s, err)
	}
	if !strings.Contains(err.Error(), "larger than") && !strings.Contains(err.Error(), "clone failed") {
		t.Errorf("unhelpful message %q", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("the aborted clone was left on disk")
	}
}

func TestCloneRepoTimesOut(t *testing.T) {
	gitOrSkip(t)
	e := newTestEngine(t)
	e.cfg.AllowLocalRepos = true
	e.cfg.CloneTimeout = 150 * time.Millisecond
	dest := filepath.Join(t.TempDir(), "src", "repo")
	os.MkdirAll(filepath.Dir(dest), 0o700)

	// A path that does not exist makes git fail fast; a blackholed address makes
	// it hang. Use the latter so the deadline is what ends it.
	err := e.cloneRepo(t.Context(), "https://10.255.255.1/acme/x", "", dest)
	if err == nil {
		t.Fatal("clone of an unreachable host succeeded")
	}
	if s := statusOf(err); s != http.StatusGatewayTimeout && s != http.StatusBadRequest {
		t.Errorf("status %d, want 504 (or 400 if the connection was refused outright): %v", s, err)
	}
	// Whatever happened, nothing was left behind and no host path leaked out.
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("the failed clone was left on disk")
	}
	if strings.Contains(err.Error(), t.TempDir()) {
		t.Errorf("the error leaks a host path: %q", err)
	}
}

// sealTree deletes what UploadDir would otherwise follow out of the sandbox.
func TestSealTreeRemovesSymlinks(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "id_rsa")
	os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600)

	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644)
	if err := os.Symlink(secret, filepath.Join(root, "stolen")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, st, err := sealTree(root, looseLimits())
	if err != nil {
		t.Fatal(err)
	}
	if st.Skipped != 1 || st.Files != 1 {
		t.Fatalf("sealTree stats = %+v, want 1 file and 1 skipped", st)
	}
	if _, err := os.Lstat(filepath.Join(root, "stolen")); err == nil {
		t.Error("a symlink out of the tree survived sealing")
	}
	if b, _ := os.ReadFile(secret); string(b) != "PRIVATE KEY" {
		t.Fatal("sealTree followed the symlink and touched the target")
	}
}

// ---- inference ----

func TestEntrypointInference(t *testing.T) {
	cases := []struct {
		name                       string
		names                      []string
		kind, source, install      string
		wantKind, wantSrc, wantIns string
	}{
		{
			name: "mcp server at the root", names: []string{"server.mjs", "package.json", "README.md"},
			wantKind: "mcp_server", wantSrc: "node server.mjs", wantIns: "npm install",
		},
		{
			name: "wrapped in a repo folder", names: []string{"notes-mcp-main/server.mjs", "notes-mcp-main/package.json"},
			wantKind: "mcp_server", wantSrc: "node server.mjs", wantIns: "npm install",
		},
		{
			name: "python server", names: []string{"server.py", "requirements.txt"},
			wantKind: "mcp_server", wantSrc: "python3 server.py",
		},
		{
			name: "zip payload", names: []string{"index.mjs", "package.json"},
			wantKind: "zip", wantSrc: "node index.mjs", wantIns: "npm install",
		},
		{
			name: "skill", names: []string{"SKILL.md", "scripts/collect.sh"},
			wantKind: "skill",
		},
		{
			name: "nothing recognisable", names: []string{"notes.txt", "data/x.csv"},
			wantKind: "zip",
		},
		{
			name: "two wrapper dirs is not a wrapper", names: []string{"a/server.mjs", "b/server.mjs"},
			wantKind: "zip",
		},
	}
	for _, c := range cases {
		kind, source, install := entrypoint(c.names)
		if kind != c.wantKind || source != c.wantSrc || install != c.wantIns {
			t.Errorf("%s: entrypoint = (%q, %q, %q), want (%q, %q, %q)",
				c.name, kind, source, install, c.wantKind, c.wantSrc, c.wantIns)
		}
	}
}

func TestCollapseRoot(t *testing.T) {
	// One wrapper directory is unwrapped...
	wrapped := t.TempDir()
	os.MkdirAll(filepath.Join(wrapped, "repo-main", "src"), 0o755)
	if got := collapseRoot(wrapped); got != filepath.Join(wrapped, "repo-main") {
		t.Errorf("collapseRoot did not unwrap: %q", got)
	}
	// ...but a real artifact layout is left exactly where it is.
	flat := t.TempDir()
	os.WriteFile(filepath.Join(flat, "server.mjs"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(flat, "lib"), 0o755)
	if got := collapseRoot(flat); got != flat {
		t.Errorf("collapseRoot unwrapped a flat layout: %q", got)
	}
}

func TestSafeBaseName(t *testing.T) {
	cases := map[string]string{
		"notes-mcp.zip":      "notes-mcp.zip",
		"../../etc/passwd":   "passwd",
		"/abs/path/x.tar.gz": "x.tar.gz",
		// Backslash is an ordinary filename byte on this host, not a separator;
		// what matters is that it cannot survive as one.
		`..\..\windows\evil`:           "windows_evil",
		"weird name (1).zip":           "weird_name__1_.zip",
		"":                             "artifact",
		"..":                           "artifact",
		"...":                          "artifact",
		"a" + strings.Repeat("b", 200): "a" + strings.Repeat("b", 95),
	}
	for in, want := range cases {
		if got := safeBaseName(in); got != want {
			t.Errorf("safeBaseName(%q) = %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"a/b", "..", "../..", `x\y`, "~/.ssh/id_rsa"} {
		got := safeBaseName(in)
		if strings.ContainsAny(got, `/\`) || got == ".." || got == "." {
			t.Errorf("safeBaseName(%q) = %q is not a safe single path element", in, got)
		}
	}
}

// ---- helpers ----

func gitOrSkip(t *testing.T) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	return git
}

// makeRepo builds a one-commit repository on disk and returns its path.
func makeRepo(t *testing.T, git string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		cmd.Env = append(cloneEnv(t.TempDir()),
			"GIT_AUTHOR_NAME=reactor", "GIT_AUTHOR_EMAIL=r@example.invalid",
			"GIT_COMMITTER_NAME=reactor", "GIT_COMMITTER_EMAIL=r@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "main")
	for name, body := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "-A")
	run("commit", "-q", "-m", "initial")
	return dir
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
