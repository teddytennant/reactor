package engine

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reactor-sec/reactor/internal/chamber"
)

// fakeChamber implements just the two methods these tests exercise. Embedding
// the interface means a chamber that grows a method does not break the fake.
type fakeChamber struct {
	chamber.Chamber
	home     string
	staged   []string
	stageErr error
	execs    []chamber.ExecOpts
	exit     int
	execErr  error
}

func (f *fakeChamber) Home() string {
	if f.home == "" {
		return "/home/daytona"
	}
	return f.home
}

func (f *fakeChamber) StageBinary(_ context.Context, hostPath string) (string, error) {
	if f.stageErr != nil {
		return "", f.stageErr
	}
	f.staged = append(f.staged, hostPath)
	return "/sandbox/.reactor/bin/" + filepath.Base(hostPath), nil
}

func (f *fakeChamber) Exec(_ context.Context, opts chamber.ExecOpts) (*chamber.ExecResult, error) {
	f.execs = append(f.execs, opts)
	if f.execErr != nil {
		return nil, f.execErr
	}
	return &chamber.ExecResult{ExitCode: f.exit}, nil
}

// The engine's bins are host paths, which are exit 127 as argv[0] inside a
// remote sandbox. Staging must return chamber paths and must not write back
// into the shared struct — detonations run concurrently against one engine.
func TestStageBinsReturnsChamberPathsWithoutTouchingTheShared(t *testing.T) {
	host := bins{victim: "/host/bin/victim", wire: "/host/bin/wire", sink: "/host/bin/reactor-sink"}
	ch := &fakeChamber{}

	got, err := stageBins(context.Background(), ch, host)
	if err != nil {
		t.Fatal(err)
	}
	want := bins{
		victim: "/sandbox/.reactor/bin/victim",
		wire:   "/sandbox/.reactor/bin/wire",
		sink:   "/sandbox/.reactor/bin/reactor-sink",
	}
	if got != want {
		t.Fatalf("staged bins = %+v, want %+v", got, want)
	}
	if host.victim != "/host/bin/victim" || host.wire != "/host/bin/wire" || host.sink != "/host/bin/reactor-sink" {
		t.Fatalf("the caller's bins were mutated: %+v", host)
	}
	// collect was empty (it is optional) and must not be staged.
	if len(ch.staged) != 3 {
		t.Fatalf("staged %v, want exactly the three non-empty binaries", ch.staged)
	}
}

func TestStageBinsFailsTheDetonationLoudly(t *testing.T) {
	ch := &fakeChamber{stageErr: fmt.Errorf("upload refused")}
	_, err := stageBins(context.Background(), ch, bins{victim: "/host/bin/victim"})
	if err == nil {
		t.Fatal("staging failure was swallowed")
	}
	if !strings.Contains(err.Error(), "victim") || !strings.Contains(err.Error(), "upload refused") {
		t.Fatalf("error %q names neither the binary nor the cause", err)
	}
}

// A freshly provisioned remote sandbox is a bare HOME. The first thing the
// engine does there is redirect a background process into logs/, and a redirect
// whose parent directory does not exist takes the process with it and leaves no
// log to say so — which is exactly how the sink silently never started. The
// directories must be made before anything is launched, through the interface,
// so it holds for every driver.
func TestPrepareChamberDirsMakesEveryDirectoryTheRunWritesInto(t *testing.T) {
	ch := &fakeChamber{home: "/home/daytona"}
	if err := prepareChamberDirs(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if len(ch.execs) != 1 {
		t.Fatalf("ran %d commands, want exactly one mkdir", len(ch.execs))
	}
	cmd := strings.Join(ch.execs[0].Cmd, " ")
	if !strings.Contains(cmd, "mkdir -p") {
		t.Fatalf("command is not a mkdir: %q", cmd)
	}
	for _, want := range []string{
		"/home/daytona/logs",
		"/home/daytona/artifact",
		"/home/daytona/.reactor",
		"/home/daytona/.reactor/bin",
		"/home/daytona/.reactor/state",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("mkdir does not cover %s: %q", want, cmd)
		}
	}
}

