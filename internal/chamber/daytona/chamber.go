package daytona

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/reactor-sec/reactor/internal/chamber"
	"github.com/reactor-sec/reactor/internal/events"
)

// Chamber is one live Daytona sandbox. Every method maps onto the sandbox
// toolbox API; nothing runs on the host.
type Chamber struct {
	d          *Driver
	id         string
	home       string
	proxyURL   string // toolbox proxy base; exec + fs live at {proxyURL}/{id}, not on the control API
	proxyToken string // optional REACTOR_DAYTONA_PROXY_TOKEN override; empty means use the driver's API key

	mu      sync.Mutex
	started []string // pid files of background processes, killed on destroy
	tailPos map[string]int64
	// staged memoises StageBinary by host path so a binary the engine stages
	// once per session is only pushed over the wire once per detonation.
	staged map[string]*stagedBin
	// uploads counts multipart uploads actually performed — the live test reads
	// it to prove the staging cache hits instead of re-uploading.
	uploads int
	// strace presence is probed at most once per chamber; see hasStrace.
	straceChecked bool
	straceOK      bool
}

// stagedBin is one in-flight or finished StageBinary. Concurrent callers for the
// same host path wait on done rather than each starting an upload.
type stagedBin struct {
	done chan struct{}
	path string
	err  error
}

// Info implements chamber.Chamber.
func (c *Chamber) Info() events.ChamberInfo {
	gpu := os.Getenv("REACTOR_DAYTONA_GPU")
	if gpu == "" {
		gpu = "cpu"
	}
	return events.ChamberInfo{Driver: "daytona", SandboxID: c.id, GPU: gpu}
}

// Home implements chamber.Chamber.
func (c *Chamber) Home() string { return c.home }

// execResult is the toolbox process/execute response. The daemon answers with
// an exit code and a single combined stream in `result` — stdout and stderr
// interleaved, with no separate fields on the wire (verified live). Stdout and
// Stderr are therefore filled in by this package, not unmarshalled: exec puts
// the combined stream in Stdout, and execSplit recovers the two separately.
type execResult struct {
	ExitCode int    `json:"exitCode"`
	Result   string `json:"result"`
	Stdout   string `json:"-"`
	Stderr   string `json:"-"`
}

// exec runs a shell command inside the sandbox. Stdout carries the combined
// output; callers that need the streams apart use execSplit.
func (c *Chamber) exec(ctx context.Context, cmd, cwd string, env map[string]string, timeout time.Duration) (*execResult, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	body := map[string]any{"command": cmd, "timeout": int(timeout.Seconds())}
	if cwd != "" {
		body["cwd"] = cwd
	}
	if len(env) > 0 {
		// The daemon reads "envs"; a body keyed "env" is accepted and then
		// silently ignored, which loses every variable the engine passes.
		body["envs"] = env
	}
	var out execResult
	err := c.proxyDo(ctx, "POST", "/process/execute", body, &out)
	if err != nil {
		return nil, err
	}
	out.Stdout = out.Result
	return &out, nil
}

