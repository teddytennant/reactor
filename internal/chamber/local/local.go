// Package local implements the chamber.Driver as a throwaway process tree on
// the host: a fresh HOME under a temp root, bait planted in it, the sink / wire
// / victim / artifact spawned as ordinary processes, everything torn down on
// destroy. It is not as strong an isolation boundary as a Daytona sandbox — it
// is the GPU-free development and rehearsal path, and the DEMO.md §7 backup that
// runs with no cloud at all. The topology and event stream are identical to the
// Daytona driver, so a demo rehearsed here behaves the same there.
package local

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/reactor-sec/reactor/internal/chamber"
	"github.com/reactor-sec/reactor/internal/events"
)

// Driver provisions local chambers.
type Driver struct{}

// New returns a local driver.
func New() *Driver { return &Driver{} }

// Name implements chamber.Driver.
func (*Driver) Name() string { return "local" }

// Available implements chamber.Driver — the local driver always works.
func (*Driver) Available() bool { return true }

// Why implements chamber.Driver.
func (*Driver) Why() string {
	return "process-isolated throwaway tree on the host (no GPU, best-effort egress containment)"
}

// Provision creates the chamber directory tree.
func (*Driver) Provision(ctx context.Context, spec chamber.Spec) (chamber.Chamber, error) {
	base := os.Getenv("REACTOR_LOCAL_ROOT")
	if base == "" {
		base = filepath.Join(os.TempDir(), "reactor")
	}
	root := filepath.Join(base, spec.DetonationID)
	home := spec.Home
	if home == "" {
		home = filepath.Join(root, "home")
	}
	for _, d := range []string{home, filepath.Join(home, "logs"), filepath.Join(home, "artifact"), filepath.Join(home, "work"), filepath.Join(home, ".reactor")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	c := &Chamber{root: root, home: home, sandboxID: "local-" + shortID(spec.DetonationID)}
	return c, nil
}

// Chamber is one local throwaway environment.
type Chamber struct {
	root      string
	home      string
	sandboxID string
	mu        sync.Mutex
	procs     []*exec.Cmd
}

// Info implements chamber.Chamber.
func (c *Chamber) Info() events.ChamberInfo {
	return events.ChamberInfo{Driver: "local", SandboxID: c.sandboxID, GPU: "cpu (host)"}
}

// Home implements chamber.Chamber.
func (c *Chamber) Home() string { return c.home }

// WriteFile implements chamber.Chamber.
func (c *Chamber) WriteFile(_ context.Context, path string, mode uint32, content []byte) error {
	p := c.resolve(path)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	return os.WriteFile(p, content, os.FileMode(mode))
}

// ReadFile implements chamber.Chamber.
func (c *Chamber) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(c.resolve(path))
}

// UploadDir implements chamber.Chamber (host copy).
func (c *Chamber) UploadDir(_ context.Context, hostDir, chamberDir string) error {
	dst := c.resolve(chamberDir)
	return filepath.Walk(hostDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(hostDir, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// StageBinary implements chamber.Chamber. The local chamber shares the host
// filesystem, so a host-built binary is already runnable where it stands and the
// path is returned unchanged — the only work is proving it is really there, so a
// missing build fails here with a readable error instead of as exit 127 inside a
// session.
func (c *Chamber) StageBinary(_ context.Context, hostPath string) (string, error) {
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
	return hostPath, nil
}

// Exec runs a command to completion inside the chamber.
func (c *Chamber) Exec(ctx context.Context, opts chamber.ExecOpts) (*chamber.ExecResult, error) {
	if len(opts.Cmd) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	argv := c.maybeTrace(opts)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = c.dirOf(opts.Dir)
	cmd.Env = c.envOf(opts.Env)
	if opts.Stdin != "" {
		cmd.Stdin = strings.NewReader(opts.Stdin)
	}
	var outBuf, errBuf strings.Builder
	if opts.StdoutPath != "" {
		f, err := c.createUnder(opts.StdoutPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		cmd.Stdout = f
	} else {
		cmd.Stdout = &outBuf
	}
	if opts.StderrPath != "" {
		f, err := c.createUnder(opts.StderrPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		cmd.Stderr = f
	} else {
		cmd.Stderr = &errBuf
	}
	start := time.Now()
	err := cmd.Run()
	res := &chamber.ExecResult{Stdout: outBuf.String(), Stderr: errBuf.String(), DurationMs: time.Since(start).Milliseconds()}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return res, err
		}
	}
	return res, nil
}

// Start launches a background process (the sink).
func (c *Chamber) Start(ctx context.Context, opts chamber.ExecOpts) (chamber.Handle, error) {
	if len(opts.Cmd) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	argv := c.maybeTrace(opts)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = c.dirOf(opts.Dir)
	cmd.Env = c.envOf(opts.Env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Not `f, _ :=`. A redirect target that cannot be opened used to leave a nil
	// *os.File in cmd.Stdout, which os/exec passes down as a bogus descriptor —
	// the process starts, writes nowhere, and the caller is none the wiser. The
	// same class of silence that a missing logs/ dir causes remotely.
	if opts.StdoutPath != "" {
		f, err := c.createUnder(opts.StdoutPath)
		if err != nil {
			return nil, err
		}
		cmd.Stdout = f
	}
	if opts.StderrPath != "" {
		f, err := c.createUnder(opts.StderrPath)
		if err != nil {
			return nil, err
		}
		cmd.Stderr = f
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.procs = append(c.procs, cmd)
	c.mu.Unlock()
	return &handle{cmd: cmd}, nil
}

// Tail streams appended lines of a chamber file until ctx is done.
func (c *Chamber) Tail(ctx context.Context, path string) (<-chan []byte, error) {
	p := c.resolve(path)
	// Ensure the file exists so the tailer can open it immediately.
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND, 0o644); err == nil {
		f.Close()
	}
	ch := make(chan []byte, 256)
	go func() {
		defer close(ch)
		f, err := os.Open(p)
		if err != nil {
			return
		}
		defer f.Close()
		r := bufio.NewReader(f)
		var carry []byte
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			for {
				line, err := r.ReadBytes('\n')
				if len(line) > 0 {
					carry = append(carry, line...)
					if carry[len(carry)-1] == '\n' {
						out := make([]byte, len(carry)-1)
						copy(out, carry[:len(carry)-1])
						carry = carry[:0]
						select {
						case ch <- out:
						case <-ctx.Done():
							return
						}
					}
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					return
				}
			}
			select {
			case <-ctx.Done():
				// Drain any final complete lines before leaving.
				for {
					line, err := r.ReadBytes('\n')
					if len(line) > 1 && line[len(line)-1] == '\n' {
						out := make([]byte, len(line)-1)
						copy(out, line[:len(line)-1])
						select {
						case ch <- out:
						default:
						}
					}
					if err != nil {
						return
					}
				}
			case <-ticker.C:
			}
		}
	}()
	return ch, nil
}

// Grep proves a token is absent from the whole chamber filesystem — the
// on-camera check that the context canary was never on disk (DEMO.md §3).
func (c *Chamber) Grep(_ context.Context, token, root string) ([]string, error) {
	base := c.home
	if root != "" {
		base = c.resolve(root)
	}
	var hits []string
	filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Size() > 8<<20 {
			return nil
		}
		if strings.Contains(p, "/logs/") {
			return nil // our own event logs legitimately mention canaries
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), token) {
			hits = append(hits, strings.TrimPrefix(p, c.home))
		}
		return nil
	})
	return hits, nil
}

