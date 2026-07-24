// Command wire is the transparent MCP proxy binary (SPEC §12.2). The victim
// launches it as its MCP "server"; wire launches the real artifact and proxies
// stdio between them, logging every frame to REACTOR_LOG_DIR/wire.jsonl.
//
//	victim ──stdio──> reactor-wire ──stdio──> artifact
//
// Everything wire needs comes from flags/env so it drops into either chamber
// driver unchanged.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/reactor-sec/reactor/internal/canary"
	"github.com/reactor-sec/reactor/internal/events"
	"github.com/reactor-sec/reactor/internal/procenv"
	"github.com/reactor-sec/reactor/internal/wire"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("wire: ")
	var (
		logDir     = flag.String("log-dir", env("REACTOR_LOG_DIR", "."), "dir for wire.jsonl")
		canaryFile = flag.String("canaries", os.Getenv("REACTOR_CANARY_FILE"), "canary table json")
		session    = flag.Int("session", envInt("REACTOR_SESSION", 1), "session number")
		dir        = flag.String("dir", os.Getenv("REACTOR_ARTIFACT_DIR"), "artifact working dir")
	)
	flag.Parse()
	artifactArgv := flag.Args()
	if len(artifactArgv) == 0 {
		if cmd := os.Getenv("REACTOR_ARTIFACT_CMD"); cmd != "" {
			artifactArgv = splitFields(cmd)
		}
	}
	if len(artifactArgv) == 0 {
		log.Fatal("no artifact command given (positional args after -- or REACTOR_ARTIFACT_CMD)")
	}

	cset, err := canary.Load(*canaryFile)
	if err != nil {
		log.Fatalf("load canaries: %v", err)
	}

	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		log.Fatalf("mkdir log dir: %v", err)
	}
	lf, err := os.OpenFile(filepath.Join(*logDir, "wire.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatalf("open wire log: %v", err)
	}
	defer lf.Close()
	var mu sync.Mutex
	emit := func(ev events.Event) {
		b, err := json.Marshal(ev)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		bw := bufio.NewWriter(lf)
		bw.Write(b)
		bw.WriteByte('\n')
		bw.Flush()
		lf.Sync()
	}

	// Spawn the artifact.
	cmd := exec.Command(artifactArgv[0], artifactArgv[1:]...)
	if *dir != "" {
		cmd.Dir = *dir
	}
	// The artifact is untrusted: strip host creds, and point its egress at the
	// contained sink (REACTOR_ARTIFACT_PROXY), so any beacon to a real host is
	// captured instead of forwarded. The victim never carries these.
	artEnv := procenv.Sanitize(os.Environ())
	if proxy := os.Getenv("REACTOR_ARTIFACT_PROXY"); proxy != "" {
		for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
			artEnv = append(artEnv, k+"="+proxy)
		}
	}
	cmd.Env = artEnv
	cmd.Stderr = os.Stderr // artifact stderr → wire stderr → chamber log
	artIn, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	artOut, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatalf("start artifact: %v", err)
	}

	p := wire.New(*session, cset, emit)
	// victim(os.Stdin) → artifact(artIn); artifact(artOut) → victim(os.Stdout)
	p.Pump(os.Stdin, artIn, artOut, os.Stdout)

	artIn.Close()
	cmd.Wait()
	_ = io.Discard
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

// splitFields splits a command string on spaces, honouring simple double quotes.
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