// execSplit runs a command and returns stdout and stderr apart. The daemon only
// reports one combined stream, so the command runs in a subshell whose stderr
// is captured to a temp file and replayed after a random marker; splitting on
// the marker recovers both streams byte-exactly. The subshell matters: a
// command that calls exit would otherwise take the whole shell with it and the
// trailer would never run.
func (c *Chamber) execSplit(ctx context.Context, cmd, cwd string, env map[string]string, timeout time.Duration) (*execResult, error) {
	marker := "__reactor_" + randomHex(8) + "__"
	wrapped := fmt.Sprintf(
		`__reactor_err=$(mktemp); ( %s ) 2>"$__reactor_err"; __reactor_rc=$?; printf %%s %s; cat "$__reactor_err" 2>/dev/null; rm -f "$__reactor_err"; exit $__reactor_rc`,
		cmd, shellQuote(marker))
	res, err := c.exec(ctx, wrapped, cwd, env, timeout)
	if err != nil {
		return nil, err
	}
	if i := strings.Index(res.Result, marker); i >= 0 {
		res.Stdout, res.Stderr = res.Result[:i], res.Result[i+len(marker):]
	}
	return res, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// proxyDo calls the sandbox toolbox through the region proxy. The toolbox hangs
// off the proxy base under the sandbox id — {proxyURL}/{sandboxID}{path} — and
// authenticates with the ordinary Daytona API key, the same credential as the
// control plane. REACTOR_DAYTONA_PROXY_TOKEN overrides that key if an account
// ever needs a distinct one; it is not required and is normally unset.
func (c *Chamber) proxyDo(ctx context.Context, method, path string, in, out any) error {
	if c.proxyURL == "" {
		return fmt.Errorf("no toolbox proxy url for sandbox %s", c.id)
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(b))
	}
	url := strings.TrimRight(c.proxyURL, "/") + "/" + c.id + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	token := c.proxyToken
	if token == "" {
		token = c.d.key
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("toolbox proxy rejected the Daytona API key for sandbox %s (POST %s): %s",
			c.id, url, truncateStr(strings.TrimSpace(string(raw)), 200))
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, truncateStr(string(raw), 200))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// shellQuote makes a value safe inside single quotes for the remote shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// shellWriteLimit is the largest payload WriteFile will push through the shell.
// Above it the base64 blob stops fitting in a single argv element: Linux caps
// one element at MAX_ARG_STRLEN = 128 KB no matter how large ARG_MAX is, so a
// ~96 KB file silently became "Argument list too long". 64 KB of content is
// ~88 KB of base64 plus the surrounding command — comfortably under the cap —
// and anything larger streams over the toolbox upload route instead.
const shellWriteLimit = 64 << 10

// WriteFile implements chamber.Chamber. Small payloads go over as base64 through
// the shell in a single round trip, which handles text, binaries and odd bait
// bytes alike; large ones stream through the multipart upload route, which has
// no size ceiling. Both paths are byte-exact.
func (c *Chamber) WriteFile(ctx context.Context, p string, mode uint32, content []byte) error {
	p = c.resolve(p)
	if mode == 0 {
		mode = 0o644
	}
	if len(content) > shellWriteLimit {
		if err := c.uploadStream(ctx, p, bytes.NewReader(content), int64(len(content))); err != nil {
			return err
		}
		return c.chmod(ctx, p, mode)
	}
	b64 := base64.StdEncoding.EncodeToString(content)
	cmd := fmt.Sprintf("mkdir -p %s && printf %%s %s | base64 -d > %s && chmod %o %s",
		shellQuote(path.Dir(p)), shellQuote(b64), shellQuote(p), mode, shellQuote(p))
	res, err := c.exec(ctx, cmd, "", nil, 60*time.Second)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("write %s: exit %d: %s", p, res.ExitCode, truncateStr(strings.TrimSpace(res.Stdout), 200))
	}
	return nil
}

// uploadStream pushes size bytes from src to an absolute sandbox path over the
// toolbox multipart route — POST {toolbox}/files/upload?path=… with the payload
// in a form field named "file" — and verifies the landed size.
//
// The body is an io.MultiReader of (part header, src, closing boundary) rather
// than an io.Pipe, because the length is known: that lets Content-Length be set,
// which avoids chunked transfer encoding. Measured against a live sandbox, the
// same 4.4 MB binary took 41 s with Content-Length and 92 s chunked. Either way
// src is never buffered, so a 10 MB binary does not exist twice in memory and
// never touches a shell command line.
func (c *Chamber) uploadStream(ctx context.Context, dest string, src io.Reader, size int64) error {
	if c.proxyURL == "" {
		return fmt.Errorf("no toolbox proxy url for sandbox %s", c.id)
	}
	// The daemon writes into an existing directory only.
	if err := c.mkdirAll(ctx, path.Dir(dest)); err != nil {
		return err
	}

	// Let mime/multipart render the framing, then hand the file itself through
	// untouched between the two halves.
	var frame bytes.Buffer
	mw := multipart.NewWriter(&frame)
	if _, err := mw.CreateFormFile("file", path.Base(dest)); err != nil {
		return err
	}
	head := append([]byte(nil), frame.Bytes()...)
	frame.Reset()
	if err := mw.Close(); err != nil {
		return err
	}
	tail := append([]byte(nil), frame.Bytes()...)

	url := strings.TrimRight(c.proxyURL, "/") + "/" + c.id + "/files/upload?path=" + neturl.QueryEscape(dest)
	body := io.MultiReader(bytes.NewReader(head), io.LimitReader(src, size), bytes.NewReader(tail))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(head)) + size + int64(len(tail))
	token := c.proxyToken
	if token == "" {
		token = c.d.key
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.d.uploadHTTP().Do(req)
	if err != nil {
		return fmt.Errorf("upload %s: %w", dest, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("upload %s: %s: %s", dest, resp.Status, truncateStr(strings.TrimSpace(string(raw)), 200))
	}
	c.mu.Lock()
	c.uploads++
	c.mu.Unlock()

	// An upload that half-lands is worse than one that fails: the artifact would
	// run truncated and the run would be quietly wrong. Confirm the byte count.
	got, err := c.statSize(ctx, dest)
	if err != nil {
		return fmt.Errorf("verify upload %s: %w", dest, err)
	}
	if got != size {
		return fmt.Errorf("upload %s: landed %d bytes, want %d", dest, got, size)
	}
	return nil
}

func (c *Chamber) mkdirAll(ctx context.Context, dir string) error {
	if dir == "" || dir == "/" || dir == "." {
		return nil
	}
	res, err := c.exec(ctx, "mkdir -p "+shellQuote(dir), "", nil, 30*time.Second)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("mkdir -p %s: exit %d: %s", dir, res.ExitCode, truncateStr(strings.TrimSpace(res.Stdout), 200))
	}
	return nil
}