// Destroy tears the chamber down. Always called (SPEC §12.6 rule 7).
func (c *Chamber) Destroy(_ context.Context) error {
	c.mu.Lock()
	procs := c.procs
	c.procs = nil
	c.mu.Unlock()
	for _, cmd := range procs {
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			cmd.Process.Kill()
		}
	}
	if os.Getenv("REACTOR_KEEP") == "1" {
		return nil
	}
	return os.RemoveAll(c.root)
}

// ---- internals ----

func (c *Chamber) resolve(path string) string {
	if path == "" {
		return c.home
	}
	if filepath.IsAbs(path) {
		// Absolute paths from bait are under the chamber HOME already if HOME
		// was set to c.home; otherwise map a leading ~ or /home/agent onto it.
		if strings.HasPrefix(path, c.home) {
			return path
		}
		for _, pre := range []string{"/home/agent", "/home/daytona", "~"} {
			if strings.HasPrefix(path, pre) {
				return filepath.Join(c.home, strings.TrimPrefix(path, pre))
			}
		}
		return path
	}
	return filepath.Join(c.home, path)
}

func (c *Chamber) dirOf(dir string) string {
	if dir == "" {
		return c.home
	}
	return c.resolve(dir)
}

func (c *Chamber) envOf(extra map[string]string) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, "HOME="+c.home)
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// createUnder opens a chamber-relative path for writing, making its parent
// directory first so a redirect can never fail merely because nothing has
// written into that directory yet.
func (c *Chamber) createUnder(p string) (*os.File, error) {
	full := c.resolve(p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, err
	}
	return os.Create(full)
}

// maybeTrace wraps the command in strace when requested and available, so file
// and network syscalls become behavioral evidence (SPEC §4.3).
func (c *Chamber) maybeTrace(opts chamber.ExecOpts) []string {
	if !opts.Trace {
		return opts.Cmd
	}
	strace, err := exec.LookPath("strace")
	if err != nil {
		return opts.Cmd
	}
	tp := opts.TracePath
	if tp == "" {
		tp = "logs/strace.log"
	}
	full := c.resolve(tp)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return opts.Cmd // no trace file is possible; run untraced rather than not at all
	}
	pre := []string{strace, "-f", "-e", "trace=file,network,process", "-o", full, "--"}
	return append(pre, opts.Cmd...)
}

type handle struct{ cmd *exec.Cmd }

func (h *handle) Wait(ctx context.Context) (int, error) {
	done := make(chan error, 1)
	go func() { done <- h.cmd.Wait() }()
	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	case err := <-done:
		if h.cmd.ProcessState != nil {
			return h.cmd.ProcessState.ExitCode(), err
		}
		return -1, err
	}
}

func (h *handle) Kill() error {
	if h.cmd.Process == nil {
		return nil
	}
	syscall.Kill(-h.cmd.Process.Pid, syscall.SIGKILL)
	return h.cmd.Process.Kill()
}

func shortID(s string) string {
	s = strings.TrimPrefix(s, "det_")
	if len(s) > 6 {
		return s[:6]
	}
	return s
}
