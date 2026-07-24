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

### Palette — warm neutral dark, calm and clean

**Reference standard: the Claude/Anthropic desktop aesthetic.** Warm neutral
near-black (hue ~30–40, brown-grey — *not* blue), soft rounded panels, borders so
faint they read as edges rather than lines, muted warm-grey type hierarchy, and
**one restrained accent** spent sparingly. Calm and flat with soft elevation. No
neon, no glow, no ornament.

Roles:

| Role | Meaning | Notes |
|---|---|---|
| `accent` | Reactor itself; interactive; primary buttons | a **restrained cool blue** (~`#3B82F6`), not violet, not neon. Cool blue on warm base is the only chromatic tension in the design — that's why it works. Solid fills, no glow. |
| `live` | streaming / in-flight telemetry | a **muted** desaturated blue-teal. Used very sparingly — live dots and the active tick, nothing else. Must never out-shout `accent`. |
| `success` | mcp-scan CLEAN, ALLOWED verdict, added-lines | soft green, reference-grade (`#4ADE80` family), not fluorescent |
| `danger` | fired signals + BLOCKED verdict | soft red (`#F87171` family) — **reserved**, never decorative |
| `warning` | sub-critical signals | muted amber |

Rules:
- Warm base, cool accent. Every neutral (bg, surfaces, lines, text) carries a
  warm cast; only the semantic colors are chromatic.
- Text is **warm off-white, never pure white** (`#E8E5E1` family), stepping down
  through muted (`#918B84`) to faint (`#6B655F`).
- Red appears **only** on fired signals, removed bytes, and BLOCKED. If red shows
  up anywhere else, delete it.
- Accent is for *one* primary action per view plus selection state. If the page
  has three blue things competing, two of them are wrong.
- Every color stays a CSS variable in `globals.css` (space-separated RGB) so
  Tailwind opacity utilities keep working.

### Elevation

Four surfaces (`bg` → `surface` → `surface-2` → `surface-3`), each a small, even
lightness step, all warm-neutral. Panels are **defined by their fill, not their
outline**:
- borders at ~`rgb(255 255 255 / 0.06)` in dark — perceived as an edge, not a
  drawn line. If you can clearly see the border, it's too strong.
- **generous radii**: panels `rounded-2xl` (16px), rows/controls `rounded-xl`
  (10–12px), chips `rounded-lg`. Nothing sharp.
- shadow is soft, wide, and low-opacity — or absent. Never a hard drop shadow.
- no inner top highlight, no rim light, no gradient fills.

### Texture

**None.** No instrument grid, no dot grid, no scanlines, no noise. Surfaces are
flat warm fills. The one permitted ambient layer:

- **Core bloom** — an extremely subtle radial warmth behind the Reactor column,
  state-reactive (idle → running → blocked/allowed). At reference intensity this
  is *barely perceptible* — it should register as the column being slightly
  warmer, never as a visible glow. If a viewer can point at it and call it a
  glow, cut its opacity in half.

### Type

Keep Inter + JetBrains Mono; change how they're used.
- **Sans carries the interface.** Section labels are sentence-case sans in
  `--muted` at 13px — like "Git tools" / "Progress" in the reference. Do **not**
  set the whole UI in tiny uppercase letterspaced mono; that was the previous
  direction and it reads as costume.
- **Mono is for machine data only**: evidence ids, byte counts, wire payloads,
  package names, diffs, timestamps, tool names. Inline code gets a subtle filled
  chip (`bg-surface-2`, `rounded-md`, no border), as in the reference.
- Prose is comfortable, not cramped: ~15px body, `leading-relaxed`.
- Tabular figures (`tnum`) on every number that can change.
- Reserve uppercase for genuine status stamps (BLOCKED, CLEAN) — never for
  routine labels.

### Motion

Purposeful, fast, physical. `cubic-bezier(0.16,1,0.3,1)`, 200–450ms. Sessions
tick in; signals snap with a red bloom; the verdict lands with weight. Live
elements breathe slowly. Nothing bounces, nothing spins for decoration. All of it
must vanish under `prefers-reduced-motion` (already wired — keep it).

---

## 2. Signature elements (the bespoke bits)

Reactor's identity comes from **structure and restraint**, not ornament. The
reference proves a dark UI can be memorable while being almost entirely flat.
Use these; don't invent a sixth, and don't add decoration.