// And a chamber that cannot make them fails the detonation rather than letting
// every later redirect fail one at a time in silence.
func TestPrepareChamberDirsFailsLoudly(t *testing.T) {
	if err := prepareChamberDirs(context.Background(), &fakeChamber{exit: 1}); err == nil {
		t.Fatal("a chamber that refused to create the log directories was treated as ready")
	}
	if err := prepareChamberDirs(context.Background(), &fakeChamber{execErr: fmt.Errorf("toolbox down")}); err == nil {
		t.Fatal("an unreachable chamber was treated as ready")
	}
}

// The victim resolves its model and credentials from the environment, and a
// remote chamber inherits none of the host's. The local driver's processes do
// inherit it, which is what hid this: remotely the victim got no key and the
// package's default model id, and every session died on a 404.
func TestVictimBackendEnvCrossesIntoTheChamber(t *testing.T) {
	t.Setenv("FIREWORKS_API_KEY", "fw-live-key")
	t.Setenv("VICTIM_MODEL", "accounts/fireworks/models/gpt-oss-120b")
	t.Setenv("XAI_API_KEY", "")

	got := victimBackendEnv()
	if got["FIREWORKS_API_KEY"] != "fw-live-key" {
		t.Errorf("the victim would reach its endpoint unauthenticated: %v", got)
	}
	if got["VICTIM_MODEL"] != "accounts/fireworks/models/gpt-oss-120b" {
		t.Errorf("the victim would fall back to the package default model: %v", got)
	}
	if _, ok := got["XAI_API_KEY"]; ok {
		t.Errorf("an unset variable was forwarded as empty: %v", got)
	}
}

// The sink listens on the CHAMBER's loopback. Probing the host's answers a
// different question — remotely it can never connect, which is how a dead sink
// used to pass for a live one.
func TestWaitForSinkGatesOnTheChamberNotTheHost(t *testing.T) {
	up := &fakeChamber{exit: 0}
	verified, err := waitForSink(context.Background(), up, 9931)
	if err != nil || !verified {
		t.Fatalf("live sink: verified=%v err=%v", verified, err)
	}
	if len(up.execs) != 1 || up.execs[0].Cmd[0] != "sh" {
		t.Fatalf("readiness was not probed inside the chamber: %+v", up.execs)
	}

	dead := &fakeChamber{exit: 1}
	if _, err := waitForSink(context.Background(), dead, 9931); err == nil {
		t.Fatal("a sink that never listened was reported as up")
	}

	// No probe tool in the image: not proof of failure, and not proof of
	// containment either — the caller is told so instead of being lied to.
	blind := &fakeChamber{exit: probeNoTool}
	verified, err = waitForSink(context.Background(), blind, 9931)
	if err != nil || verified {
		t.Fatalf("unprobeable chamber: verified=%v err=%v", verified, err)
	}
}

// The sink's port must be free where the sink runs. A chamber that already has
// something on the preferred port gets a different one; a chamber that cannot
// be asked falls back to the host heuristic rather than guessing.
func TestPickSinkPortAsksTheChamber(t *testing.T) {
	free := &fakeChamber{exit: 1} // nothing answers: the preferred port is free
	if p := pickSinkPort(context.Background(), free, 9931); p != 9931 {
		t.Fatalf("picked %d, want the preferred 9931", p)
	}
	busy := &fakeChamber{exit: 0} // everything answers: nothing is ever free
	if p := pickSinkPort(context.Background(), busy, 9931); p <= 0 {
		t.Fatalf("picked %d, want a usable fallback port", p)
	}
	if len(busy.execs) < 2 {
		t.Fatalf("gave up after %d probes without trying an alternative", len(busy.execs))
	}
}

// The probe script is the whole mechanism, so run it for real against a real
// listener and a real closed port.
func TestPortProbeScriptSeesAListenerAndItsAbsence(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"))
			c.Close()
		}
	}()
	open := ln.Addr().(*net.TCPAddr).Port

	if code := runScript(t, portProbeScript(open, 3)); code == probeNoTool {
		t.Skip("neither python3, nc, nor curl available to probe with")
	} else if code != 0 {
		t.Fatalf("probe of a listening port exited %d, want 0", code)
	}

	// A port that was listening a moment ago and is not any more.
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := tmp.Addr().(*net.TCPAddr).Port
	tmp.Close()
	if code := runScript(t, portProbeScript(closed, 2)); code != 1 {
		t.Fatalf("probe of a closed port exited %d, want 1", code)
	}
}

func runScript(t *testing.T, script string) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		t.Fatal(err)
	}
	return 0
}
