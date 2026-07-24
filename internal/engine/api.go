package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/reactor-sec/reactor/internal/events"
)

// Handler returns the engine's HTTP API (docs/CONTRACT.md "Engine HTTP + SSE
// API"). The UI and the TUI both build against exactly this surface.
func (e *Engine) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", e.handleHealth)
	mux.HandleFunc("/api/artifacts", e.handleArtifacts)
	mux.HandleFunc("/api/upload", e.handleUpload)
	mux.HandleFunc("/api/detonate", e.handleDetonate)
	mux.HandleFunc("/api/detonations", e.handleDetonations)
	mux.HandleFunc("/api/detonations/", e.handleDetonation)
	mux.HandleFunc("/api/events", e.handleEvents)
	mux.HandleFunc("/api/scan", e.handleScan)
	mux.HandleFunc("/api/scorecard", e.handleScorecard)
	return cors(mux)
}

func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Reactor-Daytona-Key, X-Reactor-Daytona-Url, X-Reactor-Fireworks-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		// Private Network Access (Chromium): a page on a public origin reaching
		// a loopback server must be granted it explicitly, on the preflight, or
		// the request is blocked before it arrives. The deployed console at
		// reactor.teddytennant.com talking to the visitor's own engine on
		// 127.0.0.1 is exactly that case — without this the browser refuses and
		// the console reports the engine as unreachable.
		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (e *Engine) handleHealth(w http.ResponseWriter, r *http.Request) {
	v := e.victimInfo(RunCredentials{})
	writeJSON(w, map[string]any{
		"ok":      true,
		"drivers": e.Drivers(),
		"analyst": e.AnalystName(),
		"victim": map[string]any{
			"model": v.Model, "served": v.Served, "simulated": v.Simulated,
			"temp": v.Temp, "seed": v.Seed, "revision": v.Revision,
		},
		"zoo": len(e.Zoo()),
	})
}

func (e *Engine) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, e.Zoo())
}

func (e *Engine) handleDetonate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req DetonateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Headers fill any field the JSON body left empty (same contract as upload).
	req.Credentials = credentialsFromRequest(r, req.Credentials)
	// Ingest failures carry their own status (413 oversize, 415 unsupported
	// archive, 504 clone timeout); everything else stays the 400 it always was.
	id, err := e.Detonate(req)
	if err != nil {
		httpFail(w, err)
		return
	}
	writeJSON(w, map[string]string{"detonation_id": id})
}

func (e *Engine) handleDetonations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, e.Reports())
}

func (e *Engine) handleDetonation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/detonations/")
	rep, ok := e.Report(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, rep)
}

func (e *Engine) handleScan(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("detonation")
	rep, ok := e.Report(id)
	if !ok || rep.Scan == nil {
		writeJSON(w, events.ScanResult{Status: "unavailable"})
		return
	}
	writeJSON(w, rep.Scan)
}

// handleEvents is the SSE stream: replay history, then live, filtered by
// detonation id. Framing is `event: <kind>\ndata: <Event json>\n\n`.
func (e *Engine) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	want := r.URL.Query().Get("detonation")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	ch, replay := e.bus.Subscribe(ctx, 1024)

	send := func(ev events.Event) bool {
		if want != "" && ev.DetonationID != want {
			return true
		}
		b, err := json.Marshal(ev)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	for _, ev := range replay {
		if !send(ev) {
			return
		}
	}

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if !send(ev) {
				return
			}
		case <-ping.C:
			fmt.Fprint(w, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}

// handleScorecard serves the offline eval output when present, else derives a
// live scorecard from the detonations this engine has run.
func (e *Engine) handleScorecard(w http.ResponseWriter, r *http.Request) {
	for _, p := range []string{"eval/scorecard.json", "eval/out/scorecard.json"} {
		if b, err := os.ReadFile(p); err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
	}
	writeJSON(w, e.liveScorecard())
}

// liveScorecard computes the SPEC §7 metrics from completed detonations.
func (e *Engine) liveScorecard() map[string]any {
	reports := e.Reports()
	var malCaught, malTotal, benTotal, falseBlocks, staticBlind, caught int
	var ttvSum, ttvN int64
	var costSum float64
	byType := map[string]int{}

	for _, rep := range reports {
		if rep.Verdict == nil {
			continue
		}
		art, _ := e.ArtifactByID(rep.ArtifactID)
		expectedBenign := art.Label == "" || art.Label == "benign"
		blocked := rep.Verdict.Label != events.LabelAllowed

		if expectedBenign {
			benTotal++
			if blocked {
				falseBlocks++
			}
		} else {
			malTotal++
			if blocked {
				malCaught++
			}
		}
		if blocked {
			caught++
			blindHere := false
			for _, s := range rep.Signals {
				byType[s.Type]++
				if s.StaticBlind {
					blindHere = true
				}
			}
			if blindHere {
				staticBlind++
			}
		}
		if rep.Verdict.TimeToVerdictMs > 0 {
			ttvSum += rep.Verdict.TimeToVerdictMs
			ttvN++
		}
		costSum += rep.Verdict.CostUSD
	}

	detection := 0.0
	if malTotal > 0 {
		detection = float64(malCaught) / float64(malTotal)
	}
	fqr := 0.0
	if benTotal > 0 {
		fqr = float64(falseBlocks) / float64(benTotal)
	}
	blindRate := 0.0
	if caught > 0 {
		blindRate = float64(staticBlind) / float64(caught)
	}
	meanTTV := int64(0)
	if ttvN > 0 {
		meanTTV = ttvSum / ttvN
	}
	return map[string]any{
		"source":                  "live",
		"detonations":             len(reports),
		"malicious_total":         malTotal,
		"malicious_caught":        malCaught,
		"detection_rate":          detection,
		"benign_total":            benTotal,
		"false_blocks":            falseBlocks,
		"false_quarantine_rate":   fqr,
		"catches":                 caught,
		"static_blind_catches":    staticBlind,
		"static_blind_rate":       blindRate,
		"mean_time_to_verdict_ms": meanTTV,
		"cost_usd_total":          costSum,
		"signals_by_type":         byType,
		"generated_ms":            nowMs(),
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

var _ = strconv.Itoa
