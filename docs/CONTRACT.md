# Reactor build contract

Read this before touching code. It is the interface every component is written
against. `SPEC.md` says *what*; this says *exactly how the pieces meet*.

Module: `github.com/reactor-sec/reactor` · Go 1.26 · repo root `/home/teddy/projects/reactor`.

## Ground rules

1. **Do not edit `go.mod` / `go.sum`.** All dependencies are already present:
   `charmbracelet/bubbletea`, `charmbracelet/lipgloss`, `google/uuid`, stdlib.
   Everything else is hand-rolled on purpose (see `internal/oai`, `internal/mcpjson`).
   If you think you need a dependency, you don't — write it.
2. **Own only your files.** Other agents are editing sibling packages in the
   same checkout at the same time. Never edit a file outside your assignment.
   Verify with `go build ./<your-package>/...`, not `go build ./...`.
3. **The analyst never sees artifact text.** If you are handing bytes toward
   `internal/analyst`, they go through `events.Event.ForAnalyst()`. No exceptions.
4. **The host never executes the artifact.** Only chamber drivers spawn it.
5. Code style: plain Go, no frameworks, comments explain *why*. Match the tone
   of `internal/events/*.go` — short doc comments that carry the argument.
6. Determinism is a feature: no wall-clock in signal logic, no map iteration
   order in output, sort before emitting.

## Already built (read these; do not modify)

| Path | What it gives you |
|---|---|
| `internal/events/events.go` | `Event` envelope + `WireEvent`, `TranscriptEvent`, `BehavioralEvent`, `Signal`, `Verdict`, `DetonationReport`, `Artifact`, `ScanResult`, `IDGen` (evidence ids like `wire:4:tools/list`, `egress:7`) |
| `internal/events/analyst_view.go` | `ForAnalyst()` — the redaction boundary, allowlist by construction |
| `internal/events/bus.go` | `Bus` fan-out: `Publish`, `Subscribe(ctx, buf) (<-chan Event, replay)` |
| `internal/bait/bait.go` | `bait.New(Options) *Set`; `Set.Match(blob) (tokens, kinds)`, `Tokens()`, `BaitPaths()`, `LabelForPath()`, `Files`, `Context` (the system-prompt canary), `InstallDir`, `DecoyRepo`, `SinkHost/SinkPort` |
| `internal/mcpjson/mcpjson.go` | newline-delimited JSON-RPC `Reader`/`WriteFrame`/`Request`/`Notification`, `Tool`, `ToolsListResult`, `CallParams`, `CallResult`, plus `ArgKeys`, `Flatten`, `ExtractPaths`, `ExtractHosts` |
| `internal/oai/oai.go` | OpenAI-compatible chat client with tool calls: `New(baseURL, key, model)`, `FromEnv()`, `Chat(ctx, Request)`, `Ping`, `F()`, `I()` |
| `internal/chamber/chamber.go` | `Driver` / `Chamber` / `ExecOpts` / `Handle` interfaces |
| `models.lock` | pinned model ids, sim backend, static baseline command |

## Runtime topology

```
host (trusted)
  reactor engine ── SSE ──> web UI / TUI
        │  chamber.Driver
        ▼
  chamber (disposable)
     reactor-sink        127.0.0.1:9931 http + :9953 dns   -> logs/sink.jsonl
     reactor-victim  ──stdio──> reactor-wire ──stdio──> artifact process
        │                           │                       (strace-wrapped)
        └─> logs/transcript.jsonl   └─> logs/wire.jsonl      -> logs/strace.log
```

- The **victim** spawns the **wire** proxy as its MCP server; the **wire** proxy
  spawns the **artifact**. So every byte between agent and server is observed.
- All four write JSONL to `<chamber>/logs/*.jsonl`. The engine tails those files
  through `Chamber.Tail` and republishes them as `events.Event` on the bus.
- Log line format is **one `events.Event` JSON object per line**, with `id` and
  `seq` left empty — the engine stamps those with its `IDGen` so ids are
  globally ordered.

## Chamber layout (identical in both drivers)

