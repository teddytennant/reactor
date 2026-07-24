package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/reactor-sec/reactor/internal/analyst"
	"github.com/reactor-sec/reactor/internal/bait"
	"github.com/reactor-sec/reactor/internal/chamber"
	"github.com/reactor-sec/reactor/internal/events"
	"github.com/reactor-sec/reactor/internal/oracle"
	"github.com/reactor-sec/reactor/internal/scan"
)

// DetonateRequest is the API request shape. Exactly one of upload_id, repo,
// artifact (with a source) or artifact_id names what to detonate; when an
// upload or a repo is named, `artifact` may still ride along to refine the
// staged artifact's name, kind, source or install step.
//
// Credentials are optional visitor BYOK keys (Daytona + Fireworks). They are
// never written into the report or SSE stream.
type DetonateRequest struct {
	ArtifactID  string           `json:"artifact_id"`
	Artifact    *events.Artifact `json:"artifact"`
	UploadID    string           `json:"upload_id"` // from POST /api/upload
	Repo        string           `json:"repo"`      // https git url to clone
	Ref         string           `json:"ref"`       // branch, tag or commit for repo
	Sessions    int              `json:"sessions"`
	Network     bool             `json:"network"`
	Credentials RunCredentials   `json:"credentials"`
}

// Detonate starts a detonation and returns its id immediately; the run proceeds
// on a background goroutine and streams events on the bus.
//
// Ingest is the exception to "returns immediately": unpacking an upload and
// cloning a repository both happen here, synchronously, so their failures reach
// the caller as an HTTP status instead of as a lifecycle error nobody is
// watching for yet.
func (e *Engine) Detonate(req DetonateRequest) (string, error) {
	id := "det_" + newID()
	art, err := e.resolveArtifact(id, req)
	if err != nil {
		e.releaseWork(id) // ingest may have staged a directory before failing
		return "", err
	}
	sessions := req.Sessions
	if sessions <= 0 {
		sessions = e.cfg.DefaultSessions
	}

	// The report is served to a browser. Where the engine staged the bytes is a
	// host path and has no business in it.
	reportArt := art
	reportArt.Env = publicEnv(art.Env)
	creds := req.Credentials.normalize()
	det := &Detonation{
		idgen:   events.NewIDGen(),
		startMs: nowMs(),
		done:    make(chan struct{}),
		creds:   creds,
		Report: &events.DetonationReport{
			DetonationID: id, ArtifactID: art.ID, Artifact: &reportArt,
			Sessions: sessions, Network: req.Network, StartedMs: nowMs(),
		},
	}
	e.mu.Lock()
	e.reports[id] = det
	e.order = append(e.order, id)
	e.mu.Unlock()

	go e.run(det, art, sessions, req.Network)
	return id, nil
}

// resolveArtifact turns a request into the artifact to detonate. The two
// ingest paths are checked first because they did not exist before and cannot
// collide with anything; the inline-artifact and zoo-id paths below them are
// exactly as they were.
func (e *Engine) resolveArtifact(detID string, req DetonateRequest) (events.Artifact, error) {
	if req.UploadID != "" {
		return e.resolveUpload(detID, req)
	}
	if req.Repo != "" {
		return e.resolveRepo(detID, req)
	}
	if req.Artifact != nil && req.Artifact.Source != "" {
		a := *req.Artifact
		if a.ID == "" {
			a.ID = "art_" + sanitize(a.Name)
		}
		return a, nil
	}
	if req.ArtifactID != "" {
		if a, ok := e.ArtifactByID(req.ArtifactID); ok {
			return a, nil
		}
		return events.Artifact{}, fmt.Errorf("unknown artifact %q", req.ArtifactID)
	}
	return events.Artifact{}, fmt.Errorf("no artifact specified")
}

// resolveUpload unpacks a staged upload into this detonation's own working
// directory. The upload itself is left staged so the same bytes can be
// detonated again without a re-upload; only the unpacked copy is per-run.
func (e *Engine) resolveUpload(detID string, req DetonateRequest) (events.Artifact, error) {
	e.mu.Lock()
	up := e.uploads[req.UploadID]
	e.mu.Unlock()
	if up == nil {
		return events.Artifact{}, ingestErrf(http.StatusNotFound,
			"that upload is unknown or has expired — upload the file again")
	}

	dir, err := e.workDir(detID)
	if err != nil {
		return events.Artifact{}, err
	}
	stats, err := extractArchive(up.Path, up.Archive, dir, e.extractLimits())
	if err != nil {
		return events.Artifact{}, err
	}

	art := up.artifact()
	art.Env = map[string]string{"_dir": collapseRoot(dir), "_ingest": "upload"}
	if up.Install != "" {
		art.Env["_install"] = up.Install
	}
	art.Note = fmt.Sprintf("uploaded %s — %s archive, %s", up.Name, up.Archive, plural(stats.Files, "file"))
	if err := applyOverrides(&art, req.Artifact); err != nil {
		return events.Artifact{}, err
	}
	return art, nil
}