func (c *Chamber) chmod(ctx context.Context, p string, mode uint32) error {
	res, err := c.exec(ctx, fmt.Sprintf("chmod %o %s", mode, shellQuote(p)), "", nil, 30*time.Second)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("chmod %o %s: exit %d: %s", mode, p, res.ExitCode, truncateStr(strings.TrimSpace(res.Stdout), 200))
	}
	return nil
}

// statSize reports the size in bytes of a file inside the sandbox.
func (c *Chamber) statSize(ctx context.Context, p string) (int64, error) {
	res, err := c.exec(ctx, "wc -c < "+shellQuote(p), "", nil, 30*time.Second)
	if err != nil {
		return 0, err
	}
	if res.ExitCode != 0 {
		return 0, fmt.Errorf("stat %s: exit %d: %s", p, res.ExitCode, truncateStr(strings.TrimSpace(res.Stdout), 200))
	}
	n, err := strconv.ParseInt(strings.TrimSpace(res.Stdout), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("stat %s: unparseable size %q", p, truncateStr(strings.TrimSpace(res.Stdout), 80))
	}
	return n, nil
}

// putFile streams a host file into the sandbox without reading it into memory.
func (c *Chamber) putFile(ctx context.Context, hostPath, dest string, mode uint32) error {
	f, err := os.Open(hostPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if err := c.uploadStream(ctx, dest, f, info.Size()); err != nil {
		return err
	}
	if mode == 0 {
		mode = uint32(info.Mode().Perm())
	}
	return c.chmod(ctx, dest, mode)
}

// StageBinary implements chamber.Chamber. Host absolute paths mean nothing
// inside a remote sandbox — argv[0] of /home/…/bin/victim is simply exit 127 —
// so the executable is uploaded under {home}/.reactor/bin and that path is
// handed back for the engine to run. The engine stages per session; the memo
// keeps that to one upload per detonation, and concurrent callers for the same
// path wait on the first rather than racing a second 10 MB push.
func (c *Chamber) StageBinary(ctx context.Context, hostPath string) (string, error) {
	if strings.TrimSpace(hostPath) == "" {
		return "", fmt.Errorf("stage binary: empty host path")
	}
	info, err := os.Stat(hostPath)
	if err != nil {
		return "", fmt.Errorf("stage binary %s: %w", hostPath, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("stage binary %s: is a directory", hostPath)
	}

	c.mu.Lock()
	if c.staged == nil {
		c.staged = map[string]*stagedBin{}
	}
	if e, ok := c.staged[hostPath]; ok {
		c.mu.Unlock()
		select {
		case <-e.done:
			return e.path, e.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	entry := &stagedBin{done: make(chan struct{})}
	c.staged[hostPath] = entry
	c.mu.Unlock()

	dest := path.Join(c.home, ".reactor", "bin", filepath.Base(hostPath))
	err = c.putFile(ctx, hostPath, dest, 0o755)
	if err != nil {
		// A failed stage is not cached: the next call gets a real retry.
		c.mu.Lock()
		delete(c.staged, hostPath)
		c.mu.Unlock()
		entry.err = fmt.Errorf("stage binary %s: %w", hostPath, err)
	} else {
		entry.path = dest
	}
	close(entry.done)
	return entry.path, entry.err
}

// ReadFile implements chamber.Chamber.
func (c *Chamber) ReadFile(ctx context.Context, p string) ([]byte, error) {
	res, err := c.exec(ctx, "base64 -w0 "+shellQuote(c.resolve(p)), "", nil, 60*time.Second)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("read %s: exit %d", p, res.ExitCode)
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(res.Stdout))
}

// UploadDir implements chamber.Chamber by walking the host dir and writing each
// file. Artifacts are usually small (a server file, a package.json, a README),
// so this is simpler and more predictable than shipping a tarball through the
// toolbox — but "usually" is not "always", and a size cut-off here would mean a
// detonation quietly running an incomplete artifact. Every regular file goes:
// small ones inline over the shell, large ones streamed through the upload
// route, and nothing is skipped.
func (c *Chamber) UploadDir(ctx context.Context, hostDir, chamberDir string) error {
	base := c.resolve(chamberDir)
	return filepath.Walk(hostDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			// Sockets, devices and dangling symlinks have no bytes to copy.
			return nil
		}
		rel, _ := filepath.Rel(hostDir, p)
		dest := path.Join(base, filepath.ToSlash(rel))
		if info.Size() > shellWriteLimit {
			return c.putFile(ctx, p, dest, uint32(info.Mode().Perm()))
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return c.WriteFile(ctx, dest, uint32(info.Mode().Perm()), data)
	})
}

// Exec implements chamber.Chamber.
func (c *Chamber) Exec(ctx context.Context, opts chamber.ExecOpts) (*chamber.ExecResult, error) {
	if err := c.ensureRedirectDirs(ctx, opts); err != nil {
		return nil, err
	}
	cmd := c.buildCmd(ctx, opts)
	start := time.Now()
	res, err := c.execSplit(ctx, cmd, c.dirOf(opts.Dir), opts.Env, opts.Timeout)
	if err != nil {
		return nil, err
	}
	return &chamber.ExecResult{
		ExitCode: res.ExitCode, Stdout: res.Stdout, Stderr: res.Stderr,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// Start implements chamber.Chamber by backgrounding the command under a small
// wrapper script. The wrapper — not a bare `nohup cmd &` — is what makes the
// exit status observable: it records the real pid, waits on it, and writes the
// status to an exit file that Wait reads. Without it a crashed sink and a clean
// one look identical from the host.
func (c *Chamber) Start(ctx context.Context, opts chamber.ExecOpts) (chamber.Handle, error) {
	if err := c.ensureRedirectDirs(ctx, opts); err != nil {
		return nil, err
	}
	stamp := fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomHex(4))
	dir := path.Join(c.home, ".reactor")
	pidFile := path.Join(dir, "proc-"+stamp+".pid")
	wrapPidFile := path.Join(dir, "proc-"+stamp+".wrap.pid")
	exitFile := path.Join(dir, "proc-"+stamp+".exit")
	script := path.Join(dir, "proc-"+stamp+".sh")
	wrapErrFile := path.Join(dir, "proc-"+stamp+".wrap.err")

	// The exit file is renamed into place so Wait can never read a half-written
	// status; the pid file holds the process itself, not the wrapper, so Kill
	// and Destroy signal what actually needs to die.
	body := strings.Join([]string{
		"#!/bin/sh",
		"echo $$ > " + shellQuote(wrapPidFile),
		c.buildCmd(ctx, opts) + " &",
		"__reactor_pid=$!",
		"echo $__reactor_pid > " + shellQuote(pidFile),
		"wait $__reactor_pid",
		"echo $? > " + shellQuote(exitFile) + ".tmp",
		"mv -f " + shellQuote(exitFile) + ".tmp " + shellQuote(exitFile),
		"",
	}, "\n")
	if err := c.WriteFile(ctx, script, 0o755, []byte(body)); err != nil {
		return nil, fmt.Errorf("start: write wrapper: %w", err)
	}
	// `;` not `&&`: with && the shell backgrounds the whole and-list.
	//
	// The wrapper's own stderr goes to a file rather than /dev/null. Everything
	// the shell itself can fail at — an unopenable redirect, a missing argv[0] —
	// is reported there and nowhere else, and discarding it is what let a sink
	// that never started look exactly like one that did.
	cmd := fmt.Sprintf("nohup sh %s >/dev/null 2>%s &", shellQuote(script), shellQuote(wrapErrFile))
	if _, err := c.exec(ctx, cmd, c.dirOf(opts.Dir), opts.Env, 30*time.Second); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.started = append(c.started, pidFile, wrapPidFile)
	c.mu.Unlock()
	return &handle{c: c, pidFile: pidFile, wrapPidFile: wrapPidFile, exitFile: exitFile, wrapErrFile: wrapErrFile}, nil
}

// redirectDirs lists the chamber-side directories opts redirects into.
func (c *Chamber) redirectDirs(opts chamber.ExecOpts) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" {
			return
		}
		d := path.Dir(c.resolve(p))
		if d == "" || d == "." || d == "/" || seen[d] {
			return
		}
		seen[d] = true
		out = append(out, d)
	}
	add(opts.StdoutPath)
	add(opts.StderrPath)
	if opts.Trace {
		tp := opts.TracePath
		if tp == "" {
			tp = defaultTracePath
		}
		add(tp)
	}
	return out
}

