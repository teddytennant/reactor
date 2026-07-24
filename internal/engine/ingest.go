// Ingest is how an artifact that is not already in the zoo gets into a chamber:
// an archive uploaded through /api/upload, or a Git repository named in the
// detonate request. Both land as an ordinary host directory that the chamber
// driver copies in, so nothing downstream of resolveArtifact has to know where
// the bytes came from.
//
// Everything in this file runs on the host, over attacker-supplied bytes,
// before any chamber exists — so it is written defensively. Archives are
// identified by content and never by filename; entries that escape their root
// or are not regular files are refused; every ceiling is enforced while copying
// rather than trusted from a header; git is invoked with an explicit argv and
// an environment built from nothing. The host still never *executes* the
// artifact (docs/CONTRACT.md rule 4) — it only unpacks it.

package engine

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/reactor-sec/reactor/internal/events"
)

// ---- errors ----

// ingestError is a failure that carries the HTTP status it should surface as.
// The web UI renders these strings straight to a person, so a message says what
// to do about it and never contains a host path.
type ingestError struct {
	status int
	msg    string
}

func (e *ingestError) Error() string { return e.msg }

func ingestErrf(status int, format string, args ...any) error {
	return &ingestError{status: status, msg: fmt.Sprintf(format, args...)}
}

// statusOf reports the HTTP status an error should surface as. Anything that is
// not an ingestError keeps the pre-existing behaviour of /api/detonate — a bad
// request — so the older resolution paths are unchanged.
func statusOf(err error) int {
	var ie *ingestError
	if errors.As(err, &ie) {
		return ie.status
	}
	return http.StatusBadRequest
}

// httpFail renders a failure the way the rest of the API already does: the
// right status and a plain-text message.
func httpFail(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), statusOf(err))
}

// ---- archive sniffing ----

// Archive kinds, as reported to the UI.
const (
	archiveZip   = "zip"
	archiveTar   = "tar"
	archiveTarGz = "tar.gz"
)

// magic identifies an archive by the bytes it starts with. A filename and a
// client-supplied Content-Type are both attacker-controlled, so neither is
// consulted anywhere in ingest.
type magic struct {
	off   int
	bytes string
	kind  string // "" means recognised but refused; why explains it
	why   string
}

var magics = []magic{
	{0, "PK\x03\x04", archiveZip, ""},
	{0, "PK\x05\x06", archiveZip, ""}, // empty archive
	{0, "PK\x07\x08", archiveZip, ""}, // spanned archive
	{0, "\x1f\x8b", archiveTarGz, ""},
	{257, "ustar", archiveTar, ""},
	{0, "BZh", "", "bzip2"},
	{0, "\xfd7zXZ\x00", "", "xz"},
	{0, "\x28\xb5\x2f\xfd", "", "zstd"},
	{0, "7z\xbc\xaf\x27\x1c", "", "7-zip"},
	{0, "Rar!", "", "rar"},
	{0, "!<arch>", "", "ar/deb"},
}

// sniffArchive identifies an uploaded file from its own leading bytes.
func sniffArchive(head []byte) (string, error) {
	for _, m := range magics {
		if len(head) < m.off+len(m.bytes) {
			continue
		}
		if !bytes.HasPrefix(head[m.off:], []byte(m.bytes)) {
			continue
		}
		if m.kind == "" {
			return "", ingestErrf(http.StatusUnsupportedMediaType,
				"%s archives are not supported — upload a .zip, .tar or .tar.gz", m.why)
		}
		return m.kind, nil
	}
	return "", ingestErrf(http.StatusUnsupportedMediaType,
		"unrecognised file: expected a zip, tar or gzipped tar archive")
}

// ---- extraction ----

// extractLimits bound what an archive is allowed to become on disk. They are
// enforced while copying and never read from the archive's own headers — a zip
// bomb lies about its uncompressed size, and a tar header is just a claim.
type extractLimits struct {
	MaxBytes int64
	MaxFiles int
}

