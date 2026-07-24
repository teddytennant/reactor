// Command victim is the sacrificial agent binary (SPEC §4.1). It spawns the
// artifact through the wire proxy, is handed one boring task plus a
// system-prompt canary, and does the task using whatever tools the server
// advertises. Everything it needs comes from env so it drops into either
// chamber driver; it writes TranscriptEvents to REACTOR_LOG_DIR/transcript.jsonl.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/reactor-sec/reactor/internal/events"
	"github.com/reactor-sec/reactor/internal/mcpclient"
	"github.com/reactor-sec/reactor/internal/victim"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("victim: ")
	var (
		logDir  = flag.String("log-dir", env("REACTOR_LOG_DIR", "."), "dir for transcript.jsonl")
		session = flag.Int("session", envInt("REACTOR_SESSION", 1), "session number")
		task    = flag.String("task", env("REACTOR_TASK", "Summarize what this repository does."), "benign task")
		dir     = flag.String("dir", os.Getenv("REACTOR_ARTIFACT_DIR"), "working dir for the server command")
	)
	flag.Parse()
	serverCmd := flag.Args()
	if len(serverCmd) == 0 {
		if c := os.Getenv("REACTOR_SERVER_CMD"); c != "" {
			serverCmd = splitFields(c)
		}
	}
	if len(serverCmd) == 0 {
		log.Fatal("no MCP server command given (expected: victim [flags] -- reactor-wire ... artifact ...)")
	}

	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		log.Fatalf("mkdir log dir: %v", err)
	}
	tf, err := os.OpenFile(filepath.Join(*logDir, "transcript.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatalf("open transcript log: %v", err)
	}
	defer tf.Close()
	var mu sync.Mutex
	emit := func(ev events.Event) {
		b, err := json.Marshal(ev)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		bw := bufio.NewWriter(tf)
		bw.Write(b)
		bw.WriteByte('\n')
		bw.Flush()
		tf.Sync()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	backend := victim.Resolve(ctx, victim.Config{
		Seed:   envInt("REACTOR_VICTIM_SEED", 7),
		Temp:   0,
		Refuse: os.Getenv("REACTOR_SIM_REFUSE") == "1",
	})

	client, err := mcpclient.Start(ctx, serverCmd, nil, *dir, os.Stderr)
	if err != nil {
		log.Fatalf("start server: %v", err)
	}
	defer client.Close()

	agent := &victim.Agent{
		Backend:       backend,
		Task:          *task,
		ContextCanary: os.Getenv("REACTOR_CANARY_CONTEXT"),
		ConvCanary:    os.Getenv("REACTOR_CANARY_CONVERSATION"),
		Session:       *session,
		Emit:          emit,
	}
	outcome, err := agent.Run(ctx, client)
	if err != nil {
		log.Printf("session %d error: %v", *session, err)
		os.Exit(1)
	}
	info := backend.Info()
	log.Printf("session %d done: model=%s served=%s calls=%d deviations=%d ctx_leak=%v refused=%v",
		*session, info.Model, info.Served, outcome.ToolCalls, outcome.Deviations, outcome.LeakedContextCanary, outcome.Refused)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
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