1. **Soft containment panel** — the Reactor column reads as a distinct contained
   surface: slightly lifted fill, generous `rounded-2xl`, faint edge. **No
   corner ticks, no bezel ring, no blast-door ornament** — that was the previous
   direction and it fights the reference aesthetic. The column earns its
   presence through fill, radius and spacing alone. On BLOCKED, its edge picks up
   a restrained danger tint; that's the whole treatment.
2. **Status dots** — small, calm, mostly flat. A soft halo is permitted **only**
   on a genuinely live/streaming dot, and kept subtle. No neon cores.
3. **Gutter rail** — dense streams (wire log, session ladder, egress, analyst
   trace) get a left gutter: a faint vertical rail with index/time in mono and a
   node marker where a row matters. Reads as a timeline, not a list. This is the
   main structural device and it survives the restyle intact.
4. **Fill-and-space separation** — rows are separated by spacing and, where
   needed, a very faint divider. **No cards inside cards**, no zebra striping, no
   boxed rows. Grouping comes from a shared panel fill, exactly as the reference
   groups "Git tools" and "Progress".
5. **Quiet in-flight affordance** — one subtle indicator for active work (a
   soft pulsing dot or a low-contrast progress line). No luminous sweeps
   traveling panel edges.

**Removed from the previous direction — do not use:** corner tick marks,
instrument/dot grids, glow shadows (`shadow-glow-*`), neon LED cores, scanline
sweeps, uppercase-mono chrome labels.

---

## 3. Layout & density

- Console fills the viewport; the two columns scroll independently, headers
  sticky within their column. No page-level double scrollbar.
- The left (scan) column is deliberately **quieter and flatter** — lower
  contrast, no bezel, no bloom. The asymmetry *is* the argument.
- **Comfortable, not compressed.** The reference breathes: ~16–20px panel
  padding, 8–10px row rhythm, clear grouping gaps between sections. Telemetry
  streams may run tighter (6–8px rows) since they're machine data, but chrome,
  prose and controls get room. Density comes from removing borders and boxes,
  not from squeezing whitespace.
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


---

## 6. Token & primitive reference (implemented)

The foundation layer is shipped, retuned to the warm-neutral reference
aesthetic. This section is the literal contract: every token, class,
data-attribute and utility that exists, and how to use it. Copy from here.

Owned files: `app/globals.css`, `tailwind.config.ts`, `app/layout.tsx`,
`components/TopBar.tsx`, `components/ThemeToggle.tsx`, `components/ReactorMark.tsx`,
`components/ui.tsx`.

### 6.0 Rules of engagement

- **No raw hex, ever.** Use the semantic utilities below. The hex column exists
  so you can reason about contrast, not so you can type it.
- Every color token is space-separated RGB in a CSS var, exposed to Tailwind via
  `rgb(var(--x) / <alpha-value>)`. So **all opacity utilities work** on all of
  them: `bg-surface-2/60`, `border-danger/30`, `text-live/70`, `ring-accent/60`,
  `from-accent/20`, `fill-accent`, `divide-line`.
- **Warm base, cool accent.** Every neutral carries a warm cast; only the
  semantic roles are chromatic. If you find yourself reaching for a second blue
  thing on a view, one of them is wrong.
- Density (DESIGN §3): panel padding `p-4`/`p-5` (16/20px), row rhythm
  `py-2`/`py-2.5` (8/10px), telemetry rows may go to `py-1.5`. Panels
  `rounded-2xl`, controls `rounded-xl`, chips `rounded-lg`.
- **If you add a new class to `@layer components` in `globals.css`, add its name
  to `safelist` in `tailwind.config.ts`** — Tailwind tree-shakes the components
  layer against `content`. Better still, make sure a real component uses it.
- **A running `next dev` server does not always reload `tailwind.config.ts`.**
  If a config-derived change (type scale, radii, `shadow-glow-*`) doesn't show
  up, restart the dev server. `globals.css` edits do hot-reload.

### 6.1 Color tokens

Each row gives the CSS var, the Tailwind color name (usable as `bg-…`, `text-…`,
`border-…`, `ring-…`, `fill-…`, `stroke-…`, `divide-…`, `from-…`), and the two
theme values.

#### Surfaces — warm neutral, even lightness steps