// resolveRepo clones a Git repository into this detonation's own working
// directory and hashes the result, so a repo artifact carries a real digest
// like every other artifact does.
func (e *Engine) resolveRepo(detID string, req DetonateRequest) (events.Artifact, error) {
	repo, err := normalizeRepoURL(req.Repo, e.cfg.AllowLocalRepos)
	if err != nil {
		return events.Artifact{}, err
	}
	ref, err := safeGitRef(req.Ref)
	if err != nil {
		return events.Artifact{}, err
	}
	dir, err := e.workDir(detID)
	if err != nil {
		return events.Artifact{}, err
	}
	// git clone insists on an empty or absent destination.
	dest := filepath.Join(dir, "repo")
	if err := e.cloneRepo(context.Background(), repo, ref, dest); err != nil {
		return events.Artifact{}, err
	}
	sum, stats, err := sealTree(dest, e.extractLimits())
	if err != nil {
		return events.Artifact{}, err
	}

	name := repoName(repo)
	kind, source, install := entrypoint(stats.Names)
	art := events.Artifact{
		ID:     "art_git_" + sanitize(name),
		Kind:   kind,
		Name:   name,
		Source: source,
		SHA256: sum,
		Note:   fmt.Sprintf("cloned %s%s — %s", repo, refSuffix(ref), plural(stats.Files, "file")),
		Env:    map[string]string{"_dir": dest, "_ingest": "git", "_repo": repo},
	}
	if ref != "" {
		art.Env["_ref"] = ref
	}
	if install != "" {
		art.Env["_install"] = install
	}
	if err := applyOverrides(&art, req.Artifact); err != nil {
		return events.Artifact{}, err
	}
	return art, nil
}

// applyOverrides lets the client refine an ingested artifact — a better name,
// the right kind, the command that actually starts it — without ever letting it
// name a host directory. `_dir` is set by ingest and by ingest alone.
func applyOverrides(art *events.Artifact, over *events.Artifact) error {
	if over == nil {
		return nil
	}
	if over.Name != "" {
		art.Name = over.Name
	}
	if over.Kind != "" {
		if !knownKind(over.Kind) {
			return ingestErrf(http.StatusBadRequest,
				"unknown artifact kind %q — use mcp_server, skill or zip", truncate(over.Kind, 32))
		}
		art.Kind = over.Kind
	}
	if over.Source != "" {
		art.Source = over.Source
	}
	if len(over.Args) > 0 {
		art.Args = over.Args
	}
	if over.Note != "" {
		art.Note = over.Note
	}
	if v := over.Env["_install"]; v != "" {
		art.Env["_install"] = v
	}
	return nil
}

// publicEnv copies an artifact's env without the host path ingest staged it at.
func publicEnv(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if k == "_dir" {
			continue
		}
		out[k] = v
	}
	return out
}

// plural renders a count for a human-facing note: "1 file", "12 files".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func refSuffix(ref string) string {
	if ref == "" {
		return ""
	}
	return " @ " + ref
}

