# Reactor — web UI

The split-panel **detonation console** for Reactor (DEMO.md §2). One page, two
columns:

- **Left — Industry standard:** a live `mcp-scan` run that says **CLEAN**.
- **Right — Reactor:** the SSE event stream — chamber header, a session ladder
  that ticks in, the MCP wire log with a **rendered byte-level diff** when a tool
  description mutates, transcript + contained-egress strips, the two red signal
  rows (`rug_pull`, then `context_exfil` — the canary that was never on disk),
  and a decisive **BLOCKED · SUPPLY-CHAIN** verdict banner.

Plus `/scorecard` — the presentation-grade metrics slide (SPEC §7): detection
rate, ~0 false-quarantine, and the headline **static-blind** comparison.

## Run it

```bash
npm install
npm run dev          # http://localhost:3000
```

Production / Vercel (`reactor.teddytennant.com`):

```bash
npm run build
npm start
# or: deploy with Root Directory = web  (see ../DEPLOY.md)
```

Set on Vercel:

| Env | Value |
|---|---|
| `NEXT_PUBLIC_ENGINE_URL` | public Go engine origin (no trailing slash) |
| `NEXT_PUBLIC_SITE_URL` | `https://reactor.teddytennant.com` |

> First `build`/`dev` fetches Inter + JetBrains Mono via `next/font/google`, then
> self-hosts them (no runtime font requests). One-time network access needed.

## Onboarding (BYOK)

On first visit the console asks for **Daytona** and **Fireworks** API keys.
They stay in `localStorage` and are attached to `POST /api/detonate` as
`credentials` (upload uses `X-Reactor-*` headers). Skip to run the bundled
replay with no keys. Re-open anytime via the gear in the top bar.

## Live engine vs. replay

The UI is **self-sufficient**. On load it probes the engine at `/api/health`:

- **Live mode** (engine reachable): `Detonate` POSTs `/api/detonate` and opens
  `EventSource` on `/api/events?detonation=<id>`. Locally, `next.config.mjs`
  rewrites `/api/*` to the Go engine (default `http://127.0.0.1:8787`). On
  Vercel, set `NEXT_PUBLIC_ENGINE_URL` so the browser talks to the engine
  directly (long SSE must not transit the edge rewrite).
- **Replay mode** (engine unreachable — the default fallback, and the DEMO §7
  backup): the bundled fixtures play through the *same* render path with
  realistic cadence (detonations ~700 ms apart; the session-4 signals land with a
  beat between them).

```bash
NEXT_PUBLIC_ENGINE_URL=http://127.0.0.1:8787 npm run dev
```

If the live stream fails before completing, the console degrades to replay
automatically — the demo never dies.

## Fixtures

`fixtures/` holds schema-exact `events.Event[]` streams and the scorecard, all
generated with **real** sha256 digests, **real** utf-8 byte lengths, and
evidence ids stamped by a faithful port of the engine's `IDGen`:

| File | What |
|---|---|
| `notes-mcp-detonation.json` | the money shot — 5 detonations, `rug_pull` (+47 bytes) then `context_exfil` (`REACTOR-a1b2`), verdict **BLOCKED / supply-chain** |
| `benign-detonation.json` | a real filesystem server, 5 clean detonations, verdict **ALLOWED** |
| `scan-clean.json` | `mcp-scan` result — CLEAN |
| `artifacts.json` | the picker zoo (`@acme/notes-mcp` is the lead) |
| `scorecard.json` | 24/25 · 0 false blocks · 9 static-blind |

Regenerate after editing `fixtures/generate.mjs`:

```bash
npm run generate:fixtures
```

The generator asserts the rug-pull delta is exactly 47 bytes and fails loudly if
the added substring drifts.

## Layout

```
app/            layout (fonts, theme), page.tsx (console), scorecard/page.tsx
components/     TopBar, ReactorMark, ThemeToggle, ui atoms
  console/      ArtifactPicker, ScanColumn, ChamberHeader, SessionLadder,
                WireLog, ByteDiff, TranscriptStrip, EgressStrip, SignalRow,
                AnalystTrace, VerdictBanner, ReactorColumn
  scorecard/    Scorecard
lib/            events.ts (typed mirror of internal/events/events.go),
                signals.ts, reducer.ts (event stream -> view state, byte diffs),
                runner.ts (live SSE + replay cadence), api.ts, fixtures.ts,
                scorecard.ts, cn.ts
fixtures/       generated Event[] streams + generate.mjs
```

## Design

- **Palette:** one calm accent (iris). **Red is reserved** for fired signals and
  the BLOCKED verdict so it carries maximum weight; green for clean / ALLOWED.
  CSS-variable tokens swap under `.dark`.
- **Type:** Inter for chrome, **JetBrains Mono for every technical stream** (wire
  log, transcript, egress, evidence ids, byte diffs) — the mono/sans contrast is
  the visual language.
- **Theme:** full light + dark, **dark by default** (security-console feel);
  respects `prefers-color-scheme` and a manual toggle, set before first paint.
- **Motion:** sessions fade/slide in as they tick; signal rows snap in with a
  red pulse; the verdict banner has a decisive entrance. Honors
  `prefers-reduced-motion`. Pure CSS — no animation dependency.

Dependencies are intentionally tight: Next.js, React, Tailwind, `lucide-react`.