// archiveStats is what one extraction (or dry run) found.
type archiveStats struct {
	Files   int      // regular files written
	Bytes   int64    // uncompressed bytes written
	Skipped int      // symlinks, hardlinks, devices — refused, not materialised
	Names   []string // relative slash paths, for entrypoint inference
}

// maxNamesTracked caps the name list; it exists only to guess an entrypoint, so
// there is no reason to hold a hundred thousand strings for a big repository.
const maxNamesTracked = 4096

// extractArchive unpacks src into dest. A dest of "" is a dry run: every check
// still runs and the stats still come back, but nothing is written. That is how
// /api/upload rejects a hostile archive at upload time, with a status the user
// can understand, instead of half-way through a detonation.
func extractArchive(src, kind, dest string, lim extractLimits) (archiveStats, error) {
	if dest != "" {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return archiveStats{}, ingestErrf(http.StatusInternalServerError, "could not create the unpack directory")
		}
	}
	switch kind {
	case archiveZip:
		return extractZip(src, dest, lim)
	case archiveTar, archiveTarGz:
		return extractTar(src, kind, dest, lim)
	}
	return archiveStats{}, ingestErrf(http.StatusUnsupportedMediaType, "unsupported archive type %q", truncate(kind, 32))
}

func extractZip(src, dest string, lim extractLimits) (archiveStats, error) {
	var st archiveStats
	zr, err := zip.OpenReader(src)
	if err != nil {
		return st, ingestErrf(http.StatusUnsupportedMediaType, "the file is not a readable zip archive")
	}
	defer zr.Close()

	for _, f := range zr.File {
		rel, err := safeEntryPath(f.Name)
		if err != nil {
			return st, ingestErrf(http.StatusBadRequest, "refusing archive: entry %q %s", truncate(f.Name, 96), err)
		}
		info := f.FileInfo()
		switch {
		case info.IsDir():
			if dest != "" {
				if err := os.MkdirAll(filepath.Join(dest, rel), 0o755); err != nil {
					return st, ingestErrf(http.StatusInternalServerError, "could not unpack the archive")
				}
			}
			continue
		case !info.Mode().IsRegular():
			// A symlink entry is precisely how an archive turns a later, entirely
			// innocent-looking regular entry into a write outside dest. Drop it.
			st.Skipped++
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return st, ingestErrf(http.StatusBadRequest, "archive entry %q could not be read", truncate(f.Name, 96))
		}
		err = writeEntry(dest, rel, entryMode(info.Mode()), rc, &st, lim)
		rc.Close()
		if err != nil {
			return st, err
		}
	}
	return st, nil
}

func extractTar(src, kind string, dest string, lim extractLimits) (archiveStats, error) {
	var st archiveStats
	f, err := os.Open(src)
	if err != nil {
		return st, ingestErrf(http.StatusInternalServerError, "the staged upload could not be reopened")
	}
	defer f.Close()

	var r io.Reader = f
	if kind == archiveTarGz {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return st, ingestErrf(http.StatusUnsupportedMediaType, "the file is not a readable gzip archive")
		}
		defer gz.Close()
		r = gz
	}

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if st.Files == 0 {
				return st, ingestErrf(http.StatusUnsupportedMediaType, "the file is not a readable tar archive")
			}
			return st, ingestErrf(http.StatusBadRequest, "the tar archive is truncated or corrupt")
		}
		switch hdr.Typeflag {
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue // metadata; archive/tar has already folded it into hdr
		case tar.TypeDir:
			rel, err := safeEntryPath(hdr.Name)
			if err != nil {
				return st, ingestErrf(http.StatusBadRequest, "refusing archive: entry %q %s", truncate(hdr.Name, 96), err)
			}
			if dest != "" {
				if err := os.MkdirAll(filepath.Join(dest, rel), 0o755); err != nil {
					return st, ingestErrf(http.StatusInternalServerError, "could not unpack the archive")
				}
			}
			continue
		case tar.TypeReg:
			// handled below
		default:
			// Symlinks, hardlinks, devices, fifos. A hardlink is as dangerous as a
			// symlink here (its Linkname is attacker-chosen), and a device node has
			// no business in an artifact at all.
			st.Skipped++
			continue
		}
		rel, err := safeEntryPath(hdr.Name)
		if err != nil {
			return st, ingestErrf(http.StatusBadRequest, "refusing archive: entry %q %s", truncate(hdr.Name, 96), err)
		}
		if err := writeEntry(dest, rel, entryMode(hdr.FileInfo().Mode()), tr, &st, lim); err != nil {
			return st, err
		}
	}
	return st, nil
}

