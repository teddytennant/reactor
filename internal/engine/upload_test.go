package engine

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reactor-sec/reactor/internal/events"
)

// upload posts a multipart body the way a browser's FormData does, and returns
// the recorder so a test can assert on the status as well as the body.
func upload(t *testing.T, e *Engine, filename string, body []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	if filename != "" {
		w, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(body)
	}
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.Handler().ServeHTTP(rec, r)
	return rec
}

func uploadOK(t *testing.T, e *Engine, filename string, body []byte, fields map[string]string) UploadResponse {
	t.Helper()
	rec := upload(t, e, filename, body, fields)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload = %d: %s", rec.Code, rec.Body.String())
	}
	var res UploadResponse
	decode(t, rec.Body.Bytes(), &res)
	return res
}

// The response is the whole contract the web client codes against: an opaque
// id, the real digest, and an artifact it can post straight back to
// /api/detonate.
func TestUploadStagesAnArchiveAndReturnsARealDigest(t *testing.T) {
	e := newTestEngine(t)
	archive := zipBytes(t, []entry{
		{name: "notes-mcp/server.mjs", body: "console.log('hi')"},
		{name: "notes-mcp/package.json", body: `{"name":"notes-mcp"}`},
		{name: "notes-mcp/README.md", body: "# notes"},
	})

	res := uploadOK(t, e, "notes-mcp.zip", archive, nil)

	if !strings.HasPrefix(res.UploadID, "up_") {
		t.Errorf("upload_id = %q, want an up_ prefix", res.UploadID)
	}
	// The digest is of the bytes that arrived, not of anything the client said.
	if res.SHA256 != sha256Hex(archive) {
		t.Errorf("sha256 = %q, want %q", res.SHA256, sha256Hex(archive))
	}
	if res.SizeBytes != int64(len(archive)) {
		t.Errorf("size_bytes = %d, want %d", res.SizeBytes, len(archive))
	}
	if res.Archive != archiveZip || res.Name != "notes-mcp.zip" {
		t.Errorf("archive/name = %q/%q", res.Archive, res.Name)
	}
	if res.Files != 3 || res.UnpackedBytes == 0 {
		t.Errorf("files/unpacked = %d/%d", res.Files, res.UnpackedBytes)
	}
	// Entrypoint inference reaches through the wrapper directory.
	if res.Kind != events.KindMCPServer || res.Source != "node server.mjs" || res.Install != "npm install" {
		t.Errorf("inference = %q %q %q", res.Kind, res.Source, res.Install)
	}
	if res.Artifact.SHA256 != res.SHA256 || res.Artifact.Kind != res.Kind || res.Artifact.Source != res.Source {
		t.Errorf("the artifact does not match the top-level fields: %+v", res.Artifact)
	}
	// No host path may reach the browser.
	blob := mustJSON(t, res)
	if strings.Contains(blob, e.workRoot) || strings.Contains(blob, os.TempDir()) {
		t.Fatalf("the upload response leaks a host path: %s", blob)
	}

	if rec := do(t, e, "GET", "/api/upload", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/upload = %d, want 405", rec.Code)
	}
}

// Fields let the UI correct the guess without a second round trip.
func TestUploadHonoursFormFieldOverrides(t *testing.T) {
	e := newTestEngine(t)
	archive := tarBytes(t, []entry{{name: "run.sh", body: "echo hi"}}, true)

	res := uploadOK(t, e, "bundle.tgz", archive, map[string]string{
		"kind": events.KindSkill, "name": "my skill", "source": "bash run.sh", "install": "make",
	})
	if res.Kind != events.KindSkill || res.Source != "bash run.sh" || res.Install != "make" {
		t.Errorf("overrides ignored: %+v", res)
	}
	if res.Name != "my_skill" {
		t.Errorf("name = %q, want the sanitised form", res.Name)
	}
	if res.Archive != archiveTarGz {
		t.Errorf("archive = %q, want %q", res.Archive, archiveTarGz)
	}
	if rec := upload(t, e, "x.zip", zipBytes(t, []entry{{name: "a", body: "b"}}), map[string]string{"kind": "rootkit"}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown kind = %d, want 400", rec.Code)
	}
}