// run is the full detonation lifecycle (SPEC §12.6). It always destroys the
// chamber, including on panic.
func (e *Engine) run(det *Detonation, art events.Artifact, sessions int, network bool) {
	defer close(det.done)
	// Whatever ingest staged for this run goes away with the run, on every path
	// out of here including a panic. Detonations that ingested nothing never
	// registered a directory, so this costs them a map lookup.
	defer e.releaseWork(det.Report.DetonationID)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	driver := e.driverFor(det.creds)
	det.emit(e.bus, life(events.PhaseQueued, "queued "+art.Name, nil))
	det.emit(e.bus, life(events.PhaseProvisioning, "provisioning "+driver.Name()+" chamber", nil))

	ch, err := driver.Provision(ctx, chamber.Spec{
		DetonationID: det.Report.DetonationID, Artifact: art, Sessions: sessions, Network: network,
	})
	if err != nil {
		e.fail(det, "provision: "+err.Error())
		return
	}
	det.Report.SandboxID = ch.Info().SandboxID
	det.Report.Driver = ch.Info().Driver
	defer func() {
		det.emit(e.bus, life(events.PhaseDestroying, "destroying chamber", nil))
		ch.Destroy(context.Background())
		det.emit(e.bus, life(events.PhaseDestroyed, "chamber destroyed", nil))
	}()

	// Every collector, log and trace in this run writes under a handful of fixed
	// directories. Make them before anything is launched: a driver whose chamber
	// starts as a bare HOME (a fresh remote sandbox does) otherwise fails the
	// first shell redirect into logs/ — and a redirect that cannot be opened
	// takes the process with it while leaving no log to say so.
	if err := prepareChamberDirs(ctx, ch); err != nil {
		e.fail(det, "prepare chamber: "+err.Error())
		return
	}

	// The chamber components are host-built executables, and the path they were
	// built at is a HOST path — meaningless inside a remote sandbox, where using
	// it as argv[0] is exit 127. Stage them into the chamber and use the paths it
	// hands back for the rest of this detonation. The result is per-detonation on
	// purpose: e.bins is shared by concurrent runs and is never mutated.
	det.emit(e.bus, life(events.PhaseProvisioning,
		"staging chamber binaries ("+binSummary(e.bins)+")", nil))
	bin, err := stageBins(ctx, ch, e.bins)
	if err != nil {
		e.fail(det, "stage chamber binaries: "+err.Error())
		return
	}

	home := ch.Home()
	// Free INSIDE the chamber, which is the only place the answer matters.
	sinkPort := pickSinkPort(ctx, ch, defaultSinkPort)
	b := bait.New(bait.Options{
		Home: home, InstallDir: filepath.Join(home, "artifact"),
		SinkHost: "127.0.0.1", SinkPort: sinkPort,
		Deterministic: e.cfg.Deterministic, Seed: e.cfg.Seed + ":" + det.Report.DetonationID,
	})

	// Victim + chamber info for the header strip. Backend is resolved lazily by
	// the victim binary; we report the intended backend from config/env here,
	// preferring a visitor Fireworks key when present.
	vinfo := e.victimInfo(det.creds)
	det.Report.Victim = vinfo
	info := ch.Info()
	info.Model, info.Served, info.Simulated = vinfo.Model, vinfo.Served, vinfo.Simulated
	info.Revision, info.ToolParser, info.Temp, info.Seed = vinfo.Revision, vinfo.ToolCallParser, vinfo.Temp, vinfo.Seed
	det.emit(e.bus, life(events.PhaseChamberReady, "chamber ready", &info))

	// Plant bait + write the canary table the chamber collectors match against.
	if err := plantBait(ctx, ch, b); err != nil {
		e.fail(det, "plant bait: "+err.Error())
		return
	}
	canaryPath := filepath.Join(home, ".reactor", "canaries.json")
	det.emit(e.bus, life(events.PhaseBaitPlanted, fmt.Sprintf("planted %d bait files + %d canaries; system-prompt canary is not on disk", len(b.Files), len(b.Canaries)), nil))

	// Install the artifact into the chamber install dir.
	installDir := filepath.Join(home, "artifact")
	artArgv, artErr := e.installArtifact(ctx, ch, art, installDir)
	if artErr != nil {
		e.fail(det, "install artifact: "+artErr.Error())
		return
	}
	det.emit(e.bus, life(events.PhaseInstalling, "installed "+art.Name+" into the chamber", nil))

	// An ingested MCP server usually needs its dependencies fetched before it
	// can start; a zoo server ships its own. The step runs in the chamber, under
	// the same containment as everything else — what it does during install is
	// evidence, and catching that is the point (SPEC §4.3). Non-MCP kinds run
	// their install inside detonateNonMCP, where it is already traced.
	if install := art.Env["_install"]; install != "" && art.Kind == events.KindMCPServer {
		if _, err := ch.Exec(ctx, chamber.ExecOpts{
			Cmd: []string{"sh", "-c", install}, Dir: installDir,
			Trace: true, TracePath: "logs/strace.0.log", Timeout: 3 * time.Minute,
			StdoutPath: "logs/install.out", StderrPath: "logs/install.err",
		}); err != nil {
			e.fail(det, "install step: "+err.Error())
			return
		}
		det.emit(e.bus, life(events.PhaseInstalling, "ran install step: "+install, nil))
	}

	// Static baseline (left column) + sink, in the background.
	go e.runScan(ctx, det, art, installDir, artArgv)

	logDir := filepath.Join(home, "logs")
	sinkHandle, err := ch.Start(ctx, chamber.ExecOpts{
		Cmd:        []string{bin.sink, "--http", fmt.Sprintf("127.0.0.1:%d", sinkPort), "--dns", "", "--log-dir", logDir, "--canaries", canaryPath},
		Env:        map[string]string{"REACTOR_LOG_DIR": logDir, "REACTOR_CANARY_FILE": canaryPath},
		StdoutPath: "logs/sink.out", StderrPath: "logs/sink.err",
	})
	if err != nil {
		e.fail(det, "start sink: "+err.Error())
		return
	}
	defer sinkHandle.Kill()
	// A dead sink is not a cosmetic problem: it is the containment boundary, and
	// every session after it would run with nothing catching egress. Gate on it.
	verified, err := waitForSink(ctx, ch, sinkPort)
	if err != nil {
		e.fail(det, "egress sink: "+err.Error()+sinkDiagnostics(ctx, ch, sinkHandle))
		return
	}
	sinkMsg := fmt.Sprintf("egress sink up on 127.0.0.1:%d — nothing egresses past here", sinkPort)
	if !verified {
		sinkMsg = fmt.Sprintf("egress sink started on 127.0.0.1:%d (unverified — the chamber has no probe tool)", sinkPort)
	}
	det.emit(e.bus, life(events.PhaseSinkUp, sinkMsg, nil))

	// Tail the chamber's collector logs and republish onto the bus.
	tctx, tcancel := context.WithCancel(ctx)
	e.tailLogs(tctx, det, ch, []string{"logs/wire.jsonl", "logs/transcript.jsonl", "logs/sink.jsonl", "logs/behavioral.jsonl"})

	// Run the sessions. A rug pull only exists across repetition, so we always
	// run N and let the oracle diff descriptions (SPEC §4.5).
	sinkURL := fmt.Sprintf("http://127.0.0.1:%d", sinkPort)
	for s := 1; s <= sessions; s++ {
		det.emit(e.bus, sessionLife(events.PhaseSessionStart, fmt.Sprintf("session %d/%d", s, sessions), s))
		if err := e.runSession(ctx, ch, det, art, b, bin, s, installDir, artArgv, canaryPath, logDir, sinkURL, network); err != nil {
			det.emit(e.bus, sessionLife(events.PhaseError, fmt.Sprintf("session %d: %s", s, err.Error()), s))
		}
		det.emit(e.bus, sessionLife(events.PhaseSessionEnd, fmt.Sprintf("session %d complete", s), s))
	}

	// Let the tailers drain the final flushed lines, then stop them.
	time.Sleep(500 * time.Millisecond)
	tcancel()
	time.Sleep(150 * time.Millisecond)

	// Deterministic oracles → analyst verdict.
	e.analyze(ctx, det, b)

	det.Report.EndedMs = nowMs()
}

