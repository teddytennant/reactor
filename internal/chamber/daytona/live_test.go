package daytona

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reactor-sec/reactor/internal/chamber"
)

// repoFile resolves a path relative to the module root, so the test can reach
// bin/ from inside the package directory.
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, rel)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// TestLiveSandbox provisions a real Daytona sandbox and exercises the whole
// chamber surface against it: exec (code/stdout/stderr/cwd/env), a binary
// WriteFile/ReadFile round-trip, Start+Wait, Tail, Grep, Destroy.
//
// It bills a real account, so it only runs when opted in:
//
//	set -a && . ./.env && set +a
//	REACTOR_DAYTONA_LIVE=1 go test ./internal/chamber/daytona -run TestLiveSandbox -v
//
// The sandbox is destroyed in t.Cleanup, so a failing assertion still reaps it.
func TestLiveSandbox(t *testing.T) {
	if os.Getenv("REACTOR_DAYTONA_LIVE") != "1" {
		t.Skip("live Daytona test; set REACTOR_DAYTONA_LIVE=1 (and DAYTONA_API_KEY) to run")
	}
	key := firstEnv("DAYTONA_API_KEY", "DAYTONA_KEY")
	if key == "" {
		t.Fatal("DAYTONA_API_KEY is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// NewWithKey, not New: BYOK is the path where the key only exists on the
	// driver, never in host env — the case the proxy auth bug broke hardest.
	d := NewWithKey(key, "")
	if !d.Available() {
		t.Fatalf("BYOK driver reports unavailable: %s", d.Why())
	}

	ch, err := d.Provision(ctx, chamber.Spec{DetonationID: "live-test-" + randomHex(4)})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	c := ch.(*Chamber)
	// Belt and braces: this runs even on a failed assertion or a panic, and
	// tolerates the sandbox already being gone (the destroy subtest deletes it).
	t.Cleanup(func() {
		dctx, dcancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer dcancel()
		if err := ch.Destroy(dctx); err != nil {
			t.Logf("cleanup destroy sandbox %s (may already be gone): %v", c.id, err)
		}
		var s struct {
			State string `json:"state"`
		}
		err := d.do(dctx, "GET", "/sandbox/"+c.id, nil, &s)
		t.Logf("cleanup: sandbox %s state=%q lookup_err=%v", c.id, s.State, err)
	})
	t.Logf("sandbox=%s home=%s proxy=%s info=%+v", c.id, ch.Home(), c.proxyURL, ch.Info())

	t.Run("exec", func(t *testing.T) {
		// The daemon does not create a missing cwd — it fails the whole exec
		// with code -1 — so make the directory the way the engine does.
		if _, err := ch.Exec(ctx, chamber.ExecOpts{Cmd: []string{"mkdir", "-p", path.Join(ch.Home(), "workdir")}}); err != nil {
			t.Fatalf("mkdir workdir: %v", err)
		}
		res, err := ch.Exec(ctx, chamber.ExecOpts{
			Cmd: []string{"sh", "-c", `pwd; echo "var=$REACTOR_LIVE_VAR"; echo on-stderr 1>&2; exit 3`},
			Dir: "workdir",
			Env: map[string]string{"REACTOR_LIVE_VAR": "hello-from-env"},
		})
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
		t.Logf("exit=%d dur=%dms stdout=%q stderr=%q", res.ExitCode, res.DurationMs, res.Stdout, res.Stderr)
		if res.ExitCode != 3 {
			t.Errorf("exit code = %d, want 3", res.ExitCode)
		}
		wantCwd := path.Join(ch.Home(), "workdir")
		if !strings.Contains(res.Stdout, wantCwd) {
			t.Errorf("stdout %q does not contain cwd %q", res.Stdout, wantCwd)
		}
		if !strings.Contains(res.Stdout, "var=hello-from-env") {
			t.Errorf("stdout %q missing env var", res.Stdout)
		}
		if res.Stderr != "on-stderr\n" {
			t.Errorf("stderr = %q, want %q", res.Stderr, "on-stderr\n")
		}
		if strings.Contains(res.Stdout, "on-stderr") {
			t.Errorf("stderr leaked into stdout: %q", res.Stdout)
		}
	})

	t.Run("exec_failure_code", func(t *testing.T) {
		res, err := ch.Exec(ctx, chamber.ExecOpts{Cmd: []string{"sh", "-c", "exit 3"}})
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
		t.Logf("exit=%d stdout=%q stderr=%q", res.ExitCode, res.Stdout, res.Stderr)
		if res.ExitCode != 3 {
			t.Errorf("exit code = %d, want 3", res.ExitCode)
		}
		if res.Stdout != "" || res.Stderr != "" {
			t.Errorf("want empty streams, got stdout=%q stderr=%q", res.Stdout, res.Stderr)
		}
	})

	t.Run("file_roundtrip_binary", func(t *testing.T) {
		// Every byte value plus lone continuation bytes and a NUL run: nothing
		// here survives a UTF-8 round-trip, so this proves the base64 path.
		content := make([]byte, 0, 300)
		for i := range 256 {
			content = append(content, byte(i))
		}
		content = append(content, 0xff, 0xfe, 0x80, 0x00, 0x00, '\n', 0xc3, 0x28)

		p := "payload.bin"
		if err := ch.WriteFile(ctx, p, 0o600, content); err != nil {
			t.Fatalf("write: %v", err)
		}
		got, err := ch.ReadFile(ctx, p)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !bytes.Equal(got, content) {
			t.Fatalf("round-trip mismatch: wrote %d bytes, read %d bytes (equal=%v)", len(content), len(got), bytes.Equal(got, content))
		}
		res, err := ch.Exec(ctx, chamber.ExecOpts{Cmd: []string{"sh", "-c", "stat -c '%s %a' " + shellQuote(path.Join(ch.Home(), p))}})
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		t.Logf("round-trip ok: %d bytes, stat: %s", len(got), strings.TrimSpace(res.Stdout))
		if !strings.Contains(res.Stdout, "600") {
			t.Errorf("mode not applied: %q", res.Stdout)
		}
	})

	t.Run("start_wait", func(t *testing.T) {
		done := path.Join(ch.Home(), "bg.done")
		h, err := ch.Start(ctx, chamber.ExecOpts{
			Cmd: []string{"sh", "-c", "sleep 3; echo finished > " + shellQuote(done)},
		})
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		wctx, wcancel := context.WithTimeout(ctx, 60*time.Second)
		defer wcancel()
		start := time.Now()
		code, err := h.Wait(wctx)
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
		t.Logf("background process exited code=%d after %s", code, time.Since(start).Round(100*time.Millisecond))
		if time.Since(start) < 2*time.Second {
			t.Errorf("Wait returned after %s — it did not actually track the process", time.Since(start))
		}
		out, err := ch.ReadFile(ctx, "bg.done")
		if err != nil {
			t.Fatalf("read marker: %v", err)
		}
		if strings.TrimSpace(string(out)) != "finished" {
			t.Errorf("marker = %q, want %q", out, "finished")
		}
	})

	t.Run("tail", func(t *testing.T) {
		logPath := "stream.jsonl"
		if err := ch.WriteFile(ctx, logPath, 0o644, nil); err != nil {
			t.Fatalf("create log: %v", err)
		}
		tctx, tcancel := context.WithTimeout(ctx, 45*time.Second)
		defer tcancel()
		lines, err := ch.Tail(tctx, logPath)
		if err != nil {
			t.Fatalf("tail: %v", err)
		}
		full := path.Join(ch.Home(), logPath)
		go func() {
			for i := range 3 {
				time.Sleep(700 * time.Millisecond)
				_, _ = ch.Exec(context.Background(), chamber.ExecOpts{
					Cmd: []string{"sh", "-c", fmt.Sprintf("echo '{\"n\":%d}' >> %s", i, shellQuote(full))},
				})
			}
		}()
		var got []string
		deadline := time.After(40 * time.Second)
	collect:
		for len(got) < 3 {
			select {
			case ln, ok := <-lines:
				if !ok {
					break collect
				}
				got = append(got, string(ln))
			case <-deadline:
				break collect
			}
		}
		t.Logf("tail received %d lines: %v", len(got), got)
		if len(got) < 3 {
			t.Fatalf("tail got %d lines, want 3: %v", len(got), got)
		}
		for i, ln := range got[:3] {
			if want := fmt.Sprintf(`{"n":%d}`, i); ln != want {
				t.Errorf("line %d = %q, want %q", i, ln, want)
			}
		}
	})

	t.Run("stage_binary", func(t *testing.T) {
		// The bug this closes: the engine passes HOST absolute paths as argv[0]
		// for a process that runs inside the sandbox, where that path does not
		// exist — every remote session died as exit 127. wire is the honest test
		// case at 4.4 MB: far past what a base64 blob in a shell command can
		// carry (MAX_ARG_STRLEN is 128 KB per argv element).
		hostBin := repoFile(t, "bin/wire")
		info, err := os.Stat(hostBin)
		if err != nil {
			t.Skipf("build the binaries first (make go): %v", err)
		}

		c.mu.Lock()
		before := c.uploads
		c.mu.Unlock()

		staged, err := ch.StageBinary(ctx, hostBin)
		if err != nil {
			t.Fatalf("stage %s: %v", hostBin, err)
		}
		t.Logf("staged %s (%d bytes) -> %s", hostBin, info.Size(), staged)
		if want := path.Join(ch.Home(), ".reactor", "bin", "wire"); staged != want {
			t.Errorf("staged path = %q, want %q", staged, want)
		}

		// It must actually be there, at full size, and executable.
		size, err := c.statSize(ctx, staged)
		if err != nil {
			t.Fatalf("stat staged binary: %v", err)
		}
		if size != info.Size() {
			t.Errorf("staged size = %d, want %d", size, info.Size())
		}

		// And it must RUN. 127 is the failure this whole change exists to fix.
		res, err := ch.Exec(ctx, chamber.ExecOpts{Cmd: []string{staged, "--help"}, Timeout: 60 * time.Second})
		if err != nil {
			t.Fatalf("exec staged binary: %v", err)
		}
		t.Logf("%s --help -> exit=%d stdout=%q stderr=%q", staged, res.ExitCode,
			truncateStr(res.Stdout, 200), truncateStr(res.Stderr, 200))
		if res.ExitCode == 127 {
			t.Fatalf("staged binary not runnable in the sandbox (exit 127): %q %q", res.Stdout, res.Stderr)
		}
		if res.ExitCode != 0 {
			t.Errorf("wire --help exit = %d, want 0", res.ExitCode)
		}
		if !strings.Contains(res.Stdout+res.Stderr, "wire") {
			t.Errorf("output does not look like wire --help: %q / %q", res.Stdout, res.Stderr)
		}

		c.mu.Lock()
		afterFirst := c.uploads
		c.mu.Unlock()
		if afterFirst != before+1 {
			t.Errorf("uploads after first stage = %d, want %d", afterFirst, before+1)
		}

		// Second stage of the same host path: cache hit, no second upload.
		again, err := ch.StageBinary(ctx, hostBin)
		if err != nil {
			t.Fatalf("stage again: %v", err)
		}
		c.mu.Lock()
		afterSecond := c.uploads
		c.mu.Unlock()
		t.Logf("second stage -> %s (uploads %d -> %d)", again, afterFirst, afterSecond)
		if again != staged {
			t.Errorf("second stage returned %q, want %q", again, staged)
		}
		if afterSecond != afterFirst {
			t.Errorf("second stage performed %d extra upload(s); want a cache hit", afterSecond-afterFirst)
		}
	})

	t.Run("write_large_binary_file", func(t *testing.T) {
		// >200 KB of non-UTF8 bytes. The old shell path capped out around 96 KB
		// of content, because base64 of it exceeded MAX_ARG_STRLEN.
		content := make([]byte, 300<<10)
		for i := range content {
			content[i] = byte(i*7 + i/251)
		}
		copy(content[1024:], []byte{0x00, 0x00, 0xff, 0xfe, 0x80, 0xc3, 0x28, '\n'})

		p := "big/payload.bin"
		if err := ch.WriteFile(ctx, p, 0o640, content); err != nil {
			t.Fatalf("write %d bytes: %v", len(content), err)
		}
		got, err := ch.ReadFile(ctx, p)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !bytes.Equal(got, content) {
			t.Fatalf("large round-trip mismatch: wrote %d bytes, read %d", len(content), len(got))
		}
		res, err := ch.Exec(ctx, chamber.ExecOpts{Cmd: []string{"sh", "-c", "stat -c '%s %a' " + shellQuote(path.Join(ch.Home(), p))}})
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		t.Logf("large round-trip ok: %d bytes, stat: %s", len(got), strings.TrimSpace(res.Stdout))
		if !strings.Contains(res.Stdout, "640") {
			t.Errorf("mode not applied to uploaded file: %q", res.Stdout)
		}
	})

	t.Run("tail_burst_no_loss", func(t *testing.T) {
		// A dropped collector line is evidence that silently never happened.
		const burst = 500
		logPath := "burst.jsonl"
		if err := ch.WriteFile(ctx, logPath, 0o644, nil); err != nil {
			t.Fatalf("create log: %v", err)
		}
		tctx, tcancel := context.WithTimeout(ctx, 90*time.Second)
		defer tcancel()
		lines, err := ch.Tail(tctx, logPath)
		if err != nil {
			t.Fatalf("tail: %v", err)
		}
		full := path.Join(ch.Home(), logPath)
		writeStart := time.Now()
		if _, err := ch.Exec(ctx, chamber.ExecOpts{
			Cmd:     []string{"sh", "-c", fmt.Sprintf(`i=1; while [ $i -le %d ]; do echo "{\"n\":$i}" >> %s; i=$((i+1)); done`, burst, shellQuote(full))},
			Timeout: 120 * time.Second,
		}); err != nil {
			t.Fatalf("append burst: %v", err)
		}
		t.Logf("appended %d lines in %s", burst, time.Since(writeStart).Round(time.Millisecond))

		var got []string
		deadline := time.After(80 * time.Second)
	collect:
		for len(got) < burst {
			select {
			case ln, ok := <-lines:
				if !ok {
					break collect
				}
				got = append(got, string(ln))
			case <-deadline:
				break collect
			}
		}
		t.Logf("tail received %d of %d lines", len(got), burst)
		if len(got) != burst {
			t.Fatalf("tail delivered %d lines, want %d — lines were dropped", len(got), burst)
		}
		for i, ln := range got {
			if want := fmt.Sprintf(`{"n":%d}`, i+1); ln != want {
				t.Fatalf("line %d = %q, want %q", i, ln, want)
			}
		}
	})

	t.Run("wait_nonzero_exit", func(t *testing.T) {
		// A sink that crashed and one that finished cleanly must not look alike.
		h, err := ch.Start(ctx, chamber.ExecOpts{Cmd: []string{"sh", "-c", "sleep 2; exit 7"}})
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		wctx, wcancel := context.WithTimeout(ctx, 60*time.Second)
		defer wcancel()
		code, err := h.Wait(wctx)
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
		t.Logf("background process exit code = %d", code)
		if code != 7 {
			t.Fatalf("Wait returned %d, want 7", code)
		}
	})

	t.Run("grep", func(t *testing.T) {
		token := "reactor-canary-" + randomHex(6)
		if err := ch.WriteFile(ctx, "secrets/planted.txt", 0o644, []byte("prefix "+token+" suffix\n")); err != nil {
			t.Fatalf("plant: %v", err)
		}
		hits, err := ch.Grep(ctx, token, "")
		if err != nil {
			t.Fatalf("grep present: %v", err)
		}
		t.Logf("grep(present) -> %v", hits)
		if len(hits) == 0 {
			t.Errorf("grep found no file containing %q", token)
		}
		absent, err := ch.Grep(ctx, "reactor-absent-"+randomHex(6), "")
		if err != nil {
			t.Fatalf("grep absent: %v", err)
		}
		t.Logf("grep(absent) -> %v", absent)
		if len(absent) != 0 {
			t.Errorf("grep found %v for a token never written", absent)
		}
	})

	t.Run("destroy", func(t *testing.T) {
		if err := ch.Destroy(ctx); err != nil {
			t.Fatalf("destroy: %v", err)
		}
		var s struct {
			State string `json:"state"`
		}
		err := d.do(ctx, "GET", "/sandbox/"+c.id, nil, &s)
		t.Logf("post-destroy state=%q err=%v", s.State, err)
		if err == nil && s.State != "" &&
			!strings.Contains(strings.ToLower(s.State), "destroy") {
			t.Errorf("sandbox %s still in state %q after Destroy", c.id, s.State)
		}
	})
}
