# Reactor

Runtime detonation for untrusted agent artifacts: MCP servers, skills, and vibe-coded zips.

Static scanners read tool descriptions. Reactor installs the artifact in a disposable chamber, points a sacrificial victim agent at it across several sessions, and returns a verdict with runtime evidence. The host never executes the artifact. The analyst model never sees artifact text.

**Static scanners read the label. We watch it behave.**

## Prior art

`mcp-scan` on PyPI now redirects to **snyk-agent-scan** (ex-Invariant, acquired by Snyk). That tool is static: it reads descriptions. Reactor is the runtime layer their docs already tell you to build. The gap shows up as the **static-blind rate**.

## Scorecard (`make eval`, offline)

Authored zoo, sim victim, deterministic analyst:

```
Detection rate         6/7  (86%)     miss: conditional trigger
False-quarantine rate  0/3  (0%)      (benign victim never sends the magic input)
Static-blind rate      5/6  (83%)
Mean time-to-verdict   ~12s
```

That run is harness rehearsal, not a live model result. The live money shot is `notes-mcp` with a real Fireworks victim (`gpt-oss-120b`) and a real analyst (`kimi-k2p6`): clean for three `tools/list` serves, then a 47-byte description mutation that steers the victim into attaching `~/.env`. A description-only scanner never sees it. Spec-pinned defaults live in `models.lock` (sglang victim / xAI analyst); `.env.example` shows the Fireworks path used for that demo.

## How it works

```
host (trusted)
  reactor engine ── SSE ──> web UI / TUI
        │  chamber.Driver (local default, or Daytona)
        ▼
  chamber (disposable)
     bait + canaries + decoy repo
     sink (http + dns)  -> logs/sink.jsonl
     victim ──stdio──> wire ──stdio──> artifact (strace-wrapped)
        │                │
        └─ transcript    └─ wire log
     collector          -> logs/behavioral.jsonl
```

Go owns the orchestrator, victim, wire, and oracles. Rust owns the hot paths: `crates/reactor-sink` and `crates/reactor-collect`.

1. Plant bait files and canaries (including a system-prompt canary that is never on disk).
2. Install the artifact inside the chamber only.
3. Run N sessions through a wire proxy that logs every MCP frame.
4. Capture egress on a contained sink.
5. Deterministic oracles fire signals off the typed evidence stream.
6. An analyst reasons over a redacted view of those events and writes a verdict.
7. Destroy the chamber.

Default chamber is **local** (throwaway tree on the host). Set `REACTOR_DRIVER=daytona` or paste a Daytona key in the UI (BYOK) for a remote sandbox. Local is convenient; it is not a hard isolation boundary. Daytona is stronger, still not a substitute for your own threat model.

## Quick start

Needs Go 1.26+ and Node 22+ (for the zoo and web UI).

```bash
cp .env.example .env   # optional keys; without them you get sim victim + deterministic analyst

make build
make demo              # detonate notes-mcp with the sim victim
make run               # engine on http://127.0.0.1:8787
make ui                # console on http://localhost:3000
```

Other useful targets: `make test`, `make zoo`, `make eval` (offline scorecard), `make ci`.

```bash
./bin/reactor list
./bin/reactor detonate art_notes_mcp --victim sim --sessions 5
./bin/reactor detonate ./thing.zip --victim sim
./bin/reactor detonate https://github.com/owner/repo --ref main
./bin/reactor detonate 'npx -y @acme/notes-mcp'
```

