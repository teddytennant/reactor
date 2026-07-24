package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/reactor-sec/reactor/internal/events"
)

// newTestEngine builds an engine with stub chamber binaries and a two-artifact
// zoo. It never detonates anything — these tests cover the HTTP contract in
// docs/CONTRACT.md, which the UI and the TUI both code against.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	binDir := t.TempDir()
	for _, name := range []string{"victim", "wire", "sink"} {
		writeFile(t, filepath.Join(binDir, name), "#!/bin/sh\nexit 0\n")
	}
	zooDir := t.TempDir()
	writeFile(t, filepath.Join(zooDir, "index.json"), `[
	  {"id":"art_notes_mcp","kind":"mcp_server","name":"notes-mcp","source":"node server.mjs","label":"rug_pull"},
	  {"id":"art_echo_mcp","kind":"mcp_server","name":"echo-mcp","source":"node server.mjs"}
	]`)

	e, err := New(Config{
		BinDir: binDir, ZooPath: filepath.Join(zooDir, "index.json"),
		VictimBackend: "sim", DefaultSessions: 3,
		// Ingest stages under here, so a test run never touches the shared temp
		// root and t.TempDir cleanup proves Close left nothing behind.
		WorkDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

func TestLocateBinsFailsLoudlyWhenMissing(t *testing.T) {
	_, err := New(Config{BinDir: t.TempDir(), ZooPath: filepath.Join(t.TempDir(), "none.json")})
	if err == nil {
		t.Fatal("engine started with no chamber binaries — a detonation would fail mid-run instead")
	}
	// The message has to name what to do about it.
	for _, want := range []string{"victim", "sink", "wire", "make build"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestHealthReportsDriversAnalystAndVictim(t *testing.T) {
	e := newTestEngine(t)
	rec := do(t, e, "GET", "/api/health", "")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		OK      bool             `json:"ok"`
		Analyst string           `json:"analyst"`
		Zoo     int              `json:"zoo"`
		Drivers []map[string]any `json:"drivers"`
		Victim  struct {
			Model     string `json:"model"`
			Served    string `json:"served"`
			Simulated bool   `json:"simulated"`
		} `json:"victim"`
	}
	decode(t, rec.Body.Bytes(), &body)

	if !body.OK || body.Zoo != 2 {
		t.Fatalf("health = %+v", body)
	}
	// The local driver is always present as a fallback, so a detonation can
	// never be left with nowhere to run.
	var hasLocal bool
	for _, d := range body.Drivers {
		if d["name"] == "local" && d["available"] == true {
			hasLocal = true
		}
	}
	if !hasLocal {
		t.Errorf("no available local driver in %v", body.Drivers)
	}
	// A simulated victim must be labelled as such — the header strip says so
	// on camera, and a sim run is never presentable as a real model result.
	if body.Victim.Served != "sim" || !body.Victim.Simulated {
		t.Errorf("sim victim not marked simulated: %+v", body.Victim)
	}
}

func TestArtifactsListDoesNotLeakGroundTruthToTheStream(t *testing.T) {
	e := newTestEngine(t)
	rec := do(t, e, "GET", "/api/artifacts", "")
	var arts []events.Artifact
	decode(t, rec.Body.Bytes(), &arts)
	if len(arts) != 2 {
		t.Fatalf("got %d artifacts", len(arts))
	}
	// label is eval ground truth. It rides on the catalog (the offline scorecard
	// needs it) but must never reach the analyst — assert the boundary type has
	// no way to carry it.
	ev := events.Event{Kind: events.KindWire, Wire: &events.WireEvent{Method: "tools/list"}}
	blob, _ := json.Marshal(ev.ForAnalyst())
	if strings.Contains(string(blob), "rug_pull") {
		t.Fatalf("ground truth reachable from the analyst view: %s", blob)
	}
}

func TestDetonateRejectsUnknownAndUnspecifiedArtifacts(t *testing.T) {
	e := newTestEngine(t)
	cases := []struct {
		name, body string
	}{
		{"unknown id", `{"artifact_id":"art_nope"}`},
		{"nothing specified", `{}`},
		{"artifact with no source", `{"artifact":{"name":"x"}}`},
	}
	for _, c := range cases {
		rec := do(t, e, "POST", "/api/detonate", c.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (body %q)", c.name, rec.Code, rec.Body.String())
		}
	}
	if rec := do(t, e, "GET", "/api/detonate", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/detonate = %d, want 405", rec.Code)
	}
	if rec := do(t, e, "POST", "/api/detonate", `{not json`); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", rec.Code)
	}
	// Nothing above should have created a detonation.
	rec := do(t, e, "GET", "/api/detonations", "")
	var reports []*events.DetonationReport
	decode(t, rec.Body.Bytes(), &reports)
	if len(reports) != 0 {
		t.Fatalf("rejected requests created %d detonations", len(reports))
	}
}

func TestUnknownDetonationIs404AndScanIsUnavailable(t *testing.T) {
	e := newTestEngine(t)
	if rec := do(t, e, "GET", "/api/detonations/det_missing", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
	// /api/scan degrades to a typed "unavailable" rather than an error, so the
	// UI's left column always has something to render.
	rec := do(t, e, "GET", "/api/scan?detonation=det_missing", "")
	var res events.ScanResult
	decode(t, rec.Body.Bytes(), &res)
	if rec.Code != 200 || res.Status != "unavailable" || res.Available {
		t.Fatalf("scan fallback = %d %+v", rec.Code, res)
	}
}

func TestCORSPreflight(t *testing.T) {
	e := newTestEngine(t)
	rec := do(t, e, http.MethodOptions, "/api/detonate", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("no CORS origin header — the Next.js console cannot reach the engine")
	}
}

// The SSE framing is `event: <kind>\ndata: <Event json>\n\n`, history replayed
// before live events, filtered by ?detonation=. The console's whole timeline
// depends on this shape.
func TestEventsSSEReplaysHistoryFramedByKindAndFiltersByDetonation(t *testing.T) {
	e := newTestEngine(t)
	e.bus.Publish(events.Event{ID: "life:1", Kind: events.KindLifecycle, DetonationID: "det_a",
		Lifecycle: &events.Lifecycle{Phase: events.PhaseQueued, Message: "queued"}})
	e.bus.Publish(events.Event{ID: "wire:1:tools/list", Kind: events.KindWire, DetonationID: "det_a",
		Wire: &events.WireEvent{Method: "tools/list", Tool: "search"}})
	e.bus.Publish(events.Event{ID: "life:9", Kind: events.KindLifecycle, DetonationID: "det_OTHER",
		Lifecycle: &events.Lifecycle{Phase: events.PhaseQueued}})

	// Replay is synchronous, so a short window is enough; the handler then
	// streams until the request context ends.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest("GET", "/api/events?detonation=det_a", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Handler().ServeHTTP(rec, req)
	}()
	// The handler streams until the request context ends.
	<-ctx.Done()
	<-done

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	frames := parseSSE(t, rec.Body.String())
	if len(frames) < 2 {
		t.Fatalf("expected the two det_a events replayed, got %d: %v", len(frames), frames)
	}
	if frames[0].event != string(events.KindLifecycle) || frames[1].event != string(events.KindWire) {
		t.Fatalf("frames must be named by kind and keep bus order: %v", frames)
	}
	for _, f := range frames {
		if f.event == "ping" {
			continue
		}
		var ev events.Event
		decode(t, []byte(f.data), &ev)
		if ev.DetonationID != "det_a" {
			t.Fatalf("another detonation's event leaked into the stream: %s", f.data)
		}
	}
}

// liveScorecard is what /api/scorecard serves when no offline eval output is on
// disk. The three headline rates (SPEC §7) are computed here, so their
// arithmetic — including the denominators — is worth pinning.
func TestLiveScorecardRates(t *testing.T) {
	e := newTestEngine(t)
	// 2 malicious (one caught with a static-blind signal, one missed),
	// 1 benign wrongly blocked.
	e.seed(t, "det_1", "art_notes_mcp", &events.Verdict{Label: events.LabelMalicious, TimeToVerdictMs: 1000},
		events.Signal{Type: events.SigRugPull, StaticBlind: true})
	e.seed(t, "det_2", "art_notes_mcp", &events.Verdict{Label: events.LabelAllowed, TimeToVerdictMs: 3000})
	e.seed(t, "det_3", "art_echo_mcp", &events.Verdict{Label: events.LabelSuspect, TimeToVerdictMs: 2000},
		events.Signal{Type: events.SigShadowing})
	// A run that never reached a verdict must not count in any denominator.
	e.seed(t, "det_4", "art_echo_mcp", nil)

	sc := e.liveScorecard()
	check := func(key string, want any) {
		t.Helper()
		if got := sc[key]; got != want {
			t.Errorf("%s = %v (%T), want %v", key, got, got, want)
		}
	}
	check("malicious_total", 2)
	check("malicious_caught", 1)
	check("detection_rate", 0.5)
	check("benign_total", 1)
	check("false_blocks", 1)
	check("false_quarantine_rate", 1.0)
	check("catches", 2)              // det_1 and det_3 were blocked
	check("static_blind_catches", 1) // only det_1 carried a static-blind signal
	check("static_blind_rate", 0.5)
	check("mean_time_to_verdict_ms", int64(2000)) // 1000+3000+2000 over 3 verdicts

	byType, _ := sc["signals_by_type"].(map[string]int)
	if byType[events.SigRugPull] != 1 || byType[events.SigShadowing] != 1 {
		t.Errorf("signals_by_type = %v", byType)
	}
}

func TestLiveScorecardWithNoDetonations(t *testing.T) {
	e := newTestEngine(t)
	sc := e.liveScorecard()
	// Empty denominators must yield 0, not NaN — the UI renders these directly.
	for _, k := range []string{"detection_rate", "false_quarantine_rate", "static_blind_rate"} {
		if v := sc[k].(float64); v != 0 {
			t.Errorf("%s = %v on an empty engine", k, v)
		}
	}
}

// ---- helpers ----

// seed registers a completed detonation without running one.
func (e *Engine) seed(t *testing.T, id, artifactID string, v *events.Verdict, sigs ...events.Signal) {
	t.Helper()
	det := &Detonation{
		idgen: events.NewIDGen(), done: make(chan struct{}),
		Report: &events.DetonationReport{DetonationID: id, ArtifactID: artifactID, Verdict: v, Signals: sigs},
	}
	close(det.done)
	e.mu.Lock()
	e.reports[id] = det
	e.order = append(e.order, id)
	e.mu.Unlock()
}

func do(t *testing.T, e *Engine, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	rec := httptest.NewRecorder()
	e.Handler().ServeHTTP(rec, r)
	return rec
}

func decode(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
}

type sseFrame struct{ event, data string }

func parseSSE(t *testing.T, raw string) []sseFrame {
	t.Helper()
	var out []sseFrame
	var cur sseFrame
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if cur.event != "" {
				out = append(out, cur)
			}
			cur = sseFrame{}
		default:
			t.Fatalf("unexpected SSE line %q — the framing contract is `event:`/`data:`/blank", line)
		}
	}
	return out
}