// chamberDirs are the directories a detonation writes into, relative to the
// chamber HOME. logs/ holds collector JSONL, process stdout/stderr and strace
// output; .reactor/ holds the canary and bait tables, the staged binaries and
// the wire's state; artifact/ is where the artifact is installed.
var chamberDirs = []string{
	"logs",
	"artifact",
	".reactor",
	".reactor/bin",
	".reactor/state",
}

// prepareChamberDirs creates them inside the chamber, whichever driver it is.
// The local driver's Provision already makes most of them because it owns a
// host directory tree; a remote sandbox is provisioned with nothing but a home,
// so the first `cmd > logs/sink.out` there is a shell error and a process that
// never ran. Doing it here, through the interface, keeps both drivers honest
// without either one having to know what the engine intends to write.
func prepareChamberDirs(ctx context.Context, ch chamber.Chamber) error {
	home := ch.Home()
	quoted := make([]string, 0, len(chamberDirs))
	for _, d := range chamberDirs {
		quoted = append(quoted, shQuote(filepath.Join(home, d)))
	}
	res, err := ch.Exec(ctx, chamber.ExecOpts{
		Cmd:     []string{"sh", "-c", "mkdir -p " + strings.Join(quoted, " ")},
		Timeout: 60 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("creating %s: %w", strings.Join(chamberDirs, ", "), err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("creating %s: exit %d: %s", strings.Join(chamberDirs, ", "),
			res.ExitCode, strings.TrimSpace(res.Stderr+res.Stdout))
	}
	return nil
}

// tailLogs streams the chamber's collector JSONL files and republishes each
// line onto the bus with a stamped evidence id. This is the only path by which
// chamber events reach the host (SPEC §12.1: only structured events cross).
func (e *Engine) tailLogs(ctx context.Context, det *Detonation, ch chamber.Chamber, paths []string) {
	for _, p := range paths {
		lines, err := ch.Tail(ctx, p)
		if err != nil {
			continue
		}
		go func(lines <-chan []byte) {
			for line := range lines {
				var ev events.Event
				if err := json.Unmarshal(line, &ev); err != nil || ev.Kind == "" {
					continue
				}
				det.emit(e.bus, ev)
			}
		}(lines)
	}
}

// runSession drives one victim session through wire→artifact.
func (e *Engine) runSession(ctx context.Context, ch chamber.Chamber, det *Detonation, art events.Artifact, b *bait.Set, bin bins, session int, installDir string, artArgv []string, canaryPath, logDir, sinkURL string, network bool) error {
	// victim -- wire --canaries --dir installDir -- <artifact argv>
	// bin holds the CHAMBER-side paths staged for this detonation, never the
	// host paths the binaries were built at.
	victimArgv := []string{
		bin.victim, "--session", strconv.Itoa(session), "--log-dir", logDir, "--task", e.cfg.Task, "--",
		bin.wire, "--session", strconv.Itoa(session), "--log-dir", logDir, "--canaries", canaryPath, "--dir", installDir, "--",
	}
	victimArgv = append(victimArgv, artArgv...)

	env := map[string]string{
		"REACTOR_SESSION":             strconv.Itoa(session),
		"REACTOR_DETONATION":          det.Report.DetonationID,
		"REACTOR_LOG_DIR":             logDir,
		"REACTOR_CANARY_FILE":         canaryPath,
		"REACTOR_CANARY_CONTEXT":      b.Context.Token,
		"REACTOR_CANARY_CONVERSATION": b.Conv.Token,
		"REACTOR_TASK":                e.cfg.Task,
		"REACTOR_SINK_HTTP":           sinkURL,
		"REACTOR_STATE_DIR":           filepath.Join(ch.Home(), ".reactor", "state"),
		"REACTOR_VICTIM_SEED":         "7",
	}
	if e.cfg.VictimBackend != "" {
		env["REACTOR_VICTIM_BACKEND"] = e.cfg.VictimBackend
	}
	// The victim resolves its own backend from the environment, and a remote
	// chamber has none: nothing of the host's is inherited there. Forward the
	// backend's configuration explicitly so the victim reaches the same model
	// wherever it runs. Without it a `fireworks` victim inside a sandbox
	// resolved with no key and the package default model id, and every session
	// died on "Model not found" — while the local driver, whose processes
	// inherit the operator's env for free, looked perfectly healthy.
	for k, v := range victimBackendEnv() {
		env[k] = v
	}
	// Visitor BYOK: hand the Fireworks key to the victim binary only. It is
	// stripped from every artifact process by internal/procenv; never written
	// into reports or the SSE stream.
	if det.creds.FireworksAPIKey != "" {
		env["FIREWORKS_API_KEY"] = det.creds.FireworksAPIKey
		if env["REACTOR_VICTIM_BACKEND"] == "" {
			env["REACTOR_VICTIM_BACKEND"] = "fireworks"
		}
	}
	// Egress containment applies to the ARTIFACT only, never the victim. The
	// victim is host-trusted infrastructure that may need to reach a hosted
	// model endpoint; routing its calls through the sink would both break it and
	// pollute the egress log. So we hand the proxy target to wire, which applies
	// it to the artifact it spawns (see cmd/wire), and keep the victim direct.
	if !network {
		env["REACTOR_ARTIFACT_PROXY"] = sinkURL
	}

	// A skill or zip has no MCP surface for the victim to drive; it detonates by
	// running its install/entrypoint under the syscall collector, and its
	// shipped text is scanned for analyzer-targeted injection.
	if art.Kind != events.KindMCPServer {
		return e.detonateNonMCP(ctx, ch, det, art, bin, session, installDir, env)
	}

	res, err := ch.Exec(ctx, chamber.ExecOpts{
		Cmd: victimArgv, Env: env, Dir: installDir, Timeout: 3 * time.Minute,
		StderrPath: fmt.Sprintf("logs/victim.%d.err", session),
	})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("victim exited %d", res.ExitCode)
	}
	return nil
}

// victimBackendVars are the environment variables internal/victim reads to
// decide which model the sacrificial agent talks to and how to authenticate.
// They are host operator configuration, not artifact input, and they are
// stripped again by internal/procenv before the artifact is ever spawned — the
// victim is the only process in the chamber that sees them.
var victimBackendVars = []string{
	"FIREWORKS_API_KEY", "FIREWORKS_KEY", "FIREWORKS_BASE_URL", "FIREWORKS_MODEL",
	"XAI_API_KEY", "XAI_OAUTH_TOKEN", "XAI_ACCESS_TOKEN", "XAI_BASE_URL",
	"VICTIM_API_KEY", "VICTIM_MODEL", "VICTIM_BASE_URL",
	"REACTOR_VICTIM_MODEL", "REACTOR_VICTIM_BASE", "SGLANG_BASE_URL",
}

// victimBackendEnv collects whichever of them the host has set.
func victimBackendEnv() map[string]string {
	out := map[string]string{}
	for _, k := range victimBackendVars {
		if v := os.Getenv(k); v != "" {
			out[k] = v
		}
	}
	return out
}

// detonateNonMCP runs a skill/zip artifact's install and entrypoint under
// strace, then parses the trace with reactor-collect into behavioral evidence
// (install hooks, credential sweeps), and scans its shipped text for analyst
// injection. Only session 1 does the work; later sessions are no-ops so the
// rug-pull/repetition machinery does not apply to a one-shot payload.
func (e *Engine) detonateNonMCP(ctx context.Context, ch chamber.Chamber, det *Detonation, art events.Artifact, bin bins, session int, installDir string, env map[string]string) error {
	if session > 1 {
		return nil
	}
	e.scanArtifactText(ctx, ch, det, art, installDir, session)

	steps := []string{}
	if install := art.Env["_install"]; install != "" {
		steps = append(steps, install)
	}
	if art.Source != "" {
		steps = append(steps, art.Source)
	}
	tracePath := fmt.Sprintf("logs/strace.%d.log", session)
	for i, step := range steps {
		_, err := ch.Exec(ctx, chamber.ExecOpts{
			Cmd: []string{"sh", "-c", step}, Env: env, Dir: installDir,
			Trace: true, TracePath: tracePath, Timeout: 90 * time.Second,
			StdoutPath: fmt.Sprintf("logs/artifact.%d.%d.out", session, i),
			StderrPath: fmt.Sprintf("logs/artifact.%d.%d.err", session, i),
		})
		if err != nil {
			return err
		}
	}

	// Parse the trace into typed behavioral events (SPEC §4.3).
	if bin.collect != "" {
		baitJSON := filepath.Join(ch.Home(), ".reactor", "bait.json")
		_, _ = ch.Exec(ctx, chamber.ExecOpts{
			Cmd: []string{bin.collect,
				"--strace", filepath.Join(ch.Home(), tracePath),
				"--bait", baitJSON,
				"--install-dir", installDir,
				"--home", ch.Home(),
				"--out", filepath.Join(ch.Home(), "logs", "behavioral.jsonl"),
				"--session", strconv.Itoa(session),
			},
			Timeout: 30 * time.Second,
		})
	}
	return nil
}

// scanArtifactText surfaces analyzer-targeted injection in a skill/zip's shipped
// text as behavioral evidence. The oracle matches on the preview and cites the
// event id; the raw text is stripped before the analyst ever sees it.
func (e *Engine) scanArtifactText(ctx context.Context, ch chamber.Chamber, det *Detonation, art events.Artifact, installDir string, session int) {
	src := art.Env["_dir"]
	if src == "" {
		return
	}
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Size() > 256<<10 {
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".md", ".txt", ".mjs", ".js", ".ts", ".sh", ".json", ".yml", ".yaml", ".py":
		default:
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		det.emit(e.bus, events.Event{Kind: events.KindBehavioral, Session: session, TSms: nowMs(),
			Behavioral: &events.BehavioralEvent{
				Op: events.OpFileRead, Source: "artifact-content", InInstall: true,
				Path: filepath.Join(installDir, rel), Preview: truncate(string(data), 8192),
			}})
		return nil
	})
}