// ensureRedirectDirs creates those directories before the command is rendered.
//
// A shell redirect into a directory that does not exist does not fail the way a
// bad command does: the shell refuses to start the process at all, so there is
// no exit status from it, no output, and — since the redirect target is exactly
// the file that would have explained things — no log either. Backgrounded, that
// is completely silent. The engine makes these directories up front, but the
// driver is the last thing between an ExecOpts and a process, and it is the only
// layer that knows a redirect is about to happen, so it checks too.
func (c *Chamber) ensureRedirectDirs(ctx context.Context, opts chamber.ExecOpts) error {
	dirs := c.redirectDirs(opts)
	if len(dirs) == 0 {
		return nil
	}
	quoted := make([]string, 0, len(dirs))
	for _, d := range dirs {
		quoted = append(quoted, shellQuote(d))
	}
	res, err := c.exec(ctx, "mkdir -p "+strings.Join(quoted, " "), "", nil, 30*time.Second)
	if err != nil {
		return fmt.Errorf("mkdir -p %s: %w", strings.Join(dirs, " "), err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("mkdir -p %s: exit %d: %s", strings.Join(dirs, " "),
			res.ExitCode, truncateStr(strings.TrimSpace(res.Stdout), 200))
	}
	return nil
}

// hasStrace probes the sandbox image for strace once and remembers the answer.
// The local driver degrades to an untraced run when strace is missing; without
// this the remote driver would instead prepend a command that does not exist and
// turn every traced session into exit 127.
func (c *Chamber) hasStrace(ctx context.Context) bool {
	c.mu.Lock()
	if c.straceChecked {
		ok := c.straceOK
		c.mu.Unlock()
		return ok
	}
	c.mu.Unlock()
	res, err := c.exec(ctx, "command -v strace >/dev/null 2>&1 && echo yes || echo no", "", nil, 20*time.Second)
	ok := err == nil && res.ExitCode == 0 && strings.Contains(res.Stdout, "yes")
	c.mu.Lock()
	c.straceChecked, c.straceOK = true, ok
	c.mu.Unlock()
	return ok
}

// defaultTracePath is where an ExecOpts that asks to be traced without naming a
// file lands. It must match what redirectDirs pre-creates.
const defaultTracePath = "logs/strace.log"

// buildCmd renders an ExecOpts into a single shell line, including redirection
// and the optional strace wrapper.
func (c *Chamber) buildCmd(ctx context.Context, opts chamber.ExecOpts) string {
	argv := opts.Cmd
	if opts.Trace && c.hasStrace(ctx) {
		tp := opts.TracePath
		if tp == "" {
			tp = defaultTracePath
		}
		argv = append([]string{"strace", "-f", "-e", "trace=file,network,process", "-o", c.resolve(tp), "--"}, argv...)
	}
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		parts = append(parts, shellQuote(a))
	}
	cmd := strings.Join(parts, " ")
	if opts.StdoutPath != "" {
		cmd += " > " + shellQuote(c.resolve(opts.StdoutPath))
	}
	if opts.StderrPath != "" {
		cmd += " 2> " + shellQuote(c.resolve(opts.StderrPath))
	}
	if opts.Stdin != "" {
		cmd = fmt.Sprintf("printf %%s %s | %s", shellQuote(opts.Stdin), cmd)
	}
	return cmd
}

