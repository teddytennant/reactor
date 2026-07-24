// Package engine is the host-side control plane (SPEC §12.1). It owns the
// chamber lifecycle, drives N sessions per artifact, republishes the chamber's
// typed events onto a bus with stable evidence ids, runs the deterministic
// oracles, asks the analyst for a verdict, and always destroys the chamber.
//
// The hard rule (SPEC §12.1): the host never executes the artifact (only a
// chamber driver does) and never feeds artifact text to the analyst (only
// events.ForAnalyst output crosses that line).
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/reactor-sec/reactor/internal/analyst"
	"github.com/reactor-sec/reactor/internal/chamber"
	"github.com/reactor-sec/reactor/internal/chamber/local"
	"github.com/reactor-sec/reactor/internal/events"
	"github.com/reactor-sec/reactor/internal/oai"
	"github.com/reactor-sec/reactor/internal/victim"
)

// Config tunes the engine.
type Config struct {
	BinDir          string
	ZooPath         string
	DefaultSessions int
	Deterministic   bool
	Seed            string
	VictimBackend   string // "", "auto", "xai", "sglang", "sim"
	Task            string

	// Ingest (see ingest.go). Every one of these is a ceiling on attacker-chosen
	// bytes arriving before any chamber exists, so all of them have a default
	// and none of them may be zero at run time.
	WorkDir         string        // where uploads and clones are staged
	MaxUploadBytes  int64         // one uploaded archive
	MaxExtractBytes int64         // what an archive or a clone may become on disk
	MaxExtractFiles int           // how many files it may become
	MaxCloneBytes   int64         // a clone is killed the moment it crosses this
	CloneTimeout    time.Duration // wall clock for one `git clone`
	UploadTTL       time.Duration // how long an unclaimed upload is kept
	AllowLocalRepos bool          // permit file:// and private hosts (dev only)
}

// Ingest defaults. 64 MiB is a generous MCP server or skill bundle and a
// miserly memory-exhaustion budget; the unpack ceilings are four times that
// because an archive legitimately grows, and a zip bomb grows far more.
const (
	defaultMaxUploadBytes  = 64 << 20
	defaultMaxExtractBytes = 256 << 20
	defaultMaxExtractFiles = 20000
	defaultMaxCloneBytes   = 256 << 20
	defaultCloneTimeout    = 90 * time.Second
	defaultUploadTTL       = 2 * time.Hour
	// ingestPrefix names this engine's staging root under WorkDir; sweepStale
	// recognises abandoned ones by it.
	ingestPrefix = "ingest-"
	// staleIngestAge is how long an ingest root from a previous, killed engine
	// is left alone before it is swept.
	staleIngestAge = 24 * time.Hour
)

// Engine is the control plane.
type Engine struct {
	cfg      Config
	drivers  []chamber.Driver
	bins     bins
	bus      *events.Bus
	workRoot string // this engine's ingest staging root; removed by Close

	mu      sync.Mutex
	zoo     []events.Artifact
	reports map[string]*Detonation
	order   []string // newest last
	uploads map[string]*stagedUpload
	works   map[string]string // detonation id -> its ingest working directory
}

// bins are the paths to the chamber component binaries.
type bins struct{ victim, wire, sink, collect string }

// Detonation is one run's live state and its report.
type Detonation struct {
	Report  *events.DetonationReport
	idgen   *events.IDGen
	startMs int64
	// creds are visitor BYOK keys for this run only. Never copied into Report.
	creds RunCredentials

	mu     sync.Mutex
	events []events.Event
	scan   *events.ScanResult
	done   chan struct{}
}

// New builds an engine. It selects drivers (Daytona if configured, else local)
// and locates the chamber binaries.
func New(cfg Config) (*Engine, error) {
	if cfg.DefaultSessions == 0 {
		cfg.DefaultSessions = 5
	}
	if cfg.ZooPath == "" {
		cfg.ZooPath = "zoo/index.json"
	}
	if cfg.Task == "" {
		cfg.Task = "Summarize what this repository does."
	}
	applyIngestDefaults(&cfg)
	e := &Engine{
		cfg:      cfg,
		bus:      events.NewBus(50000),
		reports:  map[string]*Detonation{},
		uploads:  map[string]*stagedUpload{},
		works:    map[string]string{},
		workRoot: filepath.Join(cfg.WorkDir, ingestPrefix+newID()),
	}
	if err := e.locateBins(cfg.BinDir); err != nil {
		return nil, err
	}
	// Only now that the engine is certainly usable: no point leaving a staging
	// directory behind for a New that is about to fail.
	sweepStale(cfg.WorkDir, staleIngestAge)
	if err := os.MkdirAll(e.workRoot, 0o700); err != nil {
		return nil, fmt.Errorf("ingest staging dir %s: %w", cfg.WorkDir, err)
	}
	e.drivers = selectDrivers()
	if zoo, err := loadZoo(cfg.ZooPath); err == nil {
		e.zoo = zoo
	}
	return e, nil
}

