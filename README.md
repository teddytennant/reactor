# Reactor

Detonation chamber for untrusted agent artifacts: MCP servers, agent skills, and vibe-coded zips.

Static scanners read tool descriptions. Reactor installs the artifact in a disposable chamber, points a sacrificial victim agent at it, watches what happens across several sessions, and returns a verdict backed by runtime evidence. The host never executes the artifact. The analyst model never sees artifact text.

**Static scanners read the label. We watch it behave.**

## Prior art, verified (2026-07-24)

`mcp-scan` on PyPI now redirects to **`snyk-agent-scan`** ("Agent supply chain
security scanner") — the ex-Invariant Labs tool Snyk acquired. Someone *does*
scan MCP servers; we cite them by name. They are static (they read tool
descriptions); Reactor is the runtime layer their own docs tell you to build. The
gap, as a number, is the **static-blind rate**.

## Measured (not claimed) — `make eval`

Offline scorecard over the authored zoo (sim victim + deterministic analyst):

```
  Detection rate         6/7  (86%)     one honest miss: the conditional trigger,
  False-quarantine rate  0/3  (0%)      whose magic input a benign victim never
  Static-blind rate      5/6  (83%)     sends (needs the §4.5 varied-input loop).
  Mean time-to-verdict   ~12s
```

The money shot runs with a **real** Fireworks `gpt-oss-120b` victim (it attaches
`~/.env` *and* leaks the system-prompt canary on session 4) and **real** Kimi K2.6
writing an evidence-cited verdict — not a simulation.

## How it works

```
host (trusted)
  reactor engine ── SSE ──> web UI
        │  chamber.Driver (local or Daytona)
        ▼
  chamber (disposable)
     bait creds + decoy repo + canaries
     reactor-sink (Rust/axum)   127.0.0.1  http + dns   -> logs/sink.jsonl
     victim  ──stdio──> wire ──stdio──> artifact
        │                 │                 (strace-wrapped)
        └─ transcript     └─ wire log
     reactor-collect (Rust)   strace parse  -> logs/behavioral.jsonl
```

**Rust where it pays (SPEC §12.2):** `crates/reactor-sink` (axum egress sink on
the hot path, parsing untrusted bodies — the spec-canonical component) and
`crates/reactor-collect` (the syscall collector: a hot loop over megabytes of
adversarially-shaped strace output, where memory safety is the point). Go owns
the orchestrator, victim, wire proxy and oracles.

1. Plant bait files and canary tokens (including a system-prompt canary that is never on disk).
2. Install the artifact inside the chamber only.
3. Run N sessions: a victim agent talks to the artifact through a wire proxy that logs every MCP frame.
4. Capture egress on a contained sink. Nothing leaves the chamber.
5. Deterministic oracles fire signals off the typed evidence stream.
6. An analyst reasons over a redacted view of those events and writes a verdict.
7. Destroy the chamber.

The money shot is `notes-mcp`: clean for three `tools/list` serves, then a 47-byte description mutation that steers the victim into attaching `~/.env`. A description-only scanner never sees it.

## Repo layout

| Path | Role |
|---|---|
| `cmd/reactor` | Engine CLI: `serve`, `detonate`, `list` |
| `cmd/victim`, `cmd/wire`, `cmd/sink` | Chamber binaries the engine shells into the sandbox |
| `cmd/reactor-tui` | Backup demo surface (bubbletea): same SSE stream, two columns |
| `cmd/mutate` | Offline red-team generator → escape rate (`eval/redteam.json`) |
| `crates/reactor-sink`, `crates/reactor-collect` | Rust: egress sink + syscall collector |
| `internal/engine` | Control plane, HTTP + SSE API |
| `internal/events` | Typed event contract and analyst redaction boundary |
| `internal/oracle` | Deterministic signal oracles |
| `internal/chamber` | Local and Daytona drivers |
| `zoo/` | Malicious and benign artifacts with ground-truth labels |
| `web/` | Detonation console (Next.js). Live SSE or bundled replay |
| `docs/CONTRACT.md` | Interfaces every component is written against |
| `models.lock` | Pinned victim / analyst / static-baseline pins |

## Quick start

Needs Go 1.26+, Node 20+ for the zoo and web UI.

```bash
# 1. env
cp .env.example .env
# optional: FIREWORKS_API_KEY / XAI_API_KEY / DAYTONA_API_KEY
# without keys the engine falls back to the sim victim + deterministic analyst

# 2. chamber binaries
mkdir -p bin
go build -o bin/reactor ./cmd/reactor
go build -o bin/victim  ./cmd/victim
go build -o bin/wire    ./cmd/wire
go build -o bin/sink    ./cmd/sink

# 3. one-shot detonation (sim victim, no GPU)
./bin/reactor detonate art_notes_mcp --victim sim --sessions 5

# 4. engine API (what the UI talks to)
./bin/reactor serve
# http://127.0.0.1:8787
```

Web console:

```bash
cd web && npm install && npm run dev
# http://localhost:3000
# proxies /api/* to NEXT_PUBLIC_ENGINE_URL (default http://127.0.0.1:8787)
# if the engine is down, the UI falls back to fixture replay
```

List the zoo:

```bash
./bin/reactor list
```

Verify zoo behavior without the engine:

```bash
bash zoo/verify.sh
```

## CLI

```
reactor serve                     # engine on :8787
reactor list                      # zoo catalog
reactor detonate <artifact_id>    # one run, print verdict
```

Useful flags (order does not matter):

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `127.0.0.1:8787` | listen address (`REACTOR_ADDR`) |
| `-zoo` | `zoo/index.json` | artifact catalog |
| `-bin` | `bin` | directory with victim/wire/sink |
| `-sessions` | `5` | sessions per detonation |
| `-victim` | auto | `auto\|fireworks\|xai\|sglang\|sim` |
| `-task` | summarize-repo prompt | benign task given to the victim |
| `-deterministic` | off | fixed canary seed (rehearsal) |
| `-json` | off | full `DetonationReport` on stdout |

## Config

Copy `.env.example` to `.env` (never commit `.env`):

```
DAYTONA_API_KEY=
DAYTONA_API_URL=
FIREWORKS_API_KEY=
ANALYST_MODEL=
VICTIM_MODEL=
REACTOR_VICTIM_BACKEND=
```

Also loads `~/.reactor.env`. Model pins and the static baseline command live in `models.lock`.

Without cloud keys:
- victim backend falls through to `sim` (`reactor/sim-victim-v1`)
- analyst falls through to `reactor/deterministic-analyst-v1`

Reports produced that way are real harness output. Treat them as rehearsal, not a scored result.

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

Full wire types, chamber layout, and the HTTP/SSE surface: [`docs/CONTRACT.md`](docs/CONTRACT.md). Zoo inventory and how each artifact fires: [`zoo/README.md`](zoo/README.md).

## Safety

- Artifacts run only inside a chamber driver. The host process does not exec them.
- Egress is sink-captured. Artifacts are never told the sink address.
- The analyst receives `events.Event.ForAnalyst()` only. Attacker prose cannot cross that boundary by forgetting a struct tag.
- Destructive zoo members (`dropper-zip`, `stealer-zip`, `injector-skill`) are chamber-only. `zoo/verify.sh` does not run them on your box.

## License

All rights reserved unless a LICENSE file is added.
