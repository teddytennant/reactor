// Command reactor is the engine: the host-side control plane that provisions a
// disposable chamber per artifact, drives the victim agent against it, runs the
// deterministic oracles over the resulting evidence, asks the analyst for a
// verdict, and streams every typed event to the UI over SSE.
//
//	reactor serve                 — run the engine API on :8787 (what the UI talks to)
//	reactor detonate <artifact>   — run one detonation and print the verdict
//	reactor list                  — list the loaded zoo
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/reactor-sec/reactor/internal/dotenv"
	"github.com/reactor-sec/reactor/internal/engine"
	"github.com/reactor-sec/reactor/internal/events"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("reactor: ")
	dotenv.Load(".env", os.Getenv("HOME")+"/.reactor.env")

	var (
		addr     = flag.String("addr", envOr("REACTOR_ADDR", "127.0.0.1:8787"), "engine listen address")
		zoo      = flag.String("zoo", envOr("REACTOR_ZOO", "zoo/index.json"), "artifact catalog")
		binDir   = flag.String("bin", envOr("REACTOR_BIN_DIR", "bin"), "directory holding victim/wire/sink binaries")
		sessions = flag.Int("sessions", envInt("REACTOR_SESSIONS", 5), "detonation sessions per artifact")
		backend  = flag.String("victim", os.Getenv("REACTOR_VICTIM_BACKEND"), "victim backend: auto|fireworks|xai|sglang|sim")
		task     = flag.String("task", envOr("REACTOR_TASK", "Summarize what this repository does."), "the benign task given to the victim")
		det      = flag.Bool("deterministic", false, "derive canaries from a fixed seed (rehearsal mode)")
		jsonOut  = flag.Bool("json", false, "print the full report as JSON")
	)
	// Go's flag package stops at the first non-flag arg, which would silently
	// drop flags written after a positional (`detonate x --victim sim`). Split
	// flags from positionals ourselves so order never matters.
	flags, positionals := splitFlagsPositionals(os.Args[1:], map[string]bool{"deterministic": true, "json": true})
	flag.CommandLine.Parse(flags)
	cmd := "serve"
	var cmdArgs []string
	if len(positionals) > 0 {
		cmd, cmdArgs = positionals[0], positionals[1:]
	}

	e, err := engine.New(engine.Config{
		BinDir: *binDir, ZooPath: *zoo, DefaultSessions: *sessions,
		VictimBackend: *backend, Task: *task, Deterministic: *det, Seed: "reactor-demo",
	})
	if err != nil {
		log.Fatal(err)
	}

	args := cmdArgs

	switch cmd {
	case "serve":
		serve(e, *addr)
	case "list":
		for _, a := range e.Zoo() {
			label := a.Label
			if label == "" {
				label = "benign"
			}
			fmt.Printf("%-22s %-12s %-28s %s\n", a.ID, a.Kind, a.Name, label)
		}
	case "detonate":
		if len(args) < 1 {
			log.Fatal("usage: reactor detonate <artifact_id> [--sessions N] [--victim sim|fireworks]")
		}
		detonate(e, args[0], *sessions, *jsonOut)
	default:
		log.Fatalf("unknown command %q (serve|detonate|list)", cmd)
	}
}

// splitFlagsPositionals separates flag tokens (and their values) from
// positional args, so flags and positionals can appear in any order — which
// stdlib flag.Parse does not allow. bools names the boolean flags, which do not
// consume a following value.
func splitFlagsPositionals(argv []string, bools map[string]bool) (flags, positionals []string) {
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if !strings.HasPrefix(a, "-") {
			positionals = append(positionals, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") || bools[name] { // --k=v or a bool flag
			continue
		}
		if i+1 < len(argv) { // consume the flag's value
			flags = append(flags, argv[i+1])
			i++
		}
	}
	return flags, positionals
}