// Every rejection path, with the status the web UI will branch on.
func TestUploadRejectionsCarryTheRightStatus(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.MaxUploadBytes = 32 << 10
	e.cfg.MaxExtractBytes = 64 << 10
	e.cfg.MaxExtractFiles = 8

	cases := []struct {
		name   string
		file   string
		body   []byte
		status int
		want   string
	}{
		{"not an archive", "servers.json", []byte(`{"mcpServers":{}}`), http.StatusUnsupportedMediaType, "zip"},
		{"bzip2", "x.tar.bz2", []byte("BZh91AY&SY" + strings.Repeat("x", 64)), http.StatusUnsupportedMediaType, "bzip2"},
		{"empty file", "empty.zip", nil, http.StatusBadRequest, "empty"},
		{"oversize file", "big.zip", append([]byte("PK\x03\x04"), bytes.Repeat([]byte("x"), 64<<10)...), http.StatusRequestEntityTooLarge, "larger than"},
		{"zip slip", "evil.zip", zipBytes(t, []entry{{name: "../../pwn", body: "x"}}), http.StatusBadRequest, "refusing archive"},
		{"zip bomb", "bomb.zip", zipBytes(t, []entry{{name: "bomb", zeroes: 1 << 20}}), http.StatusRequestEntityTooLarge, "unpacks to more than"},
		{"too many files", "many.zip", zipBytes(t, func() []entry {
			var out []entry
			for i := 0; i < 20; i++ {
				out = append(out, entry{name: string(rune('a'+i)) + ".txt", body: "x"})
			}
			return out
		}()), http.StatusRequestEntityTooLarge, "more than 8 files"},
		{"empty archive", "nothing.zip", zipBytes(t, nil), http.StatusBadRequest, "no files"},
	}
	for _, c := range cases {
		rec := upload(t, e, c.file, c.body, nil)
		if rec.Code != c.status {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.status, rec.Body.String())
		}
		msg := rec.Body.String()
		if !strings.Contains(msg, c.want) {
			t.Errorf("%s: message %q does not mention %q", c.name, strings.TrimSpace(msg), c.want)
		}
		// The message is rendered to a person; it must not name the host's disk.
		if strings.Contains(msg, e.workRoot) || strings.Contains(msg, "/tmp/") {
			t.Errorf("%s: message leaks a host path: %q", c.name, msg)
		}
	}

	// A body that is not multipart at all, and a multipart body with no file.
	r := httptest.NewRequest(http.MethodPost, "/api/upload", strings.NewReader("not multipart"))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.Handler().ServeHTTP(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-multipart body = %d, want 400", rec.Code)
	}
	if rec := upload(t, e, "", nil, map[string]string{"kind": "zip"}); rec.Code != http.StatusBadRequest {
		t.Errorf("multipart with no file part = %d, want 400", rec.Code)
	}
}

// A rejected upload must not leave its bytes on disk, and a staged one must
// leave exactly one.
func TestUploadStagingIsCleanedUp(t *testing.T) {
	e := newTestEngine(t)
	uploads := filepath.Join(e.workRoot, "uploads")

	upload(t, e, "servers.json", []byte(`{"mcpServers":{}}`), nil)
	if ents, _ := os.ReadDir(uploads); len(ents) != 0 {
		t.Fatalf("a rejected upload left %d directories staged", len(ents))
	}
	res := uploadOK(t, e, "ok.zip", zipBytes(t, []entry{{name: "server.mjs", body: "x"}}), nil)
	ents, _ := os.ReadDir(uploads)
	if len(ents) != 1 || ents[0].Name() != res.UploadID {
		t.Fatalf("staged dirs = %v, want just %s", ents, res.UploadID)
	}
	// Close takes the whole staging root with it.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.workRoot); !os.IsNotExist(err) {
		t.Errorf("Close left the staging root behind: %v", err)
	}
}