// Tail implements chamber.Chamber by polling the file's tail over exec. The
// sandbox has no push channel for arbitrary files, so we poll from a byte
// offset — which is enough because collectors append whole JSONL lines.
func (c *Chamber) Tail(ctx context.Context, p string) (<-chan []byte, error) {
	full := c.resolve(p)
	c.mu.Lock()
	if c.tailPos == nil {
		c.tailPos = map[string]int64{}
	}
	c.mu.Unlock()

	// The poller and the consumer are decoupled by an unbounded internal queue.
	// A bounded channel with a non-blocking send drops lines whenever a burst
	// outruns the reader, and a dropped line is behavioral evidence that simply
	// never existed as far as the report is concerned — the worst failure this
	// system can have. Queueing costs memory; it never costs evidence.
	ch := make(chan []byte, 256)
	q := newLineQueue()
	go func() {
		defer close(ch)
		for {
			line, ok := q.pop()
			if !ok {
				return
			}
			select {
			case ch <- line:
			case <-ctx.Done():
				// Cancelled: still hand over what was already collected, but do
				// not block on a consumer that has walked away for good.
				select {
				case ch <- line:
				case <-time.After(tailHandoffGrace):
					return
				}
			}
		}
	}()
	go func() {
		defer q.close()
		ticker := time.NewTicker(600 * time.Millisecond)
		defer ticker.Stop()
		var carry string
		drain := func() {
			c.mu.Lock()
			off := c.tailPos[full]
			c.mu.Unlock()
			cmd := fmt.Sprintf("if [ -f %s ]; then tail -c +%d %s; fi", shellQuote(full), off+1, shellQuote(full))
			res, err := c.exec(context.Background(), cmd, "", nil, 30*time.Second)
			if err != nil || res.Stdout == "" {
				return
			}
			c.mu.Lock()
			c.tailPos[full] = off + int64(len(res.Stdout))
			c.mu.Unlock()
			data := carry + res.Stdout
			lines := strings.Split(data, "\n")
			carry = lines[len(lines)-1] // trailing partial line
			for _, ln := range lines[:len(lines)-1] {
				if strings.TrimSpace(ln) == "" {
					continue
				}
				q.push([]byte(ln))
			}
		}
		for {
			select {
			case <-ctx.Done():
				drain() // final flush so the last events are not lost
				return
			case <-ticker.C:
				drain()
			}
		}
	}()
	return ch, nil
}

