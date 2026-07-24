// Upload staging: POST /api/upload takes one archive, parks it on disk, and
// hands back the artifact the UI should then POST to /api/detonate. Nothing is
// unpacked into a chamber here — the upload is validated by a dry run so a
// hostile archive is refused with a status a person can read, and the real
// extraction happens per detonation, into a directory that is thrown away when
// the detonation ends.

package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reactor-sec/reactor/internal/events"
)

// multipartSlack is how much of the request body may be framing rather than
// file: part headers, boundaries, and the optional kind/name/source fields.
const multipartSlack = 1 << 20

// maxFieldBytes caps a non-file form field. These are short strings.
const maxFieldBytes = 4 << 10

// stagedUpload is a file accepted by /api/upload and held until a detonation
// claims it. The host path never leaves the engine: the UI gets an opaque id.
type stagedUpload struct {
	ID       string
	Name     string // client filename, reduced to one safe path element
	Kind     string // artifact kind, inferred or client-supplied
	Archive  string // zip | tar | tar.gz
	SHA256   string // of exactly the bytes received
	Size     int64
	Source   string
	Install  string
	Path     string
	StagedMs int64
}

// UploadResponse is the /api/upload reply. It is deliberately everything the UI
// needs to turn around and POST /api/detonate without asking anything else.
type UploadResponse struct {
	UploadID      string          `json:"upload_id"`
	Name          string          `json:"name"`
	Kind          string          `json:"kind"`
	Archive       string          `json:"archive"`
	SHA256        string          `json:"sha256"`
	SizeBytes     int64           `json:"size_bytes"`
	Files         int             `json:"files"`
	UnpackedBytes int64           `json:"unpacked_bytes"`
	SkippedEntry  int             `json:"skipped_entries"`
	Source        string          `json:"source"`
	Install       string          `json:"install,omitempty"`
	ExpiresMs     int64           `json:"expires_ms"`
	Artifact      events.Artifact `json:"artifact"`
}

// handleUpload accepts one archive as multipart/form-data and stages it.
//
// The body is streamed to disk and hashed as it streams: an upload is untrusted
// attacker bytes and there is no reason for the engine to hold 64 MiB of them
// in memory, and Artifact.SHA256 has to be the digest of exactly what arrived,
// not of whatever the client claimed.
func (e *Engine) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	e.sweepUploads()

	max := e.cfg.MaxUploadBytes
	r.Body = http.MaxBytesReader(w, r.Body, max+multipartSlack)
	mr, err := r.MultipartReader()
	if err != nil {
		httpFail(w, ingestErrf(http.StatusBadRequest,
			"expected a multipart/form-data upload with one file part"))
		return
	}

	id := "up_" + newID()
	dir := filepath.Join(e.workRoot, "uploads", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		httpFail(w, ingestErrf(http.StatusInternalServerError, "the engine could not stage the upload"))
		return
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()

	up := &stagedUpload{ID: id, StagedMs: nowMs()}
	var overrideKind, overrideSource, overrideInstall, overrideName string
	got := false

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			httpFail(w, uploadReadErr(err, max))
			return
		}
		if part.FileName() == "" {
			// Optional text fields. They only ever refine the metadata; nothing
			// here can point the engine at a path.
			val, err := readField(part)
			part.Close()
			if err != nil {
				httpFail(w, uploadReadErr(err, max))
				return
			}
			switch part.FormName() {
			case "kind":
				overrideKind = val
			case "name":
				overrideName = val
			case "source":
				overrideSource = val
			case "install":
				overrideInstall = val
			}
			continue
		}
		if got {
			part.Close()
			httpFail(w, ingestErrf(http.StatusBadRequest, "send one file per upload"))
			return
		}
		got = true
		up.Name = safeBaseName(part.FileName())
		up.Path = filepath.Join(dir, up.Name)
		if err := e.stageFile(part, up, max); err != nil {
			part.Close()
			httpFail(w, err)
			return
		}
		part.Close()
	}
	if !got {
		httpFail(w, ingestErrf(http.StatusBadRequest, "the upload had no file part"))
		return
	}

	// Validate the archive before promising the UI it can be detonated: a dry
	// run applies every zip-slip, symlink and ceiling check without writing a
	// byte, so a hostile archive fails here rather than mid-detonation.
	stats, err := extractArchive(up.Path, up.Archive, "", e.extractLimits())
	if err != nil {
		httpFail(w, err)
		return
	}
	if stats.Files == 0 {
		httpFail(w, ingestErrf(http.StatusBadRequest, "the archive contains no files"))
		return
	}
	up.Kind, up.Source, up.Install = entrypoint(stats.Names)
	if overrideSource != "" {
		up.Source = overrideSource
	}
	if overrideInstall != "" {
		up.Install = overrideInstall
	}
	if overrideName != "" {
		up.Name = safeBaseName(overrideName)
	}
	if overrideKind != "" {
		if !knownKind(overrideKind) {
			httpFail(w, ingestErrf(http.StatusBadRequest,
				"unknown artifact kind %q — use mcp_server, skill or zip", truncate(overrideKind, 32)))
			return
		}
		up.Kind = overrideKind
	}

	e.mu.Lock()
	e.uploads[up.ID] = up
	e.mu.Unlock()
	ok = true

	writeJSON(w, UploadResponse{
		UploadID: up.ID, Name: up.Name, Kind: up.Kind, Archive: up.Archive,
		SHA256: up.SHA256, SizeBytes: up.Size,
		Files: stats.Files, UnpackedBytes: stats.Bytes, SkippedEntry: stats.Skipped,
		Source: up.Source, Install: up.Install,
		ExpiresMs: up.StagedMs + e.cfg.UploadTTL.Milliseconds(),
		Artifact:  up.artifact(),
	})
}

