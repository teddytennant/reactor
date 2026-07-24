package engine

import (
	"context"
	"encoding/json"
	"fmt"
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
type DetonateRequest struct {
	ArtifactID string           `json:"artifact_id"`
	Artifact   *events.Artifact `json:"artifact"`
	UploadID   string           `json:"upload_id"` // from POST /api/upload
	Repo       string           `json:"repo"`      // https git url to clone
	Ref        string           `json:"ref"`       // branch, tag or commit for repo
	Sessions   int              `json:"sessions"`
	Network    bool             `json:"network"`
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
	det := &Detonation{
		idgen:   events.NewIDGen(),
		startMs: nowMs(),
		done:    make(chan struct{}),
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

	driver := e.primaryDriver()
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

	home := ch.Home()
	sinkPort := freePort(9931)
	b := bait.New(bait.Options{
		Home: home, InstallDir: filepath.Join(home, "artifact"),
		SinkHost: "127.0.0.1", SinkPort: sinkPort,
		Deterministic: e.cfg.Deterministic, Seed: e.cfg.Seed + ":" + det.Report.DetonationID,
	})

	// Victim + chamber info for the header strip. Backend is resolved lazily by
	// the victim binary; we report the intended backend from config/env here.
	vinfo := e.victimInfo()
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
		Cmd:        []string{e.bins.sink, "--http", fmt.Sprintf("127.0.0.1:%d", sinkPort), "--dns", "", "--log-dir", logDir, "--canaries", canaryPath},
		Env:        map[string]string{"REACTOR_LOG_DIR": logDir, "REACTOR_CANARY_FILE": canaryPath},
		StdoutPath: "logs/sink.out", StderrPath: "logs/sink.err",
	})
	if err != nil {
		e.fail(det, "start sink: "+err.Error())
		return
	}
	defer sinkHandle.Kill()
	waitForSink(sinkPort)
	det.emit(e.bus, life(events.PhaseSinkUp, fmt.Sprintf("egress sink up on 127.0.0.1:%d — nothing egresses past here", sinkPort), nil))

	// Tail the chamber's collector logs and republish onto the bus.
	tctx, tcancel := context.WithCancel(ctx)
	e.tailLogs(tctx, det, ch, []string{"logs/wire.jsonl", "logs/transcript.jsonl", "logs/sink.jsonl", "logs/behavioral.jsonl"})

	// Run the sessions. A rug pull only exists across repetition, so we always
	// run N and let the oracle diff descriptions (SPEC §4.5).
	sinkURL := fmt.Sprintf("http://127.0.0.1:%d", sinkPort)
	for s := 1; s <= sessions; s++ {
		det.emit(e.bus, sessionLife(events.PhaseSessionStart, fmt.Sprintf("session %d/%d", s, sessions), s))
		if err := e.runSession(ctx, ch, det, art, b, s, installDir, artArgv, canaryPath, logDir, sinkURL, network); err != nil {
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
func (e *Engine) runSession(ctx context.Context, ch chamber.Chamber, det *Detonation, art events.Artifact, b *bait.Set, session int, installDir string, artArgv []string, canaryPath, logDir, sinkURL string, network bool) error {
	// victim -- wire --canaries --dir installDir -- <artifact argv>
	victimArgv := []string{
		e.bins.victim, "--session", strconv.Itoa(session), "--log-dir", logDir, "--task", e.cfg.Task, "--",
		e.bins.wire, "--session", strconv.Itoa(session), "--log-dir", logDir, "--canaries", canaryPath, "--dir", installDir, "--",
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
		return e.detonateNonMCP(ctx, ch, det, art, session, installDir, env)
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

// detonateNonMCP runs a skill/zip artifact's install and entrypoint under
// strace, then parses the trace with reactor-collect into behavioral evidence
// (install hooks, credential sweeps), and scans its shipped text for analyst
// injection. Only session 1 does the work; later sessions are no-ops so the
// rug-pull/repetition machinery does not apply to a one-shot payload.
func (e *Engine) detonateNonMCP(ctx context.Context, ch chamber.Chamber, det *Detonation, art events.Artifact, session int, installDir string, env map[string]string) error {
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
	if e.bins.collect != "" {
		baitJSON := filepath.Join(ch.Home(), ".reactor", "bait.json")
		_, _ = ch.Exec(ctx, chamber.ExecOpts{
			Cmd: []string{e.bins.collect,
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
	an := e.newAnalyst(steps)
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
	// chose. It is not tolerable for something a stranger just uploaded, so an
	// ingested artifact gets no host-side baseline; its column reads unavailable.
	if art.Env["_ingest"] != "" {
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

func waitForSink(port int) {
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
}

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