// applyIngestDefaults fills the ingest ceilings. A zero here would mean "no
// limit" at the point it is used, which is the opposite of what an unset field
// should mean, so every one of them is defaulted rather than checked later.
func applyIngestDefaults(cfg *Config) {
	if cfg.WorkDir == "" {
		cfg.WorkDir = os.Getenv("REACTOR_WORK_DIR")
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = filepath.Join(os.TempDir(), "reactor")
	}
	if cfg.MaxUploadBytes <= 0 {
		cfg.MaxUploadBytes = defaultMaxUploadBytes
	}
	if cfg.MaxExtractBytes <= 0 {
		cfg.MaxExtractBytes = defaultMaxExtractBytes
	}
	if cfg.MaxExtractFiles <= 0 {
		cfg.MaxExtractFiles = defaultMaxExtractFiles
	}
	if cfg.MaxCloneBytes <= 0 {
		cfg.MaxCloneBytes = defaultMaxCloneBytes
	}
	if cfg.CloneTimeout <= 0 {
		cfg.CloneTimeout = defaultCloneTimeout
	}
	if cfg.UploadTTL <= 0 {
		cfg.UploadTTL = defaultUploadTTL
	}
	if !cfg.AllowLocalRepos {
		cfg.AllowLocalRepos = os.Getenv("REACTOR_ALLOW_LOCAL_REPOS") == "1"
	}
}

// Bus exposes the event bus for the SSE handler.
func (e *Engine) Bus() *events.Bus { return e.bus }

// Zoo returns the loaded artifact catalog.
func (e *Engine) Zoo() []events.Artifact {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]events.Artifact(nil), e.zoo...)
}

// Drivers reports driver availability for /api/health.
func (e *Engine) Drivers() []map[string]any {
	var out []map[string]any
	for _, d := range e.drivers {
		out = append(out, map[string]any{"name": d.Name(), "available": d.Available(), "why": d.Why()})
	}
	return out
}

// AnalystName reports which analyst will write verdicts.
func (e *Engine) AnalystName() string {
	if analystForced() {
		return "deterministic"
	}
	if _, ok := oai.FromEnv(); ok {
		return firstEnv("ANALYST_MODEL", "VICTIM_MODEL", "grok-4.5")
	}
	return "deterministic"
}

// primaryDriver returns the first available driver (Daytona preferred).
func (e *Engine) primaryDriver() chamber.Driver {
	for _, d := range e.drivers {
		if d.Available() {
			return d
		}
	}
	return local.New()
}

// Report returns a detonation report by id.
func (e *Engine) Report(id string) (*events.DetonationReport, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	d, ok := e.reports[id]
	if !ok {
		return nil, false
	}
	return d.snapshot(), true
}

// Reports returns all detonation summaries, newest first.
func (e *Engine) Reports() []*events.DetonationReport {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*events.DetonationReport, 0, len(e.order))
	for i := len(e.order) - 1; i >= 0; i-- {
		if d, ok := e.reports[e.order[i]]; ok {
			out = append(out, d.snapshot())
		}
	}
	return out
}

// ArtifactByID finds a zoo artifact.
func (e *Engine) ArtifactByID(id string) (events.Artifact, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, a := range e.zoo {
		if a.ID == id {
			return a, true
		}
	}
	return events.Artifact{}, false
}

// snapshot returns a copy of the report with the collected events attached.
func (d *Detonation) snapshot() *events.DetonationReport {
	d.mu.Lock()
	defer d.mu.Unlock()
	r := *d.Report
	r.Events = append([]events.Event(nil), d.events...)
	if d.scan != nil {
		s := *d.scan
		r.Scan = &s
	}
	return &r
}

// emit stamps ids, normalises timestamps to detonation-relative, records the
// event, and broadcasts it. Serialised so the idgen and event slice stay ordered.
func (d *Detonation) emit(bus *events.Bus, ev events.Event) events.Event {
	d.mu.Lock()
	if ev.TSms == 0 {
		ev.TSms = nowMs()
	}
	if ev.TSms > 1_000_000_000_000 { // absolute unix ms -> relative
		ev.TSms -= d.startMs
		if ev.TSms < 0 {
			ev.TSms = 0
		}
	}
	ev.DetonationID = d.Report.DetonationID
	ev.ArtifactID = d.Report.ArtifactID
	d.idgen.Next(&ev)
	d.events = append(d.events, ev)
	d.mu.Unlock()
	bus.Publish(ev)
	return ev
}

func (d *Detonation) collected() []events.Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]events.Event(nil), d.events...)
}

// ---- zoo loading ----