func serve(e *engine.Engine, addr string) {
	v := e.Drivers()
	log.Printf("engine on http://%s", addr)
	log.Printf("analyst: %s", e.AnalystName())
	for _, d := range v {
		log.Printf("driver %-8s available=%v — %s", d["name"], d["available"], d["why"])
	}
	log.Printf("zoo: %d artifacts", len(e.Zoo()))
	srv := &http.Server{Addr: addr, Handler: e.Handler(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// detonate runs one artifact to completion and prints a demo-shaped summary.
func detonate(e *engine.Engine, artifactID string, sessions int, asJSON bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	id, err := e.Detonate(engine.DetonateRequest{ArtifactID: artifactID, Sessions: sessions})
	if err != nil {
		log.Fatal(err)
	}

	ch, replay := e.Bus().Subscribe(ctx, 4096)
	seen := map[string]bool{}
	print := func(ev events.Event) bool {
		if ev.DetonationID != id || seen[ev.ID] {
			return false
		}
		seen[ev.ID] = true
		renderEvent(ev)
		return ev.Kind == events.KindLifecycle && ev.Lifecycle != nil && ev.Lifecycle.Phase == events.PhaseDestroyed
	}
	for _, ev := range replay {
		if print(ev) {
			goto done
		}
	}
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				goto done
			}
			if print(ev) {
				goto done
			}
		case <-time.After(13 * time.Minute):
			goto done
		}
	}
done:
	rep, ok := e.Report(id)
	if !ok {
		log.Fatal("no report")
	}
	if asJSON {
		b, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(b))
		return
	}
	renderSummary(rep)
	if rep.Verdict != nil && rep.Verdict.Label != events.LabelAllowed {
		os.Exit(2) // a blocked artifact is a non-zero exit, for scripting
	}
}

func renderEvent(ev events.Event) {
	switch ev.Kind {
	case events.KindLifecycle:
		if ev.Lifecycle != nil {
			fmt.Printf("  · %-14s %s\n", ev.Lifecycle.Phase, ev.Lifecycle.Message)
		}
	case events.KindScan:
		if ev.Scan != nil && ev.Scan.Text != "" {
			fmt.Printf("  [static] %s\n", ev.Scan.Text)
		}
	case events.KindWire:
		w := ev.Wire
		if w != nil && w.Method == "tools/list" && w.Tool != "" {
			fmt.Printf("  [wire s%d] %-8s desc=%d bytes sha=%s\n", ev.Session, w.Tool, w.DescriptionBytes, shortSHA(w.DescriptionSHA256))
		}
		if w != nil && w.Method == "tools/call" && w.Dir == "agent→server" && len(w.ArgCanaries) > 0 {
			fmt.Printf("  [wire s%d] %-8s CANARY IN ARGS: %v\n", ev.Session, w.Tool, w.ArgCanaries)
		}
	case events.KindTranscript:
		t := ev.Transcript
		if t != nil && t.Action == events.ActToolCall && !t.OnTask {
			fmt.Printf("  [victim s%d] DEVIATION calling %s — %s\n", ev.Session, t.Tool, t.Deviation)
		}
	case events.KindBehavioral:
		b := ev.Behavioral
		if b != nil && len(b.Canaries) > 0 {
			fmt.Printf("  [sink] %s %s canaries=%v (%v)\n", b.Op, b.Host, b.Canaries, b.CanaryKinds)
		}
	case events.KindSignal:
		if ev.Signal != nil {
			blind := ""
			if ev.Signal.StaticBlind {
				blind = "  [STATIC-BLIND]"
			}
			fmt.Printf("\n  ** %s · %s%s\n     %s\n     evidence: %s\n\n",
				strings.ToUpper(ev.Signal.Type), ev.Signal.Severity, blind, ev.Signal.Summary, strings.Join(ev.Signal.Evidence, ", "))
		}
	case events.KindAnalyst:
		if a := ev.Analyst; a != nil {
			line := a.Thought
			if a.Tool != "" {
				line = a.Tool + "() " + a.Result
			}
			if strings.TrimSpace(line) != "" {
				fmt.Printf("  [analyst %d] %s\n", a.Step, trunc(line, 160))
			}
		}
	}
}

func renderSummary(rep *events.DetonationReport) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 78))
	if rep.Scan != nil {
		fmt.Printf("  INDUSTRY STANDARD (%s): %s — %d issue(s)\n", rep.Scan.Tool, strings.ToUpper(rep.Scan.Status), rep.Scan.Issues)
	}
	v := rep.Verdict
	if v == nil {
		fmt.Println("  no verdict")
		return
	}
	fmt.Printf("  REACTOR: %s · %s · %s\n", v.Label, strings.ToUpper(v.Family), v.Severity)
	fmt.Printf("  %s\n", v.Explanation)
	fmt.Printf("  evidence: %s\n", strings.Join(v.Evidence, ", "))
	fmt.Printf("  sessions=%d  victim=%s (%s)  analyst=%s  time=%dms\n",
		rep.Sessions, rep.Victim.Model, rep.Victim.Served, v.Analyst, v.TimeToVerdictMs)
	if rep.Bait.ContextCanaryLeaked {
		fmt.Printf("  context canary leaked: YES — a secret that was never on disk left the agent's head\n")
	}
	fmt.Println(strings.Repeat("─", 78))
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func trunc(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}