// analyze runs the oracles and the analyst, emitting signals then the verdict.
func (e *Engine) analyze(ctx context.Context, det *Detonation, b *bait.Set) {
	det.emit(e.bus, life(events.PhaseAnalyzing, "analyzing typed evidence", nil))

	evs := det.collected()
	kindOf := func(tok string) string {
		if c, ok := b.Lookup(tok); ok {
			return c.Kind
		}
		return ""
	}
	in := oracle.Input{
		Events: evs, Sessions: det.Report.Sessions,
		InstallDir:   filepath.Join(det.homeGuess(), "artifact"),
		DecoyServers: []string{"acme-vault", "acme-deploy"},
		DecoyTools:   []string{"read_secret", "deploy", "vault"},
		CanaryKind:   kindOf,
		SinkHosts:    []string{"127.0.0.1", "localhost"},
	}
	signals := oracle.Evaluate(in)
	det.Report.Signals = signals
	for _, s := range signals {
		det.emit(e.bus, events.Event{Kind: events.KindSignal, Session: s.Session, TSms: s.FirstSeenMs, Signal: &s})
	}
	det.Report.Bait = baitReport(evs, b)

	// Analyst: redacted evidence only.
	redacted := events.ForAnalystSlice(det.collected())
	steps := func(st events.AnalystStep) {
		det.emit(e.bus, events.Event{Kind: events.KindAnalyst, Analyst: &st})
	}
	an := e.newAnalyst(steps, det.creds)
	in2 := analyst.Input{
		ArtifactID: det.Report.ArtifactID, Signals: signals, Evidence: redacted,
		Sessions: det.Report.Sessions, StartedMs: det.Report.StartedMs, EndedMs: nowMs(),
	}
	verdict, err := an.Analyze(ctx, in2)
	if err != nil {
		verdict = analyst.Classify(in2)
		verdict.Fallback = true
	}
	det.Report.Verdict = &verdict
	det.emit(e.bus, events.Event{Kind: events.KindVerdict, TSms: nowMs() - det.startMs, Verdict: &verdict})
	det.emit(e.bus, life(events.PhaseVerdict, verdict.Label+" · "+strings.ToUpper(verdict.Family), nil))
}