// safeEntryPath cleans an archive entry name into a relative path under the
// destination and refuses anything that escapes it. This is the zip-slip check.
// It rejects rather than silently rewrites: an entry named
// ../../.ssh/authorized_keys is hostile, and the person uploading it should be
// told that is why their archive was turned away.
func safeEntryPath(name string) (string, error) {
	switch {
	case name == "":
		return "", errors.New("has an empty name")
	case len(name) > 1024:
		return "", errors.New("has an absurdly long name")
	case strings.ContainsRune(name, 0):
		return "", errors.New("contains a NUL byte")
	case strings.ContainsRune(name, '\\'):
		// Both the zip and the tar formats separate with '/'. A backslash means
		// nothing here and everything on Windows, so refuse it rather than let
		// the meaning of a path depend on which host unpacked it.
		return "", errors.New("contains a backslash")
	case strings.HasPrefix(name, "/"):
		return "", errors.New("is an absolute path")
	case len(name) > 1 && name[1] == ':':
		return "", errors.New("is a drive-relative path")
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("escapes the archive root")
	}
	return filepath.FromSlash(clean), nil
}

// entryMode reduces an archive's mode to something safe to materialise: no
// setuid, setgid or sticky bit, nothing group- or world-writable, and the
// executable bit only where the entry already had one.
func entryMode(m os.FileMode) os.FileMode {
	if m.Perm()&0o100 != 0 {
		return 0o755
	}
	return 0o644
}

// writeEntry copies one entry, enforcing the file-count and byte ceilings as it
// goes. A dest of "" discards the bytes but still counts them, which is what
// makes the upload-time dry run exact.
func writeEntry(dest, rel string, mode os.FileMode, r io.Reader, st *archiveStats, lim extractLimits) error {
	if st.Files >= lim.MaxFiles {
		return ingestErrf(http.StatusRequestEntityTooLarge,
			"archive contains more than %d files", lim.MaxFiles)
	}
	st.Files++
	if len(st.Names) < maxNamesTracked {
		st.Names = append(st.Names, filepath.ToSlash(rel))
	}

	// Reading one byte past the remaining budget proves the breach without ever
	// writing it. The ceiling is on the whole archive, not per entry, so a bomb
	// made of a million small files is caught by the same arithmetic.
	room := lim.MaxBytes - st.Bytes
	var w io.Writer = io.Discard
	if dest != "" {
		target := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return ingestErrf(http.StatusInternalServerError, "could not unpack the archive")
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return ingestErrf(http.StatusInternalServerError, "could not unpack the archive")
		}
		defer f.Close()
		w = f
	}
	n, err := io.Copy(w, io.LimitReader(r, room+1))
	st.Bytes += n
	if n > room {
		return ingestErrf(http.StatusRequestEntityTooLarge,
			"archive unpacks to more than %s", humanBytes(lim.MaxBytes))
	}
	if err != nil {
		return ingestErrf(http.StatusBadRequest, "archive entry %q is corrupt", truncate(filepath.ToSlash(rel), 96))
	}
	return nil
}