// loadZoo reads the catalog and resolves each entry to the directory holding
// it. The directory is discovered by walking for per-artifact reactor.json
// manifests rather than guessed from the id — ids use underscores while folders
// use hyphens, and benign controls live one level deeper, so guessing breaks.
// Entries with no manifest on disk (the live npx pins) simply have no dir and
// are run straight from their `source` command.
func loadZoo(path string) ([]events.Artifact, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw []zooEntry
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	root := filepath.Dir(path)
	dirs := discoverManifests(root)

	var out []events.Artifact
	for _, z := range raw {
		a := events.Artifact{
			ID: z.ID, Kind: z.Kind, Name: z.Name, Source: z.Source, Label: z.Label, Note: z.Note,
		}
		if a.ID == "" {
			a.ID = "art_" + sanitize(z.Name)
		}
		a.Env = map[string]string{}
		switch {
		case z.Dir != "":
			a.Env["_dir"] = filepath.Join(root, z.Dir)
		case dirs[a.ID] != "":
			a.Env["_dir"] = dirs[a.ID]
		}
		if z.Install != "" {
			a.Env["_install"] = z.Install
		}
		if z.Live {
			a.Env["_live"] = "1"
		}
		if len(z.Expect) > 0 {
			a.Env["_expect"] = strings.Join(z.Expect, ",")
		}
		a.Env["_family"] = z.Family
		if z.StaticBlind {
			a.Env["_static_blind"] = "1"
		}
		if z.Lead {
			a.Env["_lead"] = "1"
		}
		out = append(out, a)
	}
	return out, nil
}

// discoverManifests maps artifact id -> directory by finding reactor.json files.
func discoverManifests(root string) map[string]string {
	dirs := map[string]string{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != "reactor.json" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		var m struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(b, &m) == nil && m.ID != "" {
			dirs[m.ID] = filepath.Dir(p)
		}
		return nil
	})
	return dirs
}

type zooEntry struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Install     string   `json:"install"`
	Dir         string   `json:"dir"`
	Note        string   `json:"note"`
	Label       string   `json:"label"`
	Family      string   `json:"family"`
	Expect      []string `json:"expect"`
	StaticBlind bool     `json:"static_blind"`
	Lead        bool     `json:"lead"`
	Live        bool     `json:"live"`
}

// ---- helpers ----

func (e *Engine) newAnalyst(steps analyst.StepSink, creds RunCredentials) analyst.Analyst {
	if client, ok := analystClient(creds); ok {
		return analyst.Grok{Client: client, Model: client.Model, Steps: steps}
	}
	return analyst.Deterministic{Steps: steps}
}

// analystForced lets REACTOR_ANALYST=deterministic|none pin the offline
// reasoner — used by the eval harness and fast test loops so a scorecard run
// doesn't spend a hosted call per artifact.
func analystForced() bool {
	v := strings.ToLower(os.Getenv("REACTOR_ANALYST"))
	return v == "deterministic" || v == "none" || v == "offline"
}

func (e *Engine) locateBins(binDir string) error {
	if binDir == "" {
		binDir = os.Getenv("REACTOR_BIN_DIR")
	}
	candidates := []string{binDir, "bin", "./bin"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}
	// Look for the Rust release binaries too, and prefer them for the sink and
	// collector (SPEC §12.2: the sink is Rust; the syscall collector is a hot
	// loop over untrusted trace output). The Go sink remains a fallback.
	candidates = append(candidates, "crates/target/release", "target/release")
	find := func(names ...string) string {
		for _, name := range names {
			for _, dir := range candidates {
				if dir == "" {
					continue
				}
				p := filepath.Join(dir, name)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					abs, _ := filepath.Abs(p)
					return abs
				}
			}
		}
		return ""
	}
	e.bins = bins{
		victim:  find("victim"),
		wire:    find("wire"),
		sink:    find("reactor-sink", "sink"), // Rust sink preferred, Go fallback
		collect: find("reactor-collect"),      // Rust-only; optional
	}
	var missing []string
	for name, p := range map[string]string{"victim": e.bins.victim, "wire": e.bins.wire, "sink": e.bins.sink} {
		if p == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("chamber binaries not found (%s); run `make build` or set REACTOR_BIN_DIR", strings.Join(missing, ", "))
	}
	return nil
}

func nowMs() int64 { return time.Now().UnixMilli() }

func newID() string { return strings.ReplaceAll(uuid.NewString(), "-", "")[:12] }

// victimInfo reports the intended victim backend for the report header. It does
// not read attacker text — it just names the model that will eat the poison.
// A visitor Fireworks key selects the fireworks backend for the label.
func (e *Engine) victimInfo(creds RunCredentials) events.VictimInfo {
	cfg := victim.Config{Backend: e.cfg.VictimBackend, Temp: 0, Seed: 7}
	if creds.FireworksAPIKey != "" {
		if cfg.Backend == "" || cfg.Backend == "auto" {
			cfg.Backend = "fireworks"
		}
		cfg.APIKey = creds.FireworksAPIKey
	}
	b := victim.Resolve(context.Background(), cfg)
	return b.Info()
}

func sanitize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r == '-' || r == '_' || r == '/' || r == '@' || r == ' ' || r == '.' {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	if len(keys) > 0 {
		return keys[len(keys)-1]
	}
	return ""
}