| CSS var | Tailwind | Dark | Light | Use it for |
|---|---|---|---|---|
| `--bg` | `bg-bg` | `#1D1B18` | `#F8F5F1` | The page. Also `ring-offset-bg` for focus rings. |
| `--surface` | `bg-surface` | `#24211E` | `#FFFDFB` | Panel fill. The default `.panel` background. |
| `--surface-2` | `bg-surface-2` | `#2B2825` | `#F2EFEA` | The lifted/contained fill: `.bezel`, inline code chips, hovered rows, segmented-control tracks. |
| `--surface-3` | `bg-surface-3` | `#33302B` | `#EBE7E1` | The strongest neutral fill: selected row, neutral solid chip. |
| `--line` | `border-line` | `#322F2B` | `#EBE6E0` | Every edge. Tuned to ~white 6% over `surface` — it should read as an edge, not a drawn line. |
| `--line-strong` | `border-line-strong` | `#44403B` | `#D6D0C8` | Emphasized dividers, scrollbar thumb, resting rail nodes. |

Dark surfaces are even ~3.4 L\* steps off a warm near-black at hue ~30-38. Light
surfaces are near-white warm panels on warm paper, with wells stepping *down*.

#### Ink — warm greys, never pure white or pure black

| CSS var | Tailwind | Dark | Light | Use it for | Min contrast |
|---|---|---|---|---|---|
| `--fg` | `text-fg` | `#E8E5E1` | `#1F1D1A` | Body copy, headings, primary values. | 10.5 / 13.7 |
| `--muted` | `text-muted` | `#A0998F` | `#6B655D` | **Section labels**, secondary prose, glosses, telemetry values. | 4.7 / 4.7 |
| `--faint` | `text-faint` | `#837C73` | `#7F786E` | Timestamps, indices, units, inactive dots. Never prose. | 3.2 / 3.5 |

Verified on all four surfaces in both themes (`fg`/`muted` ≥ 4.5:1,
`faint` ≥ 3:1). Note `muted` and `faint` are one step lighter/darker than the
raw reference values — at the reference values `faint` measured 2.2:1 in dark
and failed DESIGN §4.

#### Roles