// collapseRoot returns the directory actually holding the artifact's files.
// `git archive`, GitHub's "Download ZIP" and `npm pack` all wrap everything in
// one top-level folder, which would otherwise become artifact/repo-main/ inside
// the chamber and break every relative `source` command.
func collapseRoot(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil || len(ents) != 1 || !ents[0].IsDir() {
		return dir
	}
	return filepath.Join(dir, ents[0].Name())
}

// ---- entrypoint inference ----

// entrypoint guesses how to launch what was just unpacked, from the file names
// alone. It is a convenience for the UI's detonate button, not a contract: the
// client can override kind, source and install on the artifact it posts to
// /api/detonate, and an empty source simply means nothing is launched.
func entrypoint(names []string) (kind, source, install string) {
	root := map[string]bool{}
	for _, n := range stripCommonRoot(names) {
		if !strings.Contains(n, "/") {
			root[n] = true
		}
	}
	for _, c := range []struct{ file, kind, source string }{
		{"server.mjs", events.KindMCPServer, "node server.mjs"},
		{"server.js", events.KindMCPServer, "node server.js"},
		{"server.py", events.KindMCPServer, "python3 server.py"},
		{"index.mjs", events.KindZip, "node index.mjs"},
		{"index.js", events.KindZip, "node index.js"},
		{"main.py", events.KindZip, "python3 main.py"},
	} {
		if root[c.file] {
			kind, source = c.kind, c.source
			break
		}
	}
	if kind == "" {
		// A skill ships instructions, not a command; its shipped text is the
		// payload and detonateNonMCP scans it whether or not anything runs.
		if root["SKILL.md"] {
			kind = events.KindSkill
		} else {
			kind = events.KindZip
		}
	}
	// Installing dependencies is not a side quest — an install hook is one of
	// the signals Reactor exists to catch, and it runs inside the chamber.
	if root["package.json"] {
		install = "npm install"
	}
	return kind, source, install
}

// stripCommonRoot drops the single wrapper directory that repository archives
// carry, so inference sees package.json rather than repo-main/package.json.
func stripCommonRoot(names []string) []string {
	prefix := ""
	for _, n := range names {
		i := strings.IndexByte(n, '/')
		if i < 0 {
			return names // something lives at the top level; there is no wrapper
		}
		if prefix == "" {
			prefix = n[:i+1]
		} else if !strings.HasPrefix(n, prefix) {
			return names
		}
	}
	if prefix == "" {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, strings.TrimPrefix(n, prefix))
	}
	return out
}

// ---- git ----

