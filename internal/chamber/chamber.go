// Package chamber abstracts the disposable environment the artifact detonates
// in (SPEC §12.1). Two drivers implement it:
//
//	daytona — one GPU sandbox per detonation, SGLang + Qwen3.6-27B-FP8 inside.
//	local   — the same topology as processes in a throwaway directory tree with
//	          HOME rewritten, for developing and rehearsing without a GPU.
//
// The interface is deliberately small and file-shaped: write files, run a
// command, start a background process, tail a JSONL file, destroy everything.
// Both drivers can honour that, and it is all the engine needs, because the
// only thing that crosses back toward the host is structured events.
package chamber

import (
	"context"
	"time"

	"github.com/reactor-sec/reactor/internal/bait"
	"github.com/reactor-sec/reactor/internal/events"
)

// Spec describes the chamber to provision.
type Spec struct {
	DetonationID string
	Artifact     events.Artifact
	Bait         *bait.Set
	Sessions     int
	Network      bool // false = contained egress only (the default and the point)
	Snapshot     string
	Home         string
	Labels       map[string]string
}

// ExecOpts is one command run inside the chamber.
type ExecOpts struct {
	Cmd     []string
	Dir     string
	Env     map[string]string
	Stdin   string
	Timeout time.Duration
	// StdoutPath/StderrPath redirect inside the chamber instead of capturing,
	// which is how long-running processes (sink, sglang, victim) report.
	StdoutPath string
	StderrPath string
	// Trace wraps the command in the syscall collector (SPEC §4.3).
	Trace     bool
	TracePath string
}

// ExecResult is a finished command.
type ExecResult struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMs int64
}

// Handle is a running background process inside the chamber.
type Handle interface {
	// Wait blocks until exit and returns the status code.
	Wait(ctx context.Context) (int, error)
	// Kill terminates the process and its children.
	Kill() error
}

// Chamber is one provisioned, disposable environment.
type Chamber interface {
	// Info is what the UI shows in the header strip.
	Info() events.ChamberInfo
	// Home is the chamber-side HOME the bait layer was planted under.
	Home() string

	WriteFile(ctx context.Context, path string, mode uint32, content []byte) error
	ReadFile(ctx context.Context, path string) ([]byte, error)
	// UploadDir copies a host directory into the chamber.
	UploadDir(ctx context.Context, hostDir, chamberDir string) error
	// StageBinary makes a host-built executable runnable inside the chamber and
	// returns the path to use as argv[0]. Local returns hostPath unchanged;
	// remote drivers upload it once and cache by content.
	StageBinary(ctx context.Context, hostPath string) (string, error)
	// Exec runs to completion.
	Exec(ctx context.Context, opts ExecOpts) (*ExecResult, error)
	// Start launches a background process.
	Start(ctx context.Context, opts ExecOpts) (Handle, error)
	// Tail streams appended lines of a chamber file until ctx is done.
	Tail(ctx context.Context, path string) (<-chan []byte, error)
	// Grep proves a token is absent from the whole filesystem — the on-camera
	// check that the context canary was never on disk (DEMO.md §3).
	Grep(ctx context.Context, token string, root string) ([]string, error)
	// Destroy tears the chamber down. Always called, including on panic.
	Destroy(ctx context.Context) error
}

// Driver provisions chambers.
type Driver interface {
	Name() string
	// Available reports whether this driver can run here (credentials, GPU…).
	Available() bool
	// Why explains an unavailable driver in one line, for the UI.
	Why() string
	Provision(ctx context.Context, spec Spec) (Chamber, error)
}
