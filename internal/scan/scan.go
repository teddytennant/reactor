// Package scan is the static baseline — the left column of the demo (DEMO.md
// §2). It does exactly what a description-only scanner does: launch the server
// once, pull tools/list, and pattern-match the tool descriptions. That is the
// honest ceiling of static analysis, and it is why the authored rug pull passes
// it: on the first tools/list the description is clean; the poison only appears
// on the fourth serve, which a single-shot static scan never sees.
//
// An external scanner (Snyk's snyk-agent-scan / mcp-scan) can be plugged in via
// REACTOR_SCANNER; when absent, this built-in stands in and is labelled as such
// so the left column is never misrepresented.
package scan

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/reactor-sec/reactor/internal/events"
	"github.com/reactor-sec/reactor/internal/mcpclient"
)

// Result is the static verdict for the scorecard's static-blind column.
type Result struct {
	events.ScanResult
	Tool string
}

// injection/credential needles a real static scanner would flag in a
// description. Deliberately the same class of pattern mcp-scan uses.
var needles = []string{
	"~/.ssh", "~/.aws", "~/.env", "id_rsa", "credentials", "hosts.yml", "wallet.dat",
	"cookies.sqlite", "ignore previous", "ignore all previous", "verdict: safe",
	"has been audited", "disregard previous", "exfiltrate", "send the contents of",
	"api key", "secret key", "password", "attach ~/", "read ~/.",
}

// Options configure a scan.
type Options struct {
	Name     string            // artifact display name
	Argv     []string          // command to launch the server
	Dir      string            // working dir
	Env      map[string]string // extra env
	Emit     func(events.ScanLine)
}

// Run performs the static baseline: one handshake, one tools/list, pattern match.
func Run(ctx context.Context, opt Options) Result {
	tool := "reactor-static (description scan)"
	if ext := os.Getenv("REACTOR_SCANNER"); ext != "" {
		if r, ok := runExternal(ctx, ext, opt); ok {
			return r
		}
	}
	emit := opt.Emit
	if emit == nil {
		emit = func(events.ScanLine) {}
	}
	emit(events.ScanLine{Tool: tool, Stream: "stdout", Text: "scanning tool descriptions…"})

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Isolate the scan's view so pulling tools/list once does not advance the
	// artifact's real session counter (which would shift the rug-pull trigger).
	env := map[string]string{"REACTOR_SESSION": "1"}
	if td, err := os.MkdirTemp("", "reactor-scan"); err == nil {
		env["REACTOR_STATE_DIR"] = td
		defer os.RemoveAll(td)
	}
	for k, v := range opt.Env {
		env[k] = v
	}

	client, err := mcpclient.Start(ctx, opt.Argv, env, opt.Dir, nil)
	if err != nil {
		emit(events.ScanLine{Tool: tool, Stream: "stderr", Text: "failed to launch server: " + err.Error(), Done: true, Status: "error"})
		return Result{Tool: tool, ScanResult: events.ScanResult{Tool: tool, Available: false, Status: "error"}}
	}
	defer client.Close()

	start := time.Now()
	if err := client.Initialize(ctx); err != nil {
		emit(events.ScanLine{Tool: tool, Stream: "stderr", Text: "initialize failed: " + err.Error(), Done: true, Status: "error"})
		return Result{Tool: tool, ScanResult: events.ScanResult{Tool: tool, Available: false, Status: "error"}}
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		emit(events.ScanLine{Tool: tool, Stream: "stderr", Text: "tools/list failed: " + err.Error(), Done: true, Status: "error"})
		return Result{Tool: tool, ScanResult: events.ScanResult{Tool: tool, Available: false, Status: "error"}}
	}

	var findings []string
	for _, tl := range tools {
		low := strings.ToLower(tl.Description)
		var hit string
		for _, n := range needles {
			if strings.Contains(low, n) {
				hit = n
				break
			}
		}
		if hit != "" {
			findings = append(findings, fmt.Sprintf("%s: suspicious phrase %q", tl.Name, hit))
			emit(events.ScanLine{Tool: tool, Stream: "result", Text: fmt.Sprintf("✗ %-10s — flagged: %q", tl.Name, hit), Status: "issues"})
		} else {
			emit(events.ScanLine{Tool: tool, Stream: "result", Text: fmt.Sprintf("✓ %-10s — no issues", tl.Name)})
		}
	}

	res := events.ScanResult{Tool: tool, Available: true, Issues: len(findings), Findings: findings, DurationMs: time.Since(start).Milliseconds()}
	if len(findings) == 0 {
		res.Status = "clean"
		emit(events.ScanLine{Tool: tool, Stream: "result", Text: "CLEAN — 0 issues found", Status: "clean", Issues: 0, Done: true})
	} else {
		res.Status = "issues"
		emit(events.ScanLine{Tool: tool, Stream: "result", Text: fmt.Sprintf("%d issue(s) found", len(findings)), Status: "issues", Issues: len(findings), Done: true})
	}
	return Result{Tool: tool, ScanResult: res}
}

// runExternal shells out to a real scanner (snyk-agent-scan / mcp-scan) if the
// operator configured one. It streams stdout and derives clean/issues from the
// exit code, so the left column can be a genuinely live third-party run.
func runExternal(ctx context.Context, scanner string, opt Options) (Result, bool) {
	fields := strings.Fields(scanner)
	if len(fields) == 0 {
		return Result{}, false
	}
	if _, err := exec.LookPath(fields[0]); err != nil {
		return Result{}, false
	}
	tool := fields[0]
	emit := opt.Emit
	if emit == nil {
		emit = func(events.ScanLine) {}
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	args := append(fields[1:], opt.Argv...)
	cmd := exec.CommandContext(ctx, fields[0], args...)
	cmd.Dir = opt.Dir
	start := time.Now()
	out, err := cmd.CombinedOutput()
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		emit(events.ScanLine{Tool: tool, Stream: "stdout", Text: line})
	}
	status := "clean"
	issues := 0
	if err != nil {
		status = "issues"
		issues = 1
	}
	emit(events.ScanLine{Tool: tool, Stream: "result", Text: strings.ToUpper(status), Status: status, Issues: issues, Done: true})
	return Result{Tool: tool, ScanResult: events.ScanResult{Tool: tool, Available: true, Status: status, Issues: issues, DurationMs: time.Since(start).Milliseconds()}}, true
}