// StagePath is the in-process twin of POST /api/upload: same digest, same
// entrypoint inference, same staged id a Detonate can claim.
func TestStagePathHappyPath(t *testing.T) {
	e := newTestEngine(t)
	archive := zipBytes(t, []entry{
		{name: "notes-mcp/server.mjs", body: "console.log('hi')"},
		{name: "notes-mcp/package.json", body: `{"name":"notes-mcp"}`},
	})
	path := filepath.Join(t.TempDir(), "notes-mcp.zip")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := e.StagePath(path, StageOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.UploadID, "up_") {
		t.Errorf("upload_id = %q", res.UploadID)
	}
	if res.SHA256 != sha256Hex(archive) {
		t.Errorf("sha256 = %q, want %q", res.SHA256, sha256Hex(archive))
	}
	if res.Kind != events.KindMCPServer || res.Source != "node server.mjs" {
		t.Errorf("inference = %q %q", res.Kind, res.Source)
	}
	if res.Name != "notes-mcp.zip" {
		t.Errorf("name = %q", res.Name)
	}
	// Overrides match the multipart form fields.
	res2, err := e.StagePath(path, StageOpts{Kind: events.KindSkill, Name: "my skill", Source: "bash run.sh"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Kind != events.KindSkill || res2.Source != "bash run.sh" || res2.Name != "my_skill" {
		t.Errorf("overrides = %+v", res2)
	}
}

func TestStagePathRejections(t *testing.T) {
	e := newTestEngine(t)
	dir := t.TempDir()

	if _, err := e.StagePath(filepath.Join(dir, "missing.zip"), StageOpts{}); err == nil {
		t.Fatal("missing file should fail")
	} else if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("missing: %v", err)
	}

	empty := filepath.Join(dir, "empty.zip")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.StagePath(empty, StageOpts{}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("empty: %v", err)
	}

	notArch := filepath.Join(dir, "servers.json")
	if err := os.WriteFile(notArch, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.StagePath(notArch, StageOpts{}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "zip") {
		t.Errorf("non-archive: %v", err)
	}

	// A rejected StagePath must not leave bytes staged.
	uploads := filepath.Join(e.workRoot, "uploads")
	if ents, _ := os.ReadDir(uploads); len(ents) != 0 {
		t.Fatalf("rejected StagePath left %d dirs", len(ents))
	}
}

// ---- upload -> detonate ----

// The second half of the contract: the id from /api/upload is what
// /api/detonate takes, and resolution unpacks it into a per-detonation
// directory that the report never names.
func TestDetonateResolvesAStagedUpload(t *testing.T) {
	e := newTestEngine(t)
	res := uploadOK(t, e, "notes.zip", zipBytes(t, []entry{
		{name: "notes-mcp/server.mjs", body: "console.log(1)"},
		{name: "notes-mcp/package.json", body: "{}"},
	}), nil)

	art, err := e.resolveArtifact("det_test", DetonateRequest{UploadID: res.UploadID})
	if err != nil {
		t.Fatal(err)
	}
	defer e.releaseWork("det_test")

	dir := art.Env["_dir"]
	if dir == "" {
		t.Fatal("no _dir on the resolved artifact")
	}
	// collapseRoot means the chamber sees server.mjs, not notes-mcp/server.mjs.
	if _, err := os.Stat(filepath.Join(dir, "server.mjs")); err != nil {
		t.Errorf("the wrapper directory was not collapsed: %v", err)
	}
	if art.SHA256 != res.SHA256 {
		t.Errorf("digest changed between upload and detonate: %q vs %q", art.SHA256, res.SHA256)
	}
	if art.Env["_ingest"] != "upload" || art.Env["_install"] != "npm install" {
		t.Errorf("env = %v", art.Env)
	}
	// The same upload can be detonated twice; the staged file is not consumed.
	if _, err := e.resolveArtifact("det_test2", DetonateRequest{UploadID: res.UploadID}); err != nil {
		t.Errorf("second detonation of the same upload: %v", err)
	}
	e.releaseWork("det_test2")

	// An unknown or expired id is a 404 so the UI can say "upload it again"
	// rather than "your request was malformed".
	rec := do(t, e, "POST", "/api/detonate", `{"upload_id":"up_nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown upload = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

// An artifact posted alongside an upload refines it, but can never point the
// engine at a directory of the caller's choosing.
func TestDetonateOverridesCannotSetTheHostDirectory(t *testing.T) {
	e := newTestEngine(t)
	res := uploadOK(t, e, "x.zip", zipBytes(t, []entry{{name: "index.mjs", body: "x"}}), nil)

	art, err := e.resolveArtifact("det_ov", DetonateRequest{
		UploadID: res.UploadID,
		Artifact: &events.Artifact{
			Name: "renamed", Kind: events.KindSkill, Source: "bash go.sh",
			Env: map[string]string{"_dir": "/etc", "_install": "make", "_ingest": "spoof"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer e.releaseWork("det_ov")

	if art.Name != "renamed" || art.Kind != events.KindSkill || art.Source != "bash go.sh" {
		t.Errorf("overrides not applied: %+v", art)
	}
	if art.Env["_install"] != "make" {
		t.Errorf("_install override not applied: %v", art.Env)
	}
	if art.Env["_dir"] == "/etc" {
		t.Fatal("a client-supplied _dir reached the chamber upload path")
	}
	if !strings.HasPrefix(art.Env["_dir"], e.workRoot) {
		t.Errorf("_dir %q is outside the engine's staging root", art.Env["_dir"])
	}
	if art.Env["_ingest"] != "upload" {
		t.Errorf("_ingest was overridden to %q", art.Env["_ingest"])
	}

	rec := do(t, e, "POST", "/api/detonate", `{"upload_id":"`+res.UploadID+`","artifact":{"kind":"rootkit"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad kind override = %d, want 400", rec.Code)
	}
}

// A detonation's ingest directory belongs to that detonation and is removed
// when it ends, however it ends.
func TestWorkDirIsPerDetonationAndReleased(t *testing.T) {
	e := newTestEngine(t)
	a, err := e.workDir("det_a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.workDir("det_b")
	if err != nil {
		t.Fatal(err)
	}
	if a == b || !strings.Contains(a, "det_a") || !strings.Contains(b, "det_b") {
		t.Fatalf("work dirs are not per detonation: %q %q", a, b)
	}
	e.releaseWork("det_a")
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Errorf("releaseWork left det_a's directory behind: %v", err)
	}
	if _, err := os.Stat(b); err != nil {
		t.Errorf("releasing det_a removed det_b's directory: %v", err)
	}
	e.mu.Lock()
	_, stillTracked := e.works["det_a"]
	e.mu.Unlock()
	if stillTracked {
		t.Error("a released detonation is still tracked")
	}
	// Resolution that fails after staging must not leak either: /api/detonate
	// releases the directory on the way out.
	before, _ := os.ReadDir(filepath.Join(e.workRoot, "work"))
	rec := do(t, e, "POST", "/api/detonate", `{"repo":"https://github.com/a/b","ref":"--upload-pack=evil"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("hostile ref = %d, want 400", rec.Code)
	}
	after, _ := os.ReadDir(filepath.Join(e.workRoot, "work"))
	if len(after) > len(before) {
		t.Errorf("a failed resolution leaked a working directory: %v -> %v", before, after)
	}
}

// The git path's errors have to reach the client with their own status, and
// without ever naming the engine's filesystem.
func TestDetonateRepoValidationSurfacesAsHTTP(t *testing.T) {
	e := newTestEngine(t)
	cases := []struct {
		name, body string
		status     int
	}{
		{"ssh remote", `{"repo":"git@github.com:acme/x.git"}`, http.StatusBadRequest},
		{"http remote", `{"repo":"http://github.com/acme/x"}`, http.StatusBadRequest},
		{"loopback", `{"repo":"https://127.0.0.1/acme/x"}`, http.StatusBadRequest},
		{"credentials", `{"repo":"https://u:p@github.com/acme/x"}`, http.StatusBadRequest},
		{"no path", `{"repo":"https://github.com"}`, http.StatusBadRequest},
		{"hostile ref", `{"repo":"https://github.com/a/b","ref":"--upload-pack=x"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := do(t, e, "POST", "/api/detonate", c.body)
		if rec.Code != c.status {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.status, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), e.workRoot) {
			t.Errorf("%s: error leaks a host path: %s", c.name, rec.Body.String())
		}
	}
	// Nothing above may have created a detonation.
	rec := do(t, e, "GET", "/api/detonations", "")
	var reports []*events.DetonationReport
	decode(t, rec.Body.Bytes(), &reports)
	if len(reports) != 0 {
		t.Fatalf("rejected repo requests created %d detonations", len(reports))
	}
}

// A cloned repository resolves to an artifact with a real tree digest and a
// working directory the report does not name.
func TestDetonateResolvesAClonedRepo(t *testing.T) {
	git := gitOrSkip(t)
	origin := makeRepo(t, git, map[string]string{
		"server.mjs":   "console.log('hi')",
		"package.json": `{"name":"cloned-mcp"}`,
	})
	e := newTestEngine(t)
	e.cfg.AllowLocalRepos = true

	art, err := e.resolveArtifact("det_git", DetonateRequest{Repo: "file://" + origin})
	if err != nil {
		t.Fatal(err)
	}
	defer e.releaseWork("det_git")

	if art.Kind != events.KindMCPServer || art.Source != "node server.mjs" {
		t.Errorf("inference over the clone = %q %q", art.Kind, art.Source)
	}
	if len(art.SHA256) != 64 {
		t.Errorf("sha256 = %q, want a real tree digest", art.SHA256)
	}
	if art.Env["_ingest"] != "git" || art.Env["_repo"] == "" {
		t.Errorf("env = %v", art.Env)
	}
	if _, err := os.Stat(filepath.Join(art.Env["_dir"], "server.mjs")); err != nil {
		t.Errorf("the clone is not where _dir says: %v", err)
	}
	// publicEnv is what the report carries, and it drops the host path.
	pub := publicEnv(art.Env)
	if _, ok := pub["_dir"]; ok {
		t.Error("_dir survived into the report view")
	}
	if pub["_ingest"] != "git" {
		t.Errorf("publicEnv dropped more than _dir: %v", pub)
	}
}

// The three pre-existing resolution paths must behave exactly as they did.
func TestExistingResolutionPathsAreUnchanged(t *testing.T) {
	e := newTestEngine(t)

	a, err := e.resolveArtifact("det_x", DetonateRequest{ArtifactID: "art_notes_mcp"})
	if err != nil || a.ID != "art_notes_mcp" || a.Source != "node server.mjs" {
		t.Fatalf("zoo path = %+v, %v", a, err)
	}
	b, err := e.resolveArtifact("det_x", DetonateRequest{
		Artifact: &events.Artifact{Name: "Ad Hoc", Source: "npx -y @acme/x"},
	})
	if err != nil || b.Source != "npx -y @acme/x" || b.ID != "art_ad_hoc" {
		t.Fatalf("inline path = %+v, %v", b, err)
	}
	for _, req := range []DetonateRequest{
		{ArtifactID: "art_nope"},
		{},
		{Artifact: &events.Artifact{Name: "x"}},
	} {
		if _, err := e.resolveArtifact("det_x", req); err == nil {
			t.Errorf("%+v resolved when it should not have", req)
		} else if statusOf(err) != http.StatusBadRequest {
			t.Errorf("%+v: status %d, want 400", req, statusOf(err))
		}
	}
	// And nothing in the ingest work tree was touched by any of that.
	if ents, _ := os.ReadDir(filepath.Join(e.workRoot, "work")); len(ents) != 0 {
		t.Errorf("the non-ingest paths staged %d directories", len(ents))
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