```
$HOME              = /home/agent            (chamber-side)
$HOME/artifact     = install dir            (the only place writes are benign)
$HOME/work/acme-notes = decoy repo
$HOME/logs         = *.jsonl collectors write here
$HOME/.aws/credentials, ~/.ssh/id_rsa, ~/.env, ~/.config/gh/hosts.yml,
$HOME/wallet.dat, ~/.mozilla/firefox/default/cookies.sqlite   = bait
$HOME/.config/mcp/servers.json  = decoy second MCP config (shadowing bait)
$HOME/.cache/agent/history.jsonl = decoy prior conversation
```

## Environment variables passed into chamber processes

| Var | Meaning |
|---|---|
| `REACTOR_LOG_DIR` | where to append JSONL |
| `REACTOR_SESSION` | 1-based session number |
| `REACTOR_DETONATION` | detonation id |
| `REACTOR_SINK_HTTP` | `http://127.0.0.1:9931` |
| `REACTOR_VICTIM_BASE` | OpenAI-compatible base url for the victim model |
| `REACTOR_VICTIM_MODEL` | model id |
| `REACTOR_CANARY_CONTEXT` | the system-prompt canary token (victim only) |
| `REACTOR_TASK` | the benign task string |
| `HOME` | chamber home |

Artifacts get `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY` pointed at the sink in
process containment mode, and a DNS-mocked internal network in docker mode.
**Artifacts are never told where the sink is.**

## Signals (SPEC §4.4) — exact type strings

`context_exfil` · `canary_exfil` · `canary_read` · `task_deviation` · `rug_pull` ·
`conditional_trigger` · `shadowing` · `install_hook` · `analyst_injection` ·
`sleeper_beacon` · `benign_profile`

Static-blind set (`Signal.StaticBlind = true`): `rug_pull`, `conditional_trigger`,
`context_exfil`, `install_hook`, `sleeper_beacon`, `task_deviation`.

## Verification bar

- `go vet ./<pkg>/...` clean, `go test ./<pkg>/...` passing.
- Every package you own has at least one test that would fail if the behaviour
  regressed. Table tests preferred. No test may require a GPU or a network.

## Engine HTTP + SSE API (frozen — UI/TUI build against this)

Base: `http://127.0.0.1:8787`. All JSON is the types in `internal/events`.

| Method | Path | Body / Query | Returns |
|---|---|---|---|
| `GET` | `/api/health` | — | `{ "ok": true, "drivers": [{"name","available","why"}], "analyst": "grok-4.5"\|"deterministic" }` |
| `GET` | `/api/artifacts` | — | `Artifact[]` (the zoo, for the picker) |
| `POST` | `/api/detonate` | `{ "artifact_id": "...", "sessions": 5, "network": false }` or a full `Artifact` under `"artifact"` | `{ "detonation_id": "det_..." }` |
| `GET` | `/api/detonations` | — | `DetonationReport[]` (summaries, newest first) |
| `GET` | `/api/detonations/{id}` | — | full `DetonationReport` |
| `GET` | `/api/events?detonation={id}` | SSE | `text/event-stream` of `Event`; replays history then live. Each SSE `data:` line is one `Event`. Event name = `Event.kind`. |
| `GET` | `/api/scan?detonation={id}` | — | `ScanResult` (static baseline / left column) |
| `GET` | `/api/scorecard` | — | scorecard JSON (see `eval/` output schema) |

SSE framing: `event: <kind>\ndata: <Event json>\n\n`. A heartbeat `event: ping`
is sent every 15s. The client keys rendering off `Event.kind` and the nested
payload. `detonation` query param filters the stream; omit it for the global feed.

### Detonation phase order on the bus (what the UI animates)

`queued → provisioning → chamber_ready → bait_planted → sink_up → installing →`
`(per session: session_start → [wire/transcript/behavioral events] → session_end) →`
`analyzing → [analyst steps + signals] → verdict → destroying → destroyed`

Signals may arrive mid-session (deterministic oracles fire as evidence lands).
The `verdict` Event carries the final `Verdict`.