func (e *Engine) fail(det *Detonation, msg string) {
	det.Report.Error = msg
	det.Report.EndedMs = nowMs()
	det.emit(e.bus, life(events.PhaseError, msg, nil))
}

// ---- bait planting / artifact install ----

func plantBait(ctx context.Context, ch chamber.Chamber, b *bait.Set) error {
	for _, f := range b.Files {
		if err := ch.WriteFile(ctx, f.Path, f.Mode, []byte(f.Body)); err != nil {
			return err
		}
	}
	// Canary table for the chamber collectors (canary.Load shape).
	type tok struct {
		Token string `json:"token"`
		Kind  string `json:"kind"`
		Label string `json:"label"`
	}
	var toks []tok
	for _, c := range b.Canaries {
		toks = append(toks, tok{c.Token, c.Kind, c.Label})
	}
	data, _ := json.Marshal(toks)
	if err := ch.WriteFile(ctx, filepath.Join(ch.Home(), ".reactor", "canaries.json"), 0o644, data); err != nil {
		return err
	}
	// Bait path table for the syscall collector (reactor-collect --bait).
	type bp struct {
		Path  string `json:"path"`
		Label string `json:"label"`
	}
	var paths []bp
	for _, f := range b.Files {
		if f.Bait {
			paths = append(paths, bp{f.Path, f.Label})
		}
	}
	bpData, _ := json.Marshal(paths)
	return ch.WriteFile(ctx, filepath.Join(ch.Home(), ".reactor", "bait.json"), 0o644, bpData)
}

// installArtifact copies the artifact into the chamber and returns the argv that
// runs it (relative to the install dir).
func (e *Engine) installArtifact(ctx context.Context, ch chamber.Chamber, art events.Artifact, installDir string) ([]string, error) {
	if src := art.Env["_dir"]; src != "" {
		if err := ch.UploadDir(ctx, src, "artifact"); err != nil {
			return nil, err
		}
	}
	source := art.Source
	if source == "" {
		source = "node server.mjs"
	}
	return splitFields(source), nil
}

func baitReport(evs []events.Event, b *bait.Set) events.BaitReport {
	read := map[string]bool{}
	exfil := map[string]bool{}
	ctxLeak := false
	for _, e := range evs {
		if e.Behavioral == nil {
			continue
		}
		bh := e.Behavioral
		if bh.Bait && bh.BaitLabel != "" {
			read[bh.BaitLabel] = true
		}
		for _, ck := range bh.CanaryKinds {
			if strings.HasPrefix(ck, "file") {
				if p := strings.SplitN(ck, ":", 2); len(p) == 2 {
					exfil[p[1]] = true
				}
			}
			if strings.HasPrefix(ck, "context") || strings.HasPrefix(ck, "conversation") {
				ctxLeak = true
			}
		}
		for _, tok := range bh.Canaries {
			if c, ok := b.Lookup(tok); ok {
				exfil[c.Label] = true
				if c.Kind == "context" || c.Kind == "conversation" {
					ctxLeak = true
				}
			}
		}
	}
	return events.BaitReport{Read: keysOf(read), Exfiltrated: keysOf(exfil), ContextCanaryLeaked: ctxLeak}
}