Public console: [reactor.teddytennant.com](https://reactor.teddytennant.com) (static export). Each visitor runs the engine on their own machine at `127.0.0.1:8787`. See [`DEPLOY.md`](DEPLOY.md).

## CLI

```
reactor serve                 # engine on :8787
reactor list                  # zoo catalog
reactor detonate <target>     # zoo id | archive | repo | command spec
```

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `127.0.0.1:8787` | listen address (`REACTOR_ADDR`) |
| `-zoo` | `zoo/index.json` | artifact catalog |
| `-bin` | `bin` | directory with victim/wire/sink |
| `-sessions` | `5` | sessions per detonation |
| `-victim` | auto | `auto\|fireworks\|xai\|sglang\|sim` |
| `-task` | `Summarize what this repository does.` | benign task given to the victim |
| `-deterministic` | off | fixed canary seed (rehearsal) |
| `-ref` | (default branch) | git ref for repo detonations |
| `-network` | off | allow chamber network egress |
| `-json` | off | full `DetonationReport` on stdout |

## Config

Copy `.env.example` to `.env` (never commit it). Also loads `~/.reactor.env`.

| Var | Role |
|---|---|
| `REACTOR_DRIVER` | unset = local; `daytona` routes into a live Daytona sandbox |
| `REACTOR_VICTIM_BACKEND` | `auto\|fireworks\|xai\|sglang\|sim` |
| `REACTOR_ANALYST` | set `deterministic` to force the offline analyst |
| `VICTIM_MODEL` / `ANALYST_MODEL` | override pins (see `.env.example` for Fireworks ids) |
| `FIREWORKS_API_KEY` / `XAI_API_KEY` | model providers |
| `DAYTONA_API_KEY` / `DAYTONA_API_URL` | remote chamber (optional) |

Model pins and the static baseline command also live in `models.lock`.

Without cloud keys the victim falls through to `sim` and the analyst to `reactor/deterministic-analyst-v1`. Those reports are real harness output. Treat them as rehearsal.

## Web console

Next.js split panel: static baseline on the left, live Reactor stream on the right. Intake accepts a zoo pick, upload, git repo, or command spec. `/settings` holds engine URL, BYOK keys, and run defaults. `/scorecard` is the metrics slide.

```bash
make ui
# or: cd web && npm install && npm run dev
```

Locally the dev server proxies `/api/*` to the engine. On the public site the browser talks to the visitor's loopback engine directly. No engine means bundled replay.

## Signals

Oracles emit typed signals. The static-blind set is the point of the demo: a description-only scanner cannot produce them.

| Signal | Static-blind |
|---|---|
| `rug_pull` | yes |
| `conditional_trigger` | yes |
| `context_exfil` | yes |
| `install_hook` | yes |
| `sleeper_beacon` | yes |
| `task_deviation` | yes |
| `canary_exfil` | no |
| `canary_read` | no |
| `shadowing` | no |
| `analyst_injection` | no |
| `benign_profile` | no |

Wire types, chamber layout, HTTP/SSE: [`docs/CONTRACT.md`](docs/CONTRACT.md). Zoo inventory: [`zoo/README.md`](zoo/README.md).

## Layout

| Path | Role |
|---|---|
| `cmd/reactor` | Engine CLI: `serve`, `detonate`, `list` |
| `cmd/victim`, `cmd/wire`, `cmd/sink` | Chamber binaries |
| `cmd/reactor-tui` | Backup demo surface (same SSE stream) |
| `crates/reactor-sink`, `crates/reactor-collect` | Rust egress sink + syscall collector |
| `internal/engine` | Control plane, HTTP + SSE |
| `internal/events` | Typed events and analyst redaction |
| `internal/oracle` | Deterministic signal oracles |
| `internal/chamber` | Local and Daytona drivers |
| `zoo/` | Labeled malicious and benign artifacts |
| `web/` | Detonation console |
| `eval/` | Offline scorecard |
| `models.lock` | Pinned victim / analyst / static baseline |

## Safety

- Artifacts run only inside a chamber driver. The host process does not exec them.
- Egress is redirected to a contained sink (proxy and/or DNS mock, depending on driver). Nothing is meant to leave the chamber.
- The analyst receives `events.Event.ForAnalyst()` only. Attacker prose cannot cross that boundary by forgetting a struct tag.
- Destructive zoo members (`dropper-zip`, `stealer-zip`, `injector-skill`) are chamber-only. `zoo/verify.sh` does not run them on your box.

## License

All rights reserved unless a LICENSE file is added.