// normalizeRepoURL validates and canonicalises a repository URL. The accepted
// set is deliberately narrow:
//
//   - https only. ssh (`git@host:path`, `ssh://`) is refused because cloning
//     over it would authenticate as whoever runs the engine, using the host's
//     keys or agent — a detonate request must never borrow the operator's
//     identity — and because the scp-like form is not a URL at all, so none of
//     the checks below would even apply to it.
//   - git:// and file:// are refused: unauthenticated, and (for file) a way to
//     read repositories off the engine host. `ext::` is a transport that runs a
//     command, and cannot reach here since it is not https.
//   - credentials in the URL are refused; they would end up in a report.
//   - IP literals in loopback, private, link-local or unspecified ranges are
//     refused so a detonate request cannot use the engine to reach services on
//     the host's own network. Names that *resolve* into those ranges are not
//     checked — that needs resolve-then-pin at dial time, which git does not
//     expose.
//
// allowLocal (Config.AllowLocalRepos, off by default) relaxes the last two for
// development and for the offline clone tests.
func normalizeRepoURL(raw string, allowLocal bool) (string, error) {
	s := strings.TrimSpace(raw)
	switch {
	case s == "":
		return "", ingestErrf(http.StatusBadRequest, "no repository url given")
	case len(s) > 512:
		return "", ingestErrf(http.StatusBadRequest, "repository url is too long")
	case strings.ContainsAny(s, " \t\r\n\x00"):
		return "", ingestErrf(http.StatusBadRequest, "repository url contains whitespace or control characters")
	case !strings.Contains(s, "://"):
		// git@github.com:owner/repo and bare paths both land here. Name the policy
		// rather than complain about a missing scheme.
		return "", ingestErrf(http.StatusBadRequest,
			"repository url must start with https:// — ssh and scp-style remotes are not accepted")
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", ingestErrf(http.StatusBadRequest, "repository url is not a valid url")
	}
	if allowLocal && u.Scheme == "file" {
		return u.String(), nil
	}
	if u.Scheme != "https" {
		return "", ingestErrf(http.StatusBadRequest,
			"only https:// repositories can be cloned, not %q", truncate(u.Scheme, 16))
	}
	if u.User != nil {
		return "", ingestErrf(http.StatusBadRequest,
			"remove the credentials from the repository url — reactor clones anonymously")
	}
	host := u.Hostname()
	if host == "" {
		return "", ingestErrf(http.StatusBadRequest, "repository url has no host")
	}
	if !allowLocal {
		if ip := net.ParseIP(host); ip != nil {
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
				return "", ingestErrf(http.StatusBadRequest,
					"refusing to clone from a private or loopback address")
			}
		} else if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return "", ingestErrf(http.StatusBadRequest,
				"refusing to clone from a private or loopback address")
		}
	}
	if strings.Trim(u.Path, "/") == "" {
		return "", ingestErrf(http.StatusBadRequest,
			"repository url has no path — expected https://host/owner/repo")
	}
	// A remote url has no use for a query or a fragment, and both are a tidy way
	// to smuggle characters past a reader's eye. Drop them.
	u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = "", "", "", false
	return u.String(), nil
}

// safeGitRef refuses anything that is not plainly a branch, tag or commit. The
// value is passed as `--branch <ref>`, and git would happily read a leading `-`
// as another option.
func safeGitRef(ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	bad := func() error {
		return ingestErrf(http.StatusBadRequest,
			"%q is not a usable branch, tag or commit", truncate(ref, 64))
	}
	if len(ref) > 200 || strings.HasPrefix(ref, "-") || strings.HasPrefix(ref, "/") ||
		strings.HasSuffix(ref, "/") || strings.Contains(ref, "..") || strings.HasSuffix(ref, ".lock") {
		return "", bad()
	}
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '-', r == '_', r == '/':
		default:
			return "", bad()
		}
	}
	return ref, nil
}

// gitCloneArgs builds the clone argv. There is no shell anywhere in this path,
// and everything that could execute is switched off here rather than left to
// whatever the ambient environment happens to say:
//
//	-c core.hooksPath=/dev/null   no hook can run during the clone
//	--template=                   and no template directory is copied in to hold one
//	-c credential.helper=         no host credential helper is consulted
//	-c protocol.allow=never       plus one explicit allow, so a redirect or an
//	-c protocol.https.allow=always  insteadOf rewrite cannot switch transports
//	--depth 1 --single-branch --no-tags   one commit of one branch
//	(no --recurse-submodules)     so a submodule url is never fetched
//	--                            the url can never be read as an option
func gitCloneArgs(repo, ref, dest string, allowLocal bool) []string {
	args := []string{
		"-c", "core.hooksPath=/dev/null",
		"-c", "credential.helper=",
		"-c", "protocol.allow=never",
		"-c", "protocol.https.allow=always",
	}
	if allowLocal {
		args = append(args, "-c", "protocol.file.allow=always")
	}
	args = append(args, "clone", "--depth", "1", "--single-branch", "--no-tags", "--template=", "--quiet")
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	return append(args, "--", repo, dest)
}

// cloneEnv is the whole environment git runs with. It is built from nothing
// rather than filtered from the host's, so no GIT_* variable, no url.insteadOf
// rewrite in the operator's ~/.gitconfig and no credential helper can change
// where these bytes come from or who they are fetched as.
func cloneEnv(home string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, // an empty scratch dir: no ~/.gitconfig
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0", // never block waiting for a username
		"GIT_ASKPASS=true",      // ...nor for a password, through a helper
		"SSH_ASKPASS=true",
		"GIT_LFS_SKIP_SMUDGE=1", // an lfs pointer must not pull further bytes
		"LC_ALL=C",
	}
}