// runScan performs the static baseline and records it on the detonation.
func (e *Engine) runScan(ctx context.Context, det *Detonation, art events.Artifact, installDir string, artArgv []string) {
	if art.Kind != events.KindMCPServer {
		return // static description scan only applies to MCP servers
	}
	// The static baseline launches the server to pull tools/list — on the host,
	// which is exactly the thing the incumbent scanners do and Reactor exists to
	// point at (SPEC §2). That is tolerable for a curated zoo entry an operator
	// chose. It is not tolerable for anything else: /api/detonate is unauthenticated
	// and CORS-open by design (it runs on the visitor's own machine), so any page
	// the visitor happens to be browsing can POST an inline artifact. If that
	// artifact's launch command reached the host it would be drive-by host RCE
	// with no chamber anywhere in the picture.
	//
	// So the baseline runs only for an artifact that IS a catalog entry, exactly
	// as curated — same command, same directory. Everything else (ingested or
	// caller-supplied) gets no host-side baseline; its column reads unavailable.
	if art.Env["_ingest"] != "" || !e.isCuratedZooEntry(art) {
		return
	}
	res := scan.Run(ctx, scan.Options{
		Name: art.Name, Argv: artArgv, Dir: art.Env["_dir"],
		Emit: func(l events.ScanLine) {
			det.emit(e.bus, events.Event{Kind: events.KindScan, Scan: &l})
		},
	})
	det.mu.Lock()
	det.scan = &res.ScanResult
	det.mu.Unlock()
}

// isCuratedZooEntry reports whether this artifact is a catalog entry the
// operator curated, unmodified in the two fields that decide what a host-side
// static baseline would execute: the launch command and its directory. A
// request that borrows a zoo id but substitutes either one is not a zoo entry.
func (e *Engine) isCuratedZooEntry(art events.Artifact) bool {
	z, ok := e.ArtifactByID(art.ID)
	if !ok {
		return false
	}
	return z.Source == art.Source && z.Env["_dir"] == art.Env["_dir"]
}

// ---- small helpers ----

func (d *Detonation) homeGuess() string {
	// The report doesn't carry the chamber home; the install-dir prefix is only
	// used for the benign-profile in_install heuristic, which the collectors
	// already resolve. Return empty so oracle uses collector-provided flags.
	return ""
}

func life(phase, msg string, info *events.ChamberInfo) events.Event {
	return events.Event{Kind: events.KindLifecycle, Lifecycle: &events.Lifecycle{Phase: phase, Message: msg, Chamber: info}}
}

