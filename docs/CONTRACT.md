# Reactor build contract

The interface every component is written against. Read this before changing wire types, chamber layout, or the engine API.

Module: `github.com/reactor-sec/reactor` · Go 1.26

## Ground rules

1. **Keep the dependency surface small.** Runtime deps are bubbletea, lipgloss, uuid, and the stdlib. JSON-RPC framing and the OpenAI client are hand-rolled (`internal/mcpjson`, `internal/oai`). Do not add a module because it is convenient; write the twenty lines.
2. **Own only your files when working in parallel.** Verify with `go build ./<your-package>/...`, not `go build ./...`.
3. **The analyst never sees artifact text.** Bytes headed at `internal/analyst` go through `events.Event.ForAnalyst()`. No exceptions.
4. **The host never executes the artifact.** Only chamber drivers spawn it.
5. **Plain Go.** No frameworks. Comments explain why. Match the tone of `internal/events/*.go`.
6. **Determinism is a feature.** No wall-clock in signal logic. No map iteration order in output. Sort before emitting.

## Already built (read these; do not casually rewrite)

| Path | What it gives you |
|---|---|
| `internal/events/events.go` | `Event` envelope + `WireEvent`, `TranscriptEvent`, `BehavioralEvent`, `Signal`, `Verdict`, `DetonationReport`, `Artifact`, `ScanResult`, `IDGen` (evidence ids like `wire:4:tools/list`, `egress:7`) |
| `internal/events/analyst_view.go` | `ForAnalyst()`: redaction boundary, allowlist by construction |
| `internal/events/bus.go` | `Bus` fan-out: `Publish`, `Subscribe(ctx, buf) (<-chan Event, replay)` |
| `internal/bait/bait.go` | `bait.New(Options) *Set`; `Set.Match`, `Tokens`, `BaitPaths`, `LabelForPath`, `Files`, `Context` (system-prompt canary), `InstallDir`, `DecoyRepo`, `SinkHost`/`SinkPort` |
| `internal/mcpjson/mcpjson.go` | newline-delimited JSON-RPC `Reader`/`WriteFrame`/`Request`/`Notification`, tool types, `ArgKeys`, `Flatten`, `ExtractPaths`, `ExtractHosts` |
| `internal/oai/oai.go` | OpenAI-compatible chat client with tool calls: `New`, `FromEnv`, `Chat`, `Ping` |
| `internal/chamber/chamber.go` | `Driver` / `Chamber` / `ExecOpts` / `Handle` |
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

- The **victim** spawns the **wire** proxy as its MCP server; the **wire** spawns the **artifact**. Every byte between agent and server is observed.
- All four write JSONL under `<chamber>/logs/`. The engine tails those files through `Chamber.Tail` and republishes them as `events.Event` on the bus.
- Log line format is one `events.Event` JSON object per line, with `id` and `seq` left empty. The engine stamps those with its `IDGen` so ids are globally ordered.

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
| `REACTOR_CANARY_CONTEXT` | system-prompt canary token (victim only) |
| `REACTOR_TASK` | the benign task string |
| `HOME` | chamber home |

Artifacts get `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY` pointed at the sink in process containment mode, and a DNS-mocked internal network in docker/Daytona mode. **Artifacts are never told where the sink is.**

## Signals (exact type strings)

From `internal/events`:

`context_exfil` · `canary_exfil` · `canary_read` · `task_deviation` · `rug_pull` ·
`conditional_trigger` · `shadowing` · `install_hook` · `analyst_injection` ·
`sleeper_beacon` · `benign_profile`

Static-blind set (`Signal.StaticBlind = true`): `rug_pull`, `conditional_trigger`,
`context_exfil`, `install_hook`, `sleeper_beacon`, `task_deviation`.

## Verification bar

- `go vet ./<pkg>/...` clean, `go test ./<pkg>/...` passing.
- Every package you own has at least one test that would fail if the behaviour regressed. Table tests preferred. No test may require a GPU or a network.

## Engine HTTP + SSE API (frozen: UI builds against this)

Base: `http://127.0.0.1:8787` (override with `-addr` / `REACTOR_ADDR`).
All JSON bodies use the types in `internal/events` unless noted.

| Method | Path | Body / Query | Returns |
|---|---|---|---|
| `GET` | `/api/health` | | `{ "ok", "drivers":[{"name","available","why"}], "analyst", "victim":{"model","served","simulated","temp","seed","revision"}, "zoo": <int> }` |
| `GET` | `/api/artifacts` | | `Artifact[]` (the zoo, for the picker) |
| `POST` | `/api/detonate` | `{ "artifact_id": "...", "sessions": 5, "network": false }` or a full `Artifact` under `"artifact"` | `{ "detonation_id": "det_..." }` |
| `GET` | `/api/detonations` | | `DetonationReport[]` (summaries, newest first) |
| `GET` | `/api/detonations/{id}` | | full `DetonationReport` |
| `GET` | `/api/events?detonation={id}` | SSE | `text/event-stream` of `Event`; replays history then live. Each SSE `data:` line is one `Event`. Event name = `Event.kind`. |
| `GET` | `/api/scan?detonation={id}` | | `ScanResult` (static baseline / left column). Missing report returns `{ "status": "unavailable" }`. |
| `GET` | `/api/scorecard` | | offline `eval/scorecard.json` if present, else a live scorecard derived from completed detonations |

CORS: `*` origin, `GET/POST/OPTIONS`, `Content-Type` allowed.

SSE framing: `event: <kind>\ndata: <Event json>\n\n`. A heartbeat `event: ping` is sent every 15s. The client keys rendering off `Event.kind` and the nested payload. `detonation` query param filters the stream; omit it for the global feed.

### Detonation phase order on the bus (what the UI animates)

```
queued → provisioning → chamber_ready → bait_planted → installing → sink_up →
  (per session: session_start → [wire/transcript/behavioral events] → session_end) →
analyzing → [analyst steps + signals] → verdict → destroying → destroyed
```

(`error` may appear on failure. `victim_ready` exists as a phase constant but is not required in the happy path above.)

Signals may arrive mid-session (deterministic oracles fire as evidence lands).
The `verdict` event carries the final `Verdict`.

### CLI surface (same engine)

```
reactor serve
reactor list
reactor detonate <artifact_id> [-sessions N] [-victim auto|fireworks|xai|sglang|sim] [-json]
```

Flags and env defaults live in `cmd/reactor/main.go`. Chamber binaries are resolved from `-bin` (`REACTOR_BIN_DIR`, default `bin`).