// cloneRepo clones repo into dest under a timeout and a size ceiling. The host
// runs git — not the artifact — so this is the one place ingest shells out, and
// it does so with an explicit argument vector and no shell at all.
func (e *Engine) cloneRepo(ctx context.Context, repo, ref, dest string) error {
	git, err := exec.LookPath("git")
	if err != nil {
		return ingestErrf(http.StatusInternalServerError,
			"git is not installed on the engine host, so repository artifacts cannot be cloned")
	}
	parent := filepath.Dir(dest)
	gitHome := filepath.Join(parent, "githome")
	if err := os.MkdirAll(gitHome, 0o700); err != nil {
		return ingestErrf(http.StatusInternalServerError, "could not create a working directory for the clone")
	}

	ctx, cancel := context.WithTimeout(ctx, e.cfg.CloneTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, git, gitCloneArgs(repo, ref, dest, e.cfg.AllowLocalRepos)...)
	cmd.Env = cloneEnv(gitHome)
	cmd.Dir = parent
	cmd.Stdout = io.Discard
	stderr := &capWriter{n: 8 << 10}
	cmd.Stderr = stderr
	// git clone is a process tree — the transport helper is a child, and it is
	// the one blocked on a socket. Killing only the parent leaves the helper
	// holding the stderr pipe, so a timeout would not actually time anything out
	// until the kernel gave up on the connection. Kill the group, and keep
	// WaitDelay as a backstop for anything that escapes it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 3 * time.Second

	// A shallow clone of a hostile repository can still be enormous. Watch the
	// tree grow and kill the clone the moment it crosses the ceiling, rather
	// than discovering it afterwards when the disk is already gone.
	overrun := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(400 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if dirSize(dest) > e.cfg.MaxCloneBytes {
					close(overrun)
					cancel()
					return
				}
			}
		}
	}()
	runErr := cmd.Run()
	close(stop)

	select {
	case <-overrun:
		os.RemoveAll(dest)
		return ingestErrf(http.StatusRequestEntityTooLarge,
			"repository is larger than %s — clone it yourself and upload an archive instead", humanBytes(e.cfg.MaxCloneBytes))
	default:
	}
	if runErr != nil {
		os.RemoveAll(dest)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ingestErrf(http.StatusGatewayTimeout,
				"cloning the repository timed out after %s", e.cfg.CloneTimeout)
		}
		return ingestErrf(http.StatusBadRequest, "git clone failed: %s", cleanGitErr(stderr.String(), parent))
	}
	// The watchdog only samples; a clone fast enough to finish between two ticks
	// would slip past it, so the finished tree is measured once for real.
	if dirSize(dest) > e.cfg.MaxCloneBytes {
		os.RemoveAll(dest)
		return ingestErrf(http.StatusRequestEntityTooLarge,
			"repository is larger than %s — clone it yourself and upload an archive instead", humanBytes(e.cfg.MaxCloneBytes))
	}
	// The working tree is what detonates. .git carries packfiles we have no use
	// for and a hooks directory we would rather never copy into a chamber.
	os.RemoveAll(filepath.Join(dest, ".git"))
	return nil
}

// cleanGitErr turns git's stderr into one short line with no host path in it.
func cleanGitErr(s string, hide ...string) string {
	for _, h := range hide {
		if h != "" {
			s = strings.ReplaceAll(s, h, "")
		}
	}
	var last string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "fatal: "))
		if line != "" {
			last = line
		}
	}
	switch {
	case last == "":
		return "the repository could not be cloned"
	case strings.Contains(last, "Authentication failed"), strings.Contains(last, "could not read Username"):
		// Forges answer 401 for "private" and for "does not exist" alike, and
		// reactor never authenticates, so both look like this from here.
		return "repository not found, or it is private — reactor clones anonymously"
	}
	return truncate(last, 200)
}