// tailHandoffGrace bounds how long the tailer keeps offering already-collected
// lines after its context is cancelled, so an abandoned consumer cannot pin the
// goroutine forever.
const tailHandoffGrace = 30 * time.Second

// lineQueue is an unbounded FIFO between the tail poller and the consumer.
type lineQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	items  [][]byte
	closed bool
}

func newLineQueue() *lineQueue {
	q := &lineQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *lineQueue) push(b []byte) {
	q.mu.Lock()
	q.items = append(q.items, b)
	q.mu.Unlock()
	q.cond.Signal()
}

// pop blocks until a line is available; ok is false once the queue is closed and
// drained, which is what closes the caller's channel.
func (q *lineQueue) pop() ([]byte, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.items) == 0 {
		return nil, false
	}
	b := q.items[0]
	q.items = q.items[1:]
	return b, true
}

func (q *lineQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.cond.Broadcast()
}

// Grep implements chamber.Chamber — proves a token is absent from the sandbox
// filesystem (the on-camera check that the context canary was never on disk).
func (c *Chamber) Grep(ctx context.Context, token, root string) ([]string, error) {
	base := c.home
	if root != "" {
		base = c.resolve(root)
	}
	cmd := fmt.Sprintf("grep -rl --binary-files=without-match %s %s 2>/dev/null | grep -v '/logs/' || true",
		shellQuote(token), shellQuote(base))
	res, err := c.exec(ctx, cmd, "", nil, 60*time.Second)
	if err != nil {
		return nil, err
	}
	var hits []string
	for _, ln := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		if ln != "" {
			hits = append(hits, ln)
		}
	}
	return hits, nil
}

