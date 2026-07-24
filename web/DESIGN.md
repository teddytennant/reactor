# Reactor — design language

The bar: Linear, Vercel, Raycast, Claude. Dark-native, dense, quiet until something
matters. **This is not a chat app and must not look like one.** It is a live
containment instrument — an oscilloscope for untrusted agents. Every design
decision serves one narrative: *the left column reads a label; the right column
watches a thing behave, and catches it.*

Non-negotiable: existing behavior, props, data flow, and reducer/runner logic stay
exactly as-is. This is a **visual** rebuild. Do not change `lib/`.

---

## 1. Foundations

### Palette — dark is the design, light is the courtesy

Base is a cold, near-black graphite with a blue cast. Elevation comes from
**lightness steps + hairlines + a 1px inner top highlight** — never from big drop
shadows. Dark mode must feel *lit from within*, not painted grey.

Roles:

| Role | Meaning | Notes |
|---|---|---|
| `accent` | Reactor itself; interactive; the right column | luminous plasma indigo-violet, high chroma, must glow convincingly on near-black |
| `live` (new) | streaming / instrument / in-flight telemetry | cool cyan-teal, used **sparsely**: live dots, sweep, active session tick |
| `success` | mcp-scan CLEAN, ALLOWED verdict | green |
| `danger` | fired signals + BLOCKED verdict | saturated crimson — **reserved**, never decorative |
| `warning` | sub-critical signals | amber |

Rules:
- Red appears **only** on fired signals and BLOCKED. If red shows up anywhere
  else, delete it.
- `live` cyan and `success` green must be far enough apart in hue to never be
  confused at dot size. Verify.
- Every color stays a CSS variable in `globals.css` (space-separated RGB) so
  Tailwind opacity utilities keep working. Add `--live` alongside the rest.

### Elevation

Four surfaces (`bg` → `surface` → `surface-2` → `surface-3`), each a small,
even lightness step. Panels get:
- a hairline border (`--line`),
- a `1px` inner top highlight (`inset 0 1px 0 rgb(255 255 255 / 0.04)` in dark),
- shadow only for genuinely floating things (verdict banner, popovers).

### Texture

Two signature ambient layers, both extremely subtle — if they're obvious, they're
too strong:
1. **Core bloom** — a radial gradient behind the Reactor column. It is
   *state-reactive*: idle = faint accent, detonating = brighter + slowly
   breathing, BLOCKED = shifts to danger, ALLOWED = success. This is the single
   most identity-defining element on the page. Drive it from a data attribute or
   class, pure CSS animation.
2. **Instrument grid** — a ~1px dot or line grid at ~2–3% opacity inside chamber
   panels. Reads as graph paper under glass.

### Type

Keep Inter + JetBrains Mono; change how they're used.
- **Mono is the voice of the machine.** Every section label, status, counter,
  evidence id, timestamp, byte count, tool name, verdict: mono, uppercase where
  it's a label, `0.1–0.16em` tracking, tiny (11px), `--faint`.
- Sans is for prose only: headings, glosses, analyst thoughts.
- Tabular figures (`tnum`) on every number that can change.
- Tighten display headings (`tracking-tight`), never below 12px for body text.

### Motion

Purposeful, fast, physical. `cubic-bezier(0.16,1,0.3,1)`, 200–450ms. Sessions
tick in; signals snap with a red bloom; the verdict lands with weight. Live
elements breathe slowly. Nothing bounces, nothing spins for decoration. All of it
must vanish under `prefers-reduced-motion` (already wired — keep it).

---

## 2. Signature elements (the bespoke bits)

These are what make it Reactor and not a generic dashboard. Use them
consistently; don't invent a sixth.

1. **Containment bezel** — the Reactor column wears a frame the scan column
   doesn't: an inset ring plus **corner tick marks** (short 1px L-brackets at the
   four corners, accent at ~35% opacity). Blast-door energy. Ticks go danger-red
   when the verdict blocks.
2. **Status LEDs** — never a flat `rounded-full`. A lit dot has a real bloom:
   small solid core + `box-shadow` halo in its own hue. Live ones pulse slowly.
3. **Gutter rail** — dense technical streams (wire log, session ladder, analyst
   trace) get a left gutter: a hairline vertical rail with index/time in mono,
   and a node marker where a row matters. Reads as a timeline, not a list.
4. **Hairline-only separation** — telemetry rows are separated by hairlines and
   density, never by cards-inside-cards or zebra striping. Kill nested card
   borders wherever they exist today.
5. **Scan-line sweep** — the single "in flight" affordance: a thin luminous line
   traveling the top edge of an active panel. Used for the running chamber and
   the active mcp-scan. One at a time.

---

## 3. Layout & density

- Console fills the viewport; the two columns scroll independently, headers
  sticky within their column. No page-level double scrollbar.
- The left (scan) column is deliberately **quieter and flatter** — lower
  contrast, no bezel, no bloom. The asymmetry *is* the argument.
- Tighten padding one step across the board (this currently reads airy and
  generic). Dense but never cramped: 12–16px panel padding, 6–8px row rhythm.
- Responsive: below `lg` the columns stack, scan first. Nothing may overflow
  horizontally; long mono strings wrap or scroll in their own container.

## 4. Accessibility

- Body text ≥ 4.5:1 on its surface; labels/faint text ≥ 3:1. Check the faint tier
  in **both** themes — that's where these designs usually fail.
- Never encode state in color alone: pair every colored state with an icon,
  label, or shape.
- Visible focus ring on every interactive element (`ring-2 ring-accent/60
  ring-offset-2 ring-offset-bg`), and real `:focus-visible` styling on the
  buttons in the top bar and picker.
- Keep the light theme genuinely good, not an afterthought. Same structure,
  ink-on-paper instrument feel.

## 5. Guardrails

- Tailwind + CSS only. **No new dependencies**, no animation libraries.
- Use semantic tokens (`bg-surface-2`, `text-faint`) — no raw hex in components,
  no `gray-800`.
- Keep all existing component names, exports, props, and file paths.
- Preserve every `aria-*`, `title`, and `key`. Preserve fixture-driven text.
- It must compile: `npx tsc --noEmit` and `npm run build` both clean.
