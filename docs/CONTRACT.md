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
| `POST` | `/api/upload` | `multipart/form-data`, one file part (any field name) + optional `kind`/`name`/`source`/`install` text fields | `UploadResponse` (see below) |
| `POST` | `/api/detonate` | `{ "artifact_id": "...", "sessions": 5, "network": false }`, or a full `Artifact` under `"artifact"`, or `{ "upload_id": "up_..." }`, or `{ "repo": "https://...", "ref": "main" }` | `{ "detonation_id": "det_..." }` |
| `GET` | `/api/detonations` | | `DetonationReport[]` (summaries, newest first) |
| `GET` | `/api/detonations/{id}` | | full `DetonationReport` |
| `GET` | `/api/events?detonation={id}` | SSE | `text/event-stream` of `Event`; replays history then live. Each SSE `data:` line is one `Event`. Event name = `Event.kind`. |
| `GET` | `/api/scan?detonation={id}` | | `ScanResult` (static baseline / left column). Missing report returns `{ "status": "unavailable" }`. |
| `GET` | `/api/scorecard` | | offline `eval/scorecard.json` if present, else a live scorecard derived from completed detonations |

CORS: `*` origin, `GET/POST/OPTIONS`, `Content-Type` allowed.

Errors are `text/plain` on every endpoint (`http.Error`), with a message meant to be shown to a person. Statuses: `400` malformed request or a refused archive/url, `404` unknown detonation or expired upload, `405` wrong method, `413` over a size ceiling, `415` unsupported archive type, `504` clone timeout, `500` engine-side failure.

### Artifact ingest (`/api/upload` + the repo path)

Two ways to detonate something that is not in the zoo. Both stage into a
per-detonation directory that is deleted when the detonation ends; neither ever
puts a host path in a response.

`POST /api/upload` — one zip/tar/tar.gz, streamed to disk and sha256'd as it
streams. The archive type is decided by content, not by filename or
`Content-Type`. Response:

```json
{
  "upload_id": "up_9bae92927ce6",
  "name": "notes-mcp.zip", "kind": "mcp_server", "archive": "zip",
  "sha256": "<64 hex, of exactly the bytes received>",
  "size_bytes": 754, "files": 3, "unpacked_bytes": 78, "skipped_entries": 0,
  "source": "node server.mjs", "install": "npm install",
  "expires_ms": 1784925698559,
  "artifact": { "id": "art_...", "kind": "...", "name": "...", "source": "...", "sha256": "...", "note": "...", "env": {"_install": "..."} }
}
```

`kind`/`source`/`install` are inferred from the entry names (`server.mjs` ⇒
`mcp_server`, `SKILL.md` ⇒ `skill`, `package.json` ⇒ `npm install`, otherwise
`zip`); the form fields and the `artifact` override on detonate both beat them.
`skipped_entries` counts symlinks, hardlinks and device nodes that were refused
rather than unpacked.

Then `POST /api/detonate` with `{"upload_id": "up_...", "sessions": 5}`, or
`{"repo": "https://github.com/owner/repo", "ref": "main"}`. An optional
`"artifact"` alongside either refines `name`, `kind`, `source`, `args`, `note`
and `env._install` — `env._dir` is set by the engine and is never taken from the
client. An upload can be detonated more than once; it is swept after its TTL.

Ceilings (all `engine.Config` fields, all overridable):

| Field | Default | Meaning |
|---|---|---|
| `MaxUploadBytes` | 64 MiB | one uploaded file → `413` |
| `MaxExtractBytes` | 256 MiB | what an archive or clone may become on disk → `413` |
| `MaxExtractFiles` | 20000 | file count in an archive or clone → `413` |
| `MaxCloneBytes` | 256 MiB | a clone is killed mid-flight past this → `413` |
| `CloneTimeout` | 90s | one `git clone` → `504` |
| `UploadTTL` | 2h | unclaimed uploads are swept |
| `WorkDir` | `$TMPDIR/reactor` (`REACTOR_WORK_DIR`) | staging root, removed by `Engine.Close` |
| `AllowLocalRepos` | false (`REACTOR_ALLOW_LOCAL_REPOS=1`) | dev only: permits `file://` and private hosts |

Repo urls are `https` only — no `ssh`/`git@`/`git://`/`file://`, no credentials
in the url, no loopback/private/link-local IP literals. Refs must look like a
branch, tag or sha. Clones are `--depth 1 --single-branch --no-tags`, with
hooks, templates, credential helpers, submodules and every non-https transport
disabled, and `.git` is removed before the tree reaches a chamber.

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