// Destroy kills background processes and deletes the sandbox. Always called.
func (c *Chamber) Destroy(ctx context.Context) error {
	c.mu.Lock()
	pids := append([]string(nil), c.started...)
	c.started = nil
	c.mu.Unlock()
	for _, pf := range pids {
		c.exec(ctx, fmt.Sprintf("kill -9 $(cat %s) 2>/dev/null || true", shellQuote(pf)), "", nil, 15*time.Second)
	}
	if os.Getenv("REACTOR_KEEP") == "1" {
		return nil
	}
	// Delete the sandbox outright — the chamber must not outlive the detonation.
	if err := c.d.do(ctx, "DELETE", "/sandbox/"+c.id+"?force=true", nil, nil); err != nil {
		return c.d.do(ctx, "DELETE", "/sandbox/"+c.id, nil, nil)
	}
	return nil
}

func (c *Chamber) resolve(p string) string {
	if p == "" {
		return c.home
	}
	if strings.HasPrefix(p, "/") {
		for _, pre := range []string{"/home/agent", "/home/daytona"} {
			if strings.HasPrefix(p, pre) && pre != c.home {
				return path.Join(c.home, strings.TrimPrefix(p, pre))
			}
		}
		return p
	}
	if strings.HasPrefix(p, "~") {
		return path.Join(c.home, strings.TrimPrefix(p, "~"))
	}
	return path.Join(c.home, p)
}

func (c *Chamber) dirOf(dir string) string {
	if dir == "" {
		return c.home
	}
	return c.resolve(dir)
}

type handle struct {
	c           *Chamber
	pidFile     string
	wrapPidFile string
	exitFile    string
	wrapErrFile string
}

// wrapperErr returns whatever the launching shell complained about, formatted
// for appending to an error. Empty when the shell was happy, which is the usual
// case — this only ever has content when the process never really started.
func (h *handle) wrapperErr() string {
	if h.wrapErrFile == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := h.c.exec(ctx, "cat "+shellQuote(h.wrapErrFile)+" 2>/dev/null || true", "", nil, 15*time.Second)
	if err != nil {
		return ""
	}
	msg := strings.TrimSpace(res.Stdout)
	if msg == "" {
		return ""
	}
	return ": " + truncateStr(msg, 300)
}

// waitUnknownTolerance is how many consecutive polls may find neither an exit
// file nor a live pid before Wait gives up. The window covers the gap between
// the exec that launches the wrapper returning and the wrapper writing its pid.
const waitUnknownTolerance = 15

// Wait blocks until the process exits and returns its real status. The wrapper
// installed by Start records `$?` in an exit file; polling that is what makes a
// sink that crashed on startup distinguishable from one that ran and stopped
// cleanly. Returning 0 unconditionally, as this once did, meant a broken run
// reported as a healthy one.
func (h *handle) Wait(ctx context.Context) (int, error) {
	probe := fmt.Sprintf(
		"if [ -f %s ]; then cat %s; elif [ -s %s ] && kill -0 $(cat %s) 2>/dev/null; then echo __running__; else echo __unknown__; fi",
		shellQuote(h.exitFile), shellQuote(h.exitFile), shellQuote(h.pidFile), shellQuote(h.pidFile))
	unknown := 0
	for {
		res, err := h.c.exec(ctx, probe, "", nil, 15*time.Second)
		if err == nil {
			out := strings.TrimSpace(res.Stdout)
			switch {
			case out == "__running__":
				unknown = 0
			case out == "__unknown__" || out == "":
				unknown++
				if unknown >= waitUnknownTolerance {
					return -1, fmt.Errorf("background process vanished without an exit status (%s)%s",
						h.pidFile, h.wrapperErr())
				}
			default:
				code, perr := strconv.Atoi(out)
				if perr != nil {
					return -1, fmt.Errorf("unreadable exit status %q for %s", truncateStr(out, 80), h.pidFile)
				}
				return code, nil
			}
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// Kill terminates the process. The wrapper is given a moment to notice and
// record the status first — a killed process should still report 137 rather than
// disappear — and is only signalled itself if it fails to.
func (h *handle) Kill() error {
	cmd := fmt.Sprintf(
		"kill -9 $(cat %s) 2>/dev/null || true; i=0; while [ $i -lt 20 ] && [ ! -f %s ]; do sleep 0.1; i=$((i+1)); done; [ -f %s ] || kill -9 $(cat %s) 2>/dev/null || true",
		shellQuote(h.pidFile), shellQuote(h.exitFile), shellQuote(h.exitFile), shellQuote(h.wrapPidFile))
	_, err := h.c.exec(context.Background(), cmd, "", nil, 20*time.Second)
	return err
}

var _ = json.Marshal
