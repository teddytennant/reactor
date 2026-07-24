package daytona

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/reactor-sec/reactor/internal/chamber"
	"github.com/reactor-sec/reactor/internal/events"
)

// Chamber is one live Daytona sandbox. Every method maps onto the sandbox
// toolbox API; nothing runs on the host.
type Chamber struct {
	d    *Driver
	id   string
	home string

	mu      sync.Mutex
	started []string // pid files of background processes, killed on destroy
	tailPos map[string]int64
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

// execResult is the toolbox process/execute response.
type execResult struct {
	ExitCode int    `json:"exitCode"`
	Result   string `json:"result"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// exec runs a shell command inside the sandbox.
func (c *Chamber) exec(ctx context.Context, cmd, cwd string, env map[string]string, timeout time.Duration) (*execResult, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	body := map[string]any{"command": cmd, "timeout": int(timeout.Seconds())}
	if cwd != "" {
		body["cwd"] = cwd
	}
	if len(env) > 0 {
		body["env"] = env
	}
	var out execResult
	err := c.d.do(ctx, "POST", "/toolbox/"+c.id+"/toolbox/process/execute", body, &out)
	if err != nil {
		return nil, err
	}
	if out.Stdout == "" && out.Result != "" {
		out.Stdout = out.Result
	}
	return &out, nil
}

// shellQuote makes a value safe inside single quotes for the remote shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// WriteFile implements chamber.Chamber. Content goes over as base64 through the
// shell so a single code path handles text, binaries and odd bait bytes.
func (c *Chamber) WriteFile(ctx context.Context, p string, mode uint32, content []byte) error {
	p = c.resolve(p)
	if mode == 0 {
		mode = 0o644
	}
	b64 := base64.StdEncoding.EncodeToString(content)
	cmd := fmt.Sprintf("mkdir -p %s && printf %%s %s | base64 -d > %s && chmod %o %s",
		shellQuote(path.Dir(p)), shellQuote(b64), shellQuote(p), mode, shellQuote(p))
	res, err := c.exec(ctx, cmd, "", nil, 60*time.Second)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("write %s: exit %d: %s", p, res.ExitCode, res.Stderr)
	}
	return nil
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
// file. Artifacts are small (a server file, a package.json, a README), so this
// is simpler and more predictable than shipping a tarball through the toolbox.
func (c *Chamber) UploadDir(ctx context.Context, hostDir, chamberDir string) error {
	return filepath.Walk(hostDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Size() > 4<<20 {
			return nil
		}
		rel, _ := filepath.Rel(hostDir, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return c.WriteFile(ctx, path.Join(c.resolve(chamberDir), filepath.ToSlash(rel)), uint32(info.Mode().Perm()), data)
	})
}

// Exec implements chamber.Chamber.
func (c *Chamber) Exec(ctx context.Context, opts chamber.ExecOpts) (*chamber.ExecResult, error) {
	cmd := c.buildCmd(opts)
	start := time.Now()
	res, err := c.exec(ctx, cmd, c.dirOf(opts.Dir), opts.Env, opts.Timeout)
	if err != nil {
		return nil, err
	}
	return &chamber.ExecResult{
		ExitCode: res.ExitCode, Stdout: res.Stdout, Stderr: res.Stderr,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// Start implements chamber.Chamber by backgrounding the command with nohup and
// recording its pid so Destroy can reap it.
func (c *Chamber) Start(ctx context.Context, opts chamber.ExecOpts) (chamber.Handle, error) {
	pidFile := fmt.Sprintf("%s/.reactor/proc-%d.pid", c.home, time.Now().UnixNano())
	cmd := fmt.Sprintf("mkdir -p %s/.reactor && nohup %s >/dev/null 2>&1 & echo $! > %s",
		shellQuote(c.home), c.buildCmd(opts), shellQuote(pidFile))
	if _, err := c.exec(ctx, cmd, c.dirOf(opts.Dir), opts.Env, 30*time.Second); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.started = append(c.started, pidFile)
	c.mu.Unlock()
	return &handle{c: c, pidFile: pidFile}, nil
}

// buildCmd renders an ExecOpts into a single shell line, including redirection
// and the optional strace wrapper.
func (c *Chamber) buildCmd(opts chamber.ExecOpts) string {
	argv := opts.Cmd
	if opts.Trace {
		tp := opts.TracePath
		if tp == "" {
			tp = "logs/strace.log"
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

	ch := make(chan []byte, 256)
	go func() {
		defer close(ch)
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
				select {
				case ch <- []byte(ln):
				default:
				}
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
	c       *Chamber
	pidFile string
}

func (h *handle) Wait(ctx context.Context) (int, error) {
	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-time.After(time.Second):
			res, err := h.c.exec(ctx, fmt.Sprintf("kill -0 $(cat %s) 2>/dev/null && echo alive || echo gone", shellQuote(h.pidFile)), "", nil, 15*time.Second)
			if err == nil && strings.Contains(res.Stdout, "gone") {
				return 0, nil
			}
		}
	}
}

func (h *handle) Kill() error {
	_, err := h.c.exec(context.Background(), fmt.Sprintf("kill -9 $(cat %s) 2>/dev/null || true", shellQuote(h.pidFile)), "", nil, 15*time.Second)
	return err
}

var _ = json.Marshal