// stageFile streams one part to disk, hashing as it goes and stopping one byte
// past the ceiling, then identifies the archive from the bytes that landed.
func (e *Engine) stageFile(part *multipart.Part, up *stagedUpload, max int64) error {
	// O_RDWR, not O_WRONLY: the type sniff below reads back the bytes that
	// actually landed rather than trusting anything the client said about them.
	f, err := os.OpenFile(up.Path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return ingestErrf(http.StatusInternalServerError, "the engine could not stage the upload")
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(part, max+1))
	if err != nil {
		return uploadReadErr(err, max)
	}
	if n > max {
		return ingestErrf(http.StatusRequestEntityTooLarge,
			"the file is larger than the %s upload limit", humanBytes(max))
	}
	if n == 0 {
		return ingestErrf(http.StatusBadRequest, "the uploaded file is empty")
	}
	up.Size = n
	up.SHA256 = hex.EncodeToString(h.Sum(nil))

	head := make([]byte, 512)
	hn, _ := f.ReadAt(head, 0)
	kind, err := sniffArchive(head[:hn])
	if err != nil {
		return err
	}
	up.Archive = kind
	return nil
}

// uploadReadErr maps a body-read failure onto a status. MaxBytesReader trips
// when the whole request, framing included, outgrows the ceiling.
func uploadReadErr(err error, max int64) error {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return ingestErrf(http.StatusRequestEntityTooLarge,
			"the upload is larger than the %s limit", humanBytes(max))
	}
	return ingestErrf(http.StatusBadRequest, "the upload was cut short or malformed")
}

func readField(part *multipart.Part) (string, error) {
	b, err := io.ReadAll(io.LimitReader(part, maxFieldBytes))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// artifact is the staged upload as the artifact the UI should post back. It
// carries no `_dir`: where the bytes live on the host is the engine's business,
// and the detonation sets it after unpacking into its own working directory.
func (u *stagedUpload) artifact() events.Artifact {
	a := events.Artifact{
		ID:     "art_" + strings.TrimPrefix(u.ID, "up_"),
		Kind:   u.Kind,
		Name:   u.Name,
		Source: u.Source,
		SHA256: u.SHA256,
		Note:   "uploaded " + u.Name,
	}
	if u.Install != "" {
		a.Env = map[string]string{"_install": u.Install}
	}
	return a
}

func knownKind(k string) bool {
	switch k {
	case events.KindMCPServer, events.KindSkill, events.KindZip:
		return true
	}
	return false
}

// sweepUploads drops staged files nobody detonated. An upload that is never
// used is otherwise a 64 MiB leak that lives as long as the process does.
func (e *Engine) sweepUploads() {
	cutoff := nowMs() - e.cfg.UploadTTL.Milliseconds()
	var stale []string
	e.mu.Lock()
	for id, up := range e.uploads {
		if up.StagedMs < cutoff {
			stale = append(stale, id)
			delete(e.uploads, id)
		}
	}
	e.mu.Unlock()
	for _, id := range stale {
		os.RemoveAll(filepath.Join(e.workRoot, "uploads", id))
	}
}

// ---- per-detonation working directories ----

// workDir is the per-detonation ingest directory. Everything unpacked or cloned
// for one detonation lives here and nowhere else, and it is removed when the
// run ends, when it fails, and when the engine shuts down.
func (e *Engine) workDir(detID string) (string, error) {
	base := filepath.Join(e.workRoot, "work", detID)
	src := filepath.Join(base, "src")
	if err := os.MkdirAll(src, 0o700); err != nil {
		return "", ingestErrf(http.StatusInternalServerError,
			"the engine could not create a working directory for the detonation")
	}
	e.mu.Lock()
	e.works[detID] = base
	e.mu.Unlock()
	return src, nil
}

// releaseWork removes a detonation's ingest directory. Called when the run ends
// either way, and when resolution fails before a run ever starts. Detonations
// that ingested nothing never registered one, so this is a no-op for them.
func (e *Engine) releaseWork(detID string) {
	e.mu.Lock()
	dir := e.works[detID]
	delete(e.works, detID)
	e.mu.Unlock()
	if dir == "" || os.Getenv("REACTOR_KEEP") == "1" {
		return
	}
	os.RemoveAll(dir)
}

// Close releases everything ingest put on disk: staged uploads and every
// detonation working directory. `reactor serve` calls it on shutdown so a
// killed demo does not leave archives in the temp dir.
func (e *Engine) Close() error {
	e.mu.Lock()
	root := e.workRoot
	e.uploads = map[string]*stagedUpload{}
	e.works = map[string]string{}
	e.mu.Unlock()
	if root == "" || os.Getenv("REACTOR_KEEP") == "1" {
		return nil
	}
	return os.RemoveAll(root)
}

// extractLimits is the ceiling pair every unpack and every clone is measured
// against.
func (e *Engine) extractLimits() extractLimits {
	return extractLimits{MaxBytes: e.cfg.MaxExtractBytes, MaxFiles: e.cfg.MaxExtractFiles}
}

// sweepStale removes ingest roots left behind by a previous engine that was
// killed before it could Close. Age, not ownership, is the test — a run older
// than the sweep age cannot still be detonating anything.
func sweepStale(base string, age time.Duration) {
	ents, err := os.ReadDir(base)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-age)
	for _, ent := range ents {
		if !ent.IsDir() || !strings.HasPrefix(ent.Name(), ingestPrefix) {
			continue
		}
		info, err := ent.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		os.RemoveAll(filepath.Join(base, ent.Name()))
	}
}