func sessionLife(phase, msg string, s int) events.Event {
	ev := life(phase, msg, nil)
	ev.Session = s
	return ev
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---- staging the chamber components ----

// stageBins makes the host-built chamber components runnable inside the chamber
// and returns the argv[0] paths to use for one detonation. The local driver
// hands back the host path unchanged; a remote driver uploads each binary and
// returns a sandbox path. The input `bins` is copied, never written through:
// detonations run concurrently against the engine's single shared copy.
func stageBins(ctx context.Context, ch chamber.Chamber, hostBins bins) (bins, error) {
	staged := hostBins
	for _, b := range []struct {
		name string
		path *string
	}{
		{"victim", &staged.victim},
		{"wire", &staged.wire},
		{"sink", &staged.sink},
		{"collect", &staged.collect},
	} {
		if *b.path == "" {
			continue // collect is optional; the others are checked at New
		}
		in, err := ch.StageBinary(ctx, *b.path)
		if err != nil {
			return bins{}, fmt.Errorf("%s: %w", b.name, err)
		}
		if in == "" {
			return bins{}, fmt.Errorf("%s: chamber returned no path", b.name)
		}
		*b.path = in
	}
	return staged, nil
}

// binSummary describes what staging is about to move, so a UI watching a
// multi-megabyte upload does not look hung.
func binSummary(b bins) string {
	var total int64
	n := 0
	for _, p := range []string{b.victim, b.wire, b.sink, b.collect} {
		if p == "" {
			continue
		}
		n++
		if st, err := os.Stat(p); err == nil {
			total += st.Size()
		}
	}
	return fmt.Sprintf("%d binaries, %.1f MB", n, float64(total)/(1<<20))
}

// ---- sink port selection + readiness, both answered inside the chamber ----

const (
	defaultSinkPort = 9931
	// sinkProbeTries × the script's 250ms pause bounds how long a sink that
	// never binds is waited on before the detonation fails.
	sinkProbeTries = 40
)

// pickSinkPort chooses the sink's port by asking the CHAMBER what is free, not
// the host. Host availability says nothing about a remote sandbox and would
// silently shift the port for no reason; inside the chamber the question is
// exactly right, and on the local driver it is the same question as before
// because the chamber shares the host's network namespace.
func pickSinkPort(ctx context.Context, ch chamber.Chamber, pref int) int {
	for _, p := range sinkPortCandidates(pref) {
		res, err := ch.Exec(ctx, chamber.ExecOpts{
			Cmd: []string{"sh", "-c", portProbeScript(p, 1)}, Timeout: 30 * time.Second,
		})
		if err != nil || res.ExitCode == probeNoTool {
			return freePort(pref) // can't ask; the host heuristic is all we have
		}
		if res.ExitCode != 0 {
			return p // nothing answered there: free
		}
	}
	return freePort(pref)
}

func sinkPortCandidates(pref int) []int {
	out := []int{pref}
	for i := 0; i < 6; i++ {
		out = append(out, 20000+rand.Intn(30000))
	}
	return out
}

// waitForSink blocks until the sink is accepting connections INSIDE the
// chamber. The sink listens on the chamber's loopback, so dialling the host's
// loopback answers a different question entirely — remotely it can never
// connect, which made a dead sink indistinguishable from a live one.
//
// Reports whether readiness was actually proven: a chamber with no probe tool
// yields (false, nil) so the caller can say so rather than claim containment.
func waitForSink(ctx context.Context, ch chamber.Chamber, port int) (bool, error) {
	res, err := ch.Exec(ctx, chamber.ExecOpts{
		Cmd: []string{"sh", "-c", portProbeScript(port, sinkProbeTries)}, Timeout: 90 * time.Second,
	})
	if err != nil {
		return false, fmt.Errorf("probing 127.0.0.1:%d inside the chamber: %w", port, err)
	}
	switch res.ExitCode {
	case 0:
		return true, nil
	case probeNoTool:
		return false, nil
	}
	return false, fmt.Errorf("nothing is listening on 127.0.0.1:%d inside the chamber — the sink did not start", port)
}

// sinkDiagnostics reads the sink's own stdout/stderr out of the chamber and its
// exit status off the handle. Without this the operator sees only that nothing
// answered on the port and has to go into the sandbox by hand to find out why —
// which is exactly what a missing logs/ directory cost the first time.
func sinkDiagnostics(ctx context.Context, ch chamber.Chamber, h chamber.Handle) string {
	dctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var parts []string
	if h != nil {
		// The sink is dead or it would have answered the probe, so a status is
		// there to be read; a short budget keeps a hung one from blocking here.
		wctx, wcancel := context.WithTimeout(dctx, 5*time.Second)
		if code, err := h.Wait(wctx); err == nil {
			parts = append(parts, fmt.Sprintf("sink exited %d", code))
		}
		wcancel()
	}
	for _, p := range []string{"logs/sink.err", "logs/sink.out"} {
		data, err := ch.ReadFile(dctx, p)
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			continue
		}
		parts = append(parts, p+": "+truncate(strings.TrimSpace(string(data)), 500))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

// probeNoTool is the probe script's "I have no way to answer" exit code, kept
// distinct from a plain negative so callers never read it as evidence.
const probeNoTool = 2

// portProbeScript renders a POSIX-sh script that polls tcp 127.0.0.1:port from
// wherever it runs, retrying up to `tries` times. Exit 0: something answered.
// Exit 1: nothing did. Exit 2 (probeNoTool): no probe tool present.
//
// The probe is deliberately TCP-only by default. An earlier curl GET / against
// the sink was logged as egress_http and the install_hook oracle treated our
// own readiness check as an install-time beacon — every benign control then
// came back MALICIOUS. python3's connect_ex is preferred (no HTTP, no body);
// nc -z covers images without python. curl is last-resort only, aimed at the
// sink's unlogged /healthz so a pure-curl image still cannot poison the log.
func portProbeScript(port, tries int) string {
	py := fmt.Sprintf(`import socket,sys
s=socket.socket(); s.settimeout(2)
sys.exit(0 if s.connect_ex(("127.0.0.1",%d))==0 else 1)`, port)
	return fmt.Sprintf(`if command -v python3 >/dev/null 2>&1; then
  probe() { python3 -c %[2]s >/dev/null 2>&1; }
elif command -v nc >/dev/null 2>&1; then
  probe() { nc -z -w 2 127.0.0.1 %[1]d >/dev/null 2>&1; }
elif command -v curl >/dev/null 2>&1; then
  # Last resort. /healthz is answered by the sink without writing sink.jsonl.
  probe() { curl -s -o /dev/null -m 2 --noproxy '*' "http://127.0.0.1:%[1]d/healthz"; [ $? -ne 7 ]; }
else
  exit %[4]d
fi
i=0
while [ $i -lt %[3]d ]; do
  if probe; then exit 0; fi
  i=$((i+1))
  if [ $i -lt %[3]d ]; then sleep 0.25; fi
done
exit 1
`, port, shQuote(py), tries, probeNoTool)
}

// shQuote makes a value safe inside single quotes for a chamber shell.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// freePort is the host-side fallback used only when the chamber cannot be asked
// (no probe tool, or exec failed). On the local driver it is exactly right.
func freePort(pref int) int {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", pref))
	if err == nil {
		defer l.Close()
		return pref
	}
	l, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return pref + 1
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func splitFields(s string) []string {
	var out []string
	var cur []rune
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
		case r == ' ' && !inQ:
			if len(cur) > 0 {
				out = append(out, string(cur))
				cur = cur[:0]
			}
		default:
			cur = append(cur, r)
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}