| CSS var | Tailwind | Dark | Light | Use it for |
|---|---|---|---|---|
| `--accent` | `text-accent` `bg-accent` `border-accent` | `#5B9BF8` | `#1F5FD8` | The one primary action per view, plus selection state. Restrained cool blue — the design's only chromatic tension. Solid fills, **no glow**. |
| `--accent-fg` | `text-accent-fg` | `#1A1815` | `#FFFFFF` | Text **on** a solid `bg-accent`. Flips per theme. |
| `--accent-dim` | `bg-accent-dim` | `#1B2637` | `#E7EEFB` | An opaque accent-tinted fill when `/10` alpha won't do. |
| `--live` | `text-live` `bg-live` | `#7BA8AC` | `#356B70` | **Very sparingly**: live dots, the active tick, the in-flight indicator. Desaturated blue-teal (sat 23 vs accent's 92) so it can never out-shout accent. |
| `--live-fg` | `text-live-fg` | `#1A1815` | `#FFFFFF` | Text on solid `bg-live`. |
| `--live-dim` | `bg-live-dim` | `#1E2A2C` | `#E4EEEF` | Opaque live-tinted fill. |
| `--success` | `text-success` `bg-success` | `#4ADE80` | `#13713D` | mcp-scan CLEAN, ALLOWED verdict, added lines, `benign_profile`. |
| `--success-fg` | `text-success-fg` | `#1A1815` | `#FFFFFF` | Text on solid `bg-success`. |
| `--success-dim` | `bg-success-dim` | `#1A3023` | `#E4F3E9` | Opaque success-tinted fill. |
| `--danger` | `text-danger` `bg-danger` | `#F87171` | `#B4322B` | **Reserved.** Fired signals, removed bytes, BLOCKED. Nothing else. |
| `--danger-fg` | `text-danger-fg` | `#1A1815` | `#FFFFFF` | Text on solid `bg-danger`. |
| `--danger-dim` | `bg-danger-dim` | `#3A211F` | `#FCEBE9` | Opaque danger-tinted fill. |
| `--warning` | `text-warning` `bg-warning` | `#DCA94F` | `#7E5410` | Sub-critical signals (`canary_read`, medium severity). |
| `--warning-fg` | `text-warning-fg` | `#1A1815` | `#FFFFFF` | Text on solid `bg-warning`. |
| `--warning-dim` | `bg-warning-dim` | `#372B18` | `#FAF0E0` | Opaque warning-tinted fill. |

> **Always use `bg-danger text-danger-fg`, never `bg-danger text-white`.** In
> dark mode the role colors are light, so `*-fg` resolves to warm near-black;
> white on them fails contrast badly (1.7-2.8:1).

`live` (hue ~186) and `success` (hue ~142-147) sit ~44° apart and differ sharply
in saturation. Even so, **never encode state in color alone** — pair every dot
with a label.

#### Non-color theme vars

| Var | Dark | Light | Meaning |
|---|---|---|---|
| `--bloom-k` | `1` | `1.4` | Per-theme multiplier on `.core-bloom` / `.console-veil` alpha. |
| `--led-halo` | `1` | `0.85` | Pulsing-dot halo strength. |
| `--shadow` | `0 0 0` | `60 48 36` | The RGB shadows are cast in (warm in light). |
| `--panel-shadow` | — | — | Soft, wide, low-opacity elevation. No inner highlight. |
| `--panel-shadow-flat` | — | — | A composable **no-op** (`0 0 0 0 transparent`), so `var(--panel-shadow-flat), <more>` stays valid CSS. |

### 6.2 CSS component classes

Written in `@layer components` in `globals.css`, so **every Tailwind utility
beats them** — `<div class="panel bg-surface-2">` works as expected.

> **Pseudo-element budget.** Only three primitives still use a pseudo-element:
> `.core-bloom` (`::before`), `.rail` (`::before`) and `.scan-sweep` (`::after`).
> Everything else is plain declarations. `.bezel` no longer uses either, so it
> now composes freely. The only invalid combination is `.core-bloom` + `.rail`
> on the same element (both want `::before`) — nest them.

---

#### `.panel` — the surface

Defined by its fill, not its outline: `rounded-2xl`, faint edge, soft wide
shadow. No inner top highlight, no rim light, no gradient.

```html
<div class="panel p-5">…</div>
```

Variants: `.panel-flat` (identical, zero cast shadow — for grouping inside
another panel's rhythm) and `.panel-float` (a soft, wide, *low-opacity* drop
shadow; **only** for genuinely floating things: the verdict banner, popovers).

#### `.hairline` / `.rule`

```html
<div class="hairline pt-3">…</div>             <!-- a 1px top edge in --line -->
<span class="rule" aria-hidden="true"></span>  <!-- flex-1 edge that fades out -->
```

`.rule` is `height:1px; flex:1` — it must live inside a flex row. Prefer spacing
over dividers; reach for these second.

#### `.bezel` — the soft containment panel (DESIGN §2.1)

The Reactor column's treatment. **The corner tick marks, the inset ring and the
blast-door ornament are gone.** What remains: a slightly lifted fill
(`surface-2`), a generous radius, a faint edge, and on BLOCKED a restrained
danger tint on that edge. That is the whole treatment.

Class name and the `data-state` API are unchanged.

```html
<div class="bezel p-5" data-state="idle">…</div>
```

| `data-state` | Edge |
|---|---|
| *(omitted)* or `"idle"` | `--line` — the standard faint edge |
| `"running"` | `--line-strong` at 70% — barely firmer |
| `"allowed"` | `--success` at 28% |
| `"blocked"` | `--danger` at 30% |

Border and background transition over 300ms on the DESIGN easing, so state
changes settle rather than snap. React wrapper: `<Bezel state="blocked">…</Bezel>`.

#### `.core-bloom` — the one permitted ambient layer

State-reactive radial warmth behind the Reactor column. **It is barely
perceptible by design** — the column should read as slightly warmer, never as a
visible glow. If you can point at it and call it a glow, halve `--bloom-a`.

Put it on the **Reactor column wrapper**, not on a panel.

```html
<div class="core-bloom" data-core="running">
  <div class="panel …">…</div>
  <div class="panel …">…</div>
</div>
```

| `data-core` | Hue | Alpha | Motion |
|---|---|---|---|
| *(omitted)* or `"idle"` | `--fg` (warm, no chroma) | 0.014 | none |
| `"running"` | `--accent` | 0.022 | slow breath, `core-breathe 6s` |
| `"blocked"` | `--danger` | 0.03 | none |
| `"allowed"` | `--success` | 0.022 | none |

**How it stacks:** the bloom paints above the wrapper's own background and
*below* every descendant's background, so it reads through the gaps between
panels. Tunable: `--bloom` (hue), `--bloom-a` (alpha).

#### `.instrument-grid` / `.instrument-grid-lines` — NEUTRALIZED, do not use

The reference aesthetic has no texture; surfaces are flat warm fills. Both
classes are retained as **no-ops** so existing markup keeps working — they paint
nothing at all. Do not add them to new markup, and strip them when you touch a
file that has them.

#### `.rail` / `.rail-row` / `.rail-time` / `.rail-node` — gutter rail (DESIGN §2.3)

**The main structural device, kept intact.** Dense streams (wire log, session
ladder, egress, analyst trace) read as a timeline rather than a list.

```html
<div class="rail">
  <div class="rail-row py-2">
    <div class="rail-time">00:01.4<span class="rail-node" data-tone="live"></span></div>
    <div class="font-mono text-xs text-muted">tools/call notes.read</div>
  </div>
  <div class="rail-row py-2">
    <div class="rail-time">00:02.7<span class="rail-node" data-tone="danger"></span></div>
    <div class="font-mono text-xs text-danger">rug_pull · +47 bytes</div>
  </div>
</div>
```

- `.rail-time` is already mono, 11px, tabular, `--faint`, right-aligned. Don't
  restyle it; put the node marker inside it.
- `--rail-x` (default `4rem`) is both the distance from the rail box's left edge
  to the rail and the width of the index column. Override inline
  (`style={{ "--rail-x": "2.5rem" }}`) or use `.rail-tight` (`2rem`).
- `.rail-node` accepts `data-tone="danger" | "live" | "accent" | "success" | "warning"`.
  Omit for a resting `--line-strong` bead. **Nodes are flat — no halos.**
- Rows separate by spacing and, where needed, one faint divider. No cards inside
  cards, no zebra striping, no boxed rows.

#### `.scan-sweep` — the quiet in-flight indicator (DESIGN §2.5)

Name and `data-tone` API retained; **the luminous traveling line is gone.** It is
now a single low-contrast hairline inset along the panel's top edge that breathes
slowly. It should be easy to miss until you look for it. One at a time on the
page.

```html
<div class="panel scan-sweep p-5">…</div>
```

Defaults to `live`. Retone with `data-tone="accent" | "danger" | "success"`, or
set `--sweep`. Uses `::after` only, so it composes with `.panel` and `.bezel`.
Under `prefers-reduced-motion` it holds steady instead of breathing.

#### `.led` — the status dot (DESIGN §2.2)

Small, calm, flat. **No neon core, no specular gloss, no resting halo.** A soft
halo appears only on `data-pulse="true"`, reserved for genuinely live/streaming
state.

```html
<span class="led" data-tone="live" data-pulse="true"></span>
```

- `data-tone`: `accent` | `live` | `danger` | `success` | `warning` | `neutral`.
  `neutral` is an inactive dot at 45% `--faint`, and never pulses. Omitting
  `data-tone` gives the same inactive look.
- `data-pulse="true"` adds the slow 2.8s halo breath. Omit the attribute when
  not pulsing.
- Size: `.led-sm` (5px), default (7px), `.led-lg` (9px), or set `--led-size`.
- **Prefer the React atoms**: `<Led tone="live" pulse size="lg" />` or
  `<StatusDot tone="danger" />`.

#### Type helpers

| Class | What it is |
|---|---|
| `.strip-label` | `text-sm font-medium text-muted` — **sentence-case sans, 13px.** The section label. Not uppercase, not mono, not letterspaced. |
| `.telemetry` | `font-mono text-2xs tabular-nums text-muted` — a machine-data value. |
| `.code-chip` | `rounded-md bg-surface-2 px-1.5 py-0.5 font-mono text-2xs text-fg` — inline code as a subtle filled chip, **no border**. |
| `.tnum` | Tabular figures. Put on **every number that can change**. |

#### Chrome

| Class | What it is |
|---|---|
| `.chrome-edge` | Retained name, **neutralized** — the luminous accent underline is gone. Calm chrome carries its own `border-b border-line` and nothing else. |
| `.focus-ring` | Opt-in `:focus-visible` ring (`ring-2 ring-accent/60 ring-offset-2 ring-offset-bg`). A zero-specificity outline already applies to every `a`, `button`, `summary`, `input`, `select`, `textarea`, `[role="button"]`, `[tabindex]` — use this class when you want the offset ring look. |
| `.console-veil` | Retained name, **neutralized** to a barely-there warm lift behind the console. |
| `.sweep-bg` | Retained name, **neutralized** to a low-contrast neutral shimmer (pairs with `animate-sweep`). No accent glow. |

### 6.3 Animations & shadows

`animate-<name>` utilities. The first three have their `@keyframes` in
`globals.css` (they are also consumed directly by `.core-bloom`, `.scan-sweep`
and `.led`); the rest are declared in `tailwind.config.ts`.

| Utility | Timing | Use |
|---|---|---|
| `animate-core-breathe` | `6s` infinite | Slow ambient opacity breath. Already on `.core-bloom[data-core="running"]`. |
| `animate-scan-sweep` | `3.2s` infinite | **No longer travels** — the same quiet breath. Already on `.scan-sweep`. |
| `animate-led-pulse` | `2.8s` infinite | The live-dot halo breath. Already on `.led[data-pulse="true"]`. |
| `animate-tick-in` | `0.3s` instrument, `both` | A row ticking in from the rail — the session-ladder cadence. |
| `animate-fade-slide-up` | `0.34s` instrument, `both` | Element entering from below. |
| `animate-fade-in` | `0.4s` ease-out, `both` | Plain reveal. |
| `animate-signal-snap` | `0.42s` instrument + `danger-pulse 1.1s` | A fired signal row. `danger-pulse` is now a soft warm danger cast that settles — no neon rim. |
| `animate-verdict-in` | `0.5s` instrument, `both` | The verdict banner landing with weight. |
| `animate-spin-slow` | `1.1s` linear infinite | Spinner. |
| `animate-pulse-dot` | `1.3s` infinite | Legacy opacity blink. Prefer `.led[data-pulse]`. |
| `animate-sweep` | `1.6s` infinite | Pairs with `.sweep-bg`. |
| `animate-bar-grow` | `0.9s` instrument, `both` | Scorecard bars. |

Easing utility: **`ease-instrument`** = `cubic-bezier(0.16,1,0.3,1)`. Use it on
every transition (200-450ms). All motion is disabled under
`prefers-reduced-motion`.

Shadows — **soft, wide, low-opacity, or absent. Never a hard drop shadow.**

| Utility | What it is |
|---|---|
| `shadow-panel` | The standard soft elevation (`var(--panel-shadow)`). |
| `shadow-panel-flat` | The composable no-op. |
| `shadow-float` | Soft wide elevation for genuinely floating things. |
| `shadow-card` / `shadow-card-dark` | Both alias `--panel-shadow`. |
| `shadow-glow-accent` `-live` `-danger` `-success` `-warning` | **Names retained, neon removed.** Now a soft, wide, low-opacity cast tinted by the role (`0 8px 28px -14px rgb(var(--role) / 0.3-0.4)`). No `0 0 0 1px` rim, no halo. **Prefer a border tint (`border-danger/30`) over reaching for these at all.** |

### 6.4 Type scale

Comfortable, not compressed. **Sans carries the interface; mono is for machine
data only.**

| Utility | Size | Line height | Tracking | Use |
|---|---|---|---|---|
| `text-3xs` | 10px | 15px | `0.005em` | Mono evidence ids. Machine data only. |
| `text-2xs` | 11px | 16px | `0.005em` | Mono telemetry values, byte counts, rail times. |
| `text-xs` | 12px | 18px | — | Mono wire payloads, transcript lines. |
| `text-sm` / `text-label` | 13px | 20px | — | **Section labels** and secondary prose. The workhorse. |
| `text-base` | 15px | 24px | — | Body prose. |
| `text-lg` | 17px | 26px | `-0.008em` | Small headings. |
| `text-xl` | 19px | 28px | `-0.012em` | Panel headings. |
| `text-2xl` | 22px | 30px | `-0.016em` | Section headings, the verdict word. |
| `text-3xl` | 27px | 34px | `-0.02em` | Display. |
| `text-4xl` | 33px | 40px | `-0.022em` | Scorecard headline. |
| `text-5xl` | 42px | 48px | `-0.026em` | The one big number. |

Tracking utilities (softened hard, names retained): `tracking-label` (`0.02em`),
`tracking-label-wide` (`0.08em`). **Uppercase + `tracking-label-wide` is reserved
for genuine status stamps — BLOCKED, CLEAN, CRITICAL — and nothing else.** Routine
labels are sentence case.

### 6.5 Copy-paste recipes

**A section label** — sentence case, sans, muted:

```tsx
import { SectionLabel } from "@/components/ui";

<SectionLabel right={<span className="telemetry">5 detonations</span>}>
  Wire log
</SectionLabel>
```

Raw equivalent:

```html
<div class="flex items-center justify-between gap-3">
  <span class="strip-label">Wire log</span>
  <span class="telemetry">5 detonations</span>
</div>
```

**A telemetry row** — spacing-separated, mono values, no boxes:

```html
<div class="flex items-baseline justify-between gap-3 py-2">
  <span class="font-mono text-xs text-muted">tools/list</span>
  <span class="tnum font-mono text-2xs text-faint">2,048 B</span>
</div>
```

**A status dot with its label** — never color alone:

```tsx
import { Led } from "@/components/ui";

<span className="inline-flex items-center gap-2">
  <Led tone="live" pulse />
  <span className="text-sm text-muted">Live</span>
</span>
```

**Buttons** — soft radius, filled, restrained, one blue primary per view:

```html
<button class="focus-ring rounded-xl bg-accent px-3.5 py-2 text-sm font-medium text-accent-fg">Detonate</button>
<button class="focus-ring rounded-xl bg-surface-2 px-3.5 py-2 text-sm font-medium text-fg hover:bg-surface-3">Replay demo</button>
<button class="focus-ring rounded-xl px-3.5 py-2 text-sm font-medium text-muted hover:bg-surface-2 hover:text-fg">Cancel</button>
```

**Inline code / an evidence id**:

```html
<span class="code-chip">@acme/notes-mcp</span>
<code class="code-chip tnum text-3xs text-danger bg-danger/10">wire:4:tools/list</code>
```

**The Reactor column shell** — bloom + soft containment + quiet indicator:

```html
<div class="core-bloom flex min-h-0 flex-col gap-4" data-core="running">
  <div class="bezel scan-sweep flex min-h-0 flex-1 flex-col p-5" data-state="running">
    …
  </div>
</div>
```

### 6.6 `components/ui.tsx` exports

All export names, prop signatures and the `Tone` import contract are unchanged.

| Export | Signature | Notes |
|---|---|---|
| `UiTone` | `type UiTone = Tone \| "accent" \| "live"` | `Tone` still comes from `@/lib/signals`. |
| `ToneStyles` | interface | The shape of a `toneClasses` entry. |
| `toneClasses` | `Record<UiTone, ToneStyles>` | Keys: `danger`, `warning`, `success`, `neutral`, `accent`, `live`. Fields: `text`, `soft`, `border`, `dot`, `solid`, `led` (the `data-tone` value), `glow` (a softened `shadow-glow-*`). `solid` uses `text-*-fg`, never `text-white`. |
| `Chip` | `{ children, tone?: UiTone, className?, mono? }` | Soft-radius filled pill, **borderless**. `mono` switches to `font-mono text-2xs` for machine data. |
| `SeverityChip` | `{ severity: Severity \| string }` | A genuine status stamp: sans, uppercase, `tracking-label-wide`. |
| `StaticBlindBadge` | `()` | Soft accent chip with a small accent dot. Sentence case. `title` preserved. |
| `EvidenceIds` | `{ ids: string[], tone?: UiTone }` | `.code-chip` pills at `text-3xs`, tinted by tone. |
| `Led` | `{ tone: UiTone, pulse?, size?: "sm"\|"md"\|"lg", className? }` | The status dot. |
| `StatusDot` | `{ tone: UiTone, pulse? }` | Same contract as before; renders `<Led>`. |
| `Bezel` | `{ children?, state?: "idle"\|"running"\|"blocked"\|"allowed", className?, …divProps }` | `.bezel` wrapper. |
| `SectionLabel` | `{ children, right? }` | Sentence-case sans label + right slot. |
