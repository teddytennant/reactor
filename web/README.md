# Reactor web UI

Split-panel detonation console.

- **Left:** static baseline (`mcp-scan` / snyk-agent-scan) that says CLEAN on the money shot.
- **Right:** live SSE stream from the engine: chamber header, session ladder, wire log with byte-level diffs on description mutation, transcript and contained egress, signal rows, verdict.
- **Intake:** zoo pick, archive upload, git repo, or command spec.
- **`/settings`:** engine URL, BYOK keys (Daytona + Fireworks), run defaults.
- **`/scorecard`:** detection / false-quarantine / static-blind slide.

## Run

```bash
# from repo root (preferred)
make ui

# or
npm install
npm run dev          # http://localhost:3000
```

Needs the engine on `http://127.0.0.1:8787` (`make run` from the repo root). Dev rewrites `/api/*` there. If the engine is down, the UI falls back to bundled replay.

## Production

Public site: [reactor.teddytennant.com](https://reactor.teddytennant.com). Static export only. Each visitor runs the Go engine on their own machine; the page talks to loopback. Full deploy notes: [`../DEPLOY.md`](../DEPLOY.md).

Optional build-time overrides:

| Env | When |
|---|---|
| `NEXT_PUBLIC_ENGINE_URL` | Shared engine origin (no trailing slash). Visitors can still override in Settings. |
| `NEXT_PUBLIC_SITE_URL` | Site moves off `reactor.teddytennant.com`. |

```bash
npm run build
# Vercel: root vercel.json publishes web/out
```

## Onboarding (BYOK)

First visit asks for Daytona and Fireworks keys plus the engine URL. Values stay in `localStorage` and ride on `POST /api/detonate` as `credentials` (uploads use `X-Reactor-*` headers). Skip to run the bundled replay with no keys. Edit anytime on `/settings`.

## Live vs replay

On load the console probes `/api/health`:

- **Live:** `Detonate` posts `/api/detonate` and opens `EventSource` on `/api/events?detonation=<id>`.
- **Replay:** fixtures play through the same render path. Used when the engine is unreachable (including Safari/iOS, which block https → loopback).

If a live stream dies mid-run, the console degrades to replay.

## Fixtures

`fixtures/` holds schema-exact `events.Event[]` streams generated with real digests and evidence ids:

| File | What |
|---|---|
| `notes-mcp-detonation.json` | money shot: `rug_pull` (+47 bytes) then `context_exfil`, BLOCKED |
| `benign-detonation.json` | clean filesystem server, ALLOWED |
| `scan-clean.json` | static baseline CLEAN |
| `artifacts.json` | zoo picker |
| `scorecard.json` | offline metrics slide |

```bash
npm run generate:fixtures
```

## Layout

```
app/            page (console), settings/, scorecard/
components/     TopBar, OnboardingModal, console/*, scorecard/*
lib/            events, signals, reducer, runner, api, fixtures, scorecard
fixtures/       generated streams + generate.mjs
```

## Design

Iris accent. Red only for fired signals and BLOCKED. Inter for chrome, JetBrains Mono for every technical stream. Dark by default, light available. Motion is CSS-only and respects `prefers-reduced-motion`.

Stack: Next.js, React, Tailwind, `lucide-react`.