// capWriter keeps at most n bytes and silently drops the rest. Git's stderr is
// partly whatever the remote decided to say, so it does not get to be unbounded.
type capWriter struct {
	b strings.Builder
	n int
}

func (c *capWriter) Write(p []byte) (int, error) {
	if room := c.n - c.b.Len(); room > 0 {
		if len(p) > room {
			p = p[:room]
		}
		c.b.Write(p)
	}
	return len(p), nil
}

func (c *capWriter) String() string { return c.b.String() }

// ---- tree hashing / sealing ----

// sealTree makes a freshly cloned directory safe to hand to a chamber and
// returns its digest. Anything that is not a regular file or a directory is
// deleted — a repository may legitimately contain symlinks, but UploadDir
// follows them, which would copy the *host's* /etc/passwd into a place the
// artifact can read and exfiltrate. The digest is sha256 over each surviving
// file in sorted relative-path order, as `path\x00contents`, so two clones of
// one commit hash identically and any changed byte changes the digest — which
// is what the rest of Reactor means by Artifact.SHA256.
func sealTree(root string, lim extractLimits) (string, archiveStats, error) {
	var st archiveStats
	type entry struct{ rel, abs string }
	var files []entry

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			os.Remove(p)
			st.Skipped++
			return nil
		}
		files = append(files, entry{filepath.ToSlash(rel), p})
		st.Bytes += info.Size()
		return nil
	})
	if err != nil {
		return "", st, ingestErrf(http.StatusInternalServerError, "the cloned repository could not be read")
	}
	st.Files = len(files)
	if st.Files > lim.MaxFiles {
		return "", st, ingestErrf(http.StatusRequestEntityTooLarge,
			"repository contains more than %d files", lim.MaxFiles)
	}
	if st.Bytes > lim.MaxBytes {
		return "", st, ingestErrf(http.StatusRequestEntityTooLarge,
			"repository is larger than %s", humanBytes(lim.MaxBytes))
	}

	// Sorted, so the digest does not depend on directory iteration order
	// (docs/CONTRACT.md ground rule 6).
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	h := sha256.New()
	for _, f := range files {
		if len(st.Names) < maxNamesTracked {
			st.Names = append(st.Names, f.rel)
		}
		fh, err := os.Open(f.abs)
		if err != nil {
			return "", st, ingestErrf(http.StatusInternalServerError, "the cloned repository could not be read")
		}
		io.WriteString(h, f.rel)
		h.Write([]byte{0})
		_, err = io.Copy(h, fh)
		fh.Close()
		if err != nil {
			return "", st, ingestErrf(http.StatusInternalServerError, "the cloned repository could not be read")
		}
	}
	return hex.EncodeToString(h.Sum(nil)), st, nil
}

// ---- small helpers ----

func dirSize(root string) int64 {
	var total int64
	filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// humanBytes renders a ceiling the way a person would say it, for error text.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.4g GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.4g MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.4g KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// safeBaseName reduces a client-supplied filename to one boring path element.
// It is used for display and for the name of the staged file on disk; it is
// never joined with anything else the client controls.
func safeBaseName(name string) string {
	name = filepath.Base(filepath.FromSlash(strings.TrimSpace(name)))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if len(out) > 96 {
		out = out[:96]
	}
	if out == "" {
		out = "artifact"
	}
	return out
}

// repoName is the last path element of a repository url, for the artifact's
// display name: https://github.com/acme/notes-mcp.git -> notes-mcp.
func repoName(repo string) string {
	u, err := url.Parse(repo)
	if err != nil {
		return "repository"
	}
	name := strings.TrimSuffix(path.Base(strings.TrimRight(u.Path, "/")), ".git")
	if name == "" || name == "." || name == "/" {
		return "repository"
	}
	return safeBaseName(name)
}
