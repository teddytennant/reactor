"use client";

import { cn } from "@/lib/cn";
import type { Scorecard as ScorecardData } from "@/lib/scorecard";
import { signalMeta } from "@/lib/signals";
import { SectionLabel, StaticBlindBadge } from "@/components/ui";

/* ==========================================================================
   The presentation-grade metrics slide (SPEC §7). Projected in front of a
   room, so it has to read from the back — and it has to fit on ONE screen.
   Everything below is sized to land inside 1440x900 without a scrollbar, so
   the "comfortable" density of DESIGN §3 yields to the fit constraint here:
   this is a slide, not a document. Nothing is dropped to get there — every
   figure the fixture provides is still on the page; the rhythm is what gives.

   Four beats, in one hierarchy, top to bottom:
     1. the headline
     2. three claims        24/25 · 0 false blocks · 9 static-blind
     3. the comparison      THE HERO — one honest baseline, two detectors
     4. supporting metrics + the line they leave with

   The whole slide is drawn on ONE honest baseline: the zoo itself, one cell
   per artifact, 25 cells wide, always starting at zero. Nothing is scaled,
   truncated or rescaled per row — so the two detector rows in the centerpiece
   sit on identical column geometry and the gap between them is literal: the
   nine columns that are hollow in the mcp-scan row are lit in the Reactor row,
   and the dimension line underneath measures exactly those nine columns.

   Cell vocabulary (form first, then weight — never colour alone):
     filled --faint  caught by both detectors — table stakes
     filled --fg     caught by Reactor only   — the static-blind delta
     filled green    a benign server cleared  — no false quarantine
     hollow red      walked past this detector

   Reactor's series is `--fg` — white in dark, near-black in light, the same
   ink as the primary action. There is no blue on this page: the two detector
   fills are one warm-neutral ordinal ramp two steps apart (OKLab ΔE 33 dark /
   34 light, well past the 15 normal-vision floor; both are near-achromatic so
   CVD leaves the gap intact), and each still clears 3:1 against the panel it
   sits on (fg 11.7:1 / 14.7:1, faint 3.6:1 / 3.8:1). `--line-strong` was the
   obvious third step down and is unusable as a fill: 1.4:1 on `surface-2`.
   ========================================================================== */

export function Scorecard({ data, live }: { data: ScorecardData; live: boolean }) {
  const { detection, false_quarantine, static_blind, redteam, time_to_verdict_ms, cost_usd, zoo } = data;

  // Every segment length below comes straight out of the scorecard. `missed`
  // keeps the original fallback so a live payload without `missed_by_both`
  // still renders the same figure.
  const total = zoo.malicious;
  const both = static_blind.static_caught;
  const blind = static_blind.static_blind_catches;
  const missed = static_blind.missed_by_both ?? zoo.malicious - static_blind.reactor_caught;
  const uncaught = Math.max(0, detection.total - detection.caught);
  const cleared = Math.max(0, false_quarantine.total - false_quarantine.blocked);

  return (
    <div className="mx-auto w-full max-w-[1220px]">
      {/* ---- headline ----------------------------------------------------
          No eyebrow band. The title is the first thing on the slide, and
          provenance has moved to the quiet footer note at the very bottom. */}
      <header>
        <h1 className="text-2xl font-semibold tracking-tight text-fg sm:text-3xl">
          The same zoo, through two detectors
        </h1>
        <p className="mt-1 max-w-[760px] text-balance text-sm text-muted">
          <span className="tnum">{zoo.malicious}</span> malicious artifacts,{" "}
          <span className="tnum">{zoo.benign}</span> real MCP servers.{" "}
          <span className="text-fg">Static reads the label; Reactor watches it behave.</span>
        </p>
      </header>

      {/* ---- the three claims -------------------------------------------- */}
      <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Claim
          lamp="neutral"
          label="Detection"
          value={`${detection.caught}`}
          unit={`/${detection.total}`}
          caption="Malicious artifacts caught"
          footnote={`${pct(detection.rate)} detection rate`}
          strip={
            <CellStrip
              total={detection.total}
              height="h-[clamp(0.625rem,1.7vh,1.125rem)]"
              label={`${detection.caught} of ${detection.total} malicious artifacts caught, ${uncaught} not caught`}
              segments={[
                { n: detection.caught, kind: "reactor", title: `${detection.caught} caught` },
                { n: uncaught, kind: "escaped", title: `${uncaught} not caught` },
              ]}
            />
          }
        />
        <Claim
          lamp="success"
          label="False quarantine"
          value={`${false_quarantine.blocked}`}
          caption={`False blocks · ${false_quarantine.total} real servers`}
          footnote={`${pct(false_quarantine.rate)} false-quarantine rate`}
          strip={
            <CellStrip
              total={false_quarantine.total}
              height="h-[clamp(0.625rem,1.7vh,1.125rem)]"
              label={`${false_quarantine.blocked} of ${false_quarantine.total} benign servers falsely blocked; ${cleared} cleared`}
              segments={[
                { n: false_quarantine.blocked, kind: "blocked", title: `${false_quarantine.blocked} falsely blocked` },
                { n: cleared, kind: "clean", title: `${cleared} cleared` },
              ]}
            />
          }
        />
        <Claim
          lamp="neutral"
          label="Static-blind"
          value={`${blind}`}
          caption="Catches with no static signature"
          footnote={`mcp-scan caught ${both} of ${total}`}
          strip={
            <CellStrip
              total={total}
              height="h-[clamp(0.625rem,1.7vh,1.125rem)]"
              label={`${both} caught by both detectors, ${blind} caught only by Reactor, ${missed} missed by both`}
              segments={[
                { n: both, kind: "both", title: `${both} caught by both` },
                { n: blind, kind: "reactor", title: `${blind} caught only by Reactor` },
                { n: missed, kind: "escaped", title: `${missed} missed by both` },
              ]}
            />
          }
        />
      </div>

      {/* ---- the centerpiece: one baseline, two detectors -----------------
          The hero. It keeps its full 25-cell geometry at every size; the cell
          *height* is what flexes with the viewport, never the column count. */}
      <div className="panel mt-3 bg-surface-2 p-4 sm:p-5">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="strip-label whitespace-nowrap">Static-blind rate</span>
          <span className="rule" aria-hidden="true" />
          <span className="tnum whitespace-nowrap text-sm text-muted">
            {total} malicious artifacts · 1 cell = 1 artifact
          </span>
        </div>

        <div className="mt-3.5">
          <DetectorRow name="mcp-scan" note="Static · reads the description" caught={both} total={total} />
          <CellStrip
            total={total}
            height="h-[clamp(1rem,3.1vh,2rem)]"
            label={`mcp-scan: ${both} of ${total} caught, ${blind + missed} walked past`}
            segments={[
              { n: both, kind: "both", title: `${both} caught by both` },
              { n: blind, kind: "escaped", title: `${blind} walked past mcp-scan — Reactor caught these` },
              { n: missed, kind: "escaped", title: `${missed} missed by both` },
            ]}
          />

          <div className="mt-3">
            <DetectorRow
              name="Reactor"
              note="Runtime · watches it behave"
              caught={static_blind.reactor_caught}
              total={total}
              emphasize
            />
            <CellStrip
              total={total}
              height="h-[clamp(1rem,3.1vh,2rem)]"
              label={`Reactor: ${static_blind.reactor_caught} of ${total} caught, of which ${blind} are static-blind`}
              segments={[
                { n: both, kind: "both", title: `${both} caught by both` },
                { n: blind, kind: "reactor", title: `${blind} caught only by Reactor` },
                { n: missed, kind: "escaped", title: `${missed} missed by both` },
              ]}
            />
          </div>

          {/* The dimension line: measures the exact columns that differ. */}
          {blind > 0 ? (
            <div
              className="grid gap-x-0.5"
              style={{ gridTemplateColumns: `repeat(${total}, minmax(0, 1fr))` }}
              aria-hidden="true"
            >
              <div
                className="flex items-center gap-2 pt-1.5"
                style={{ gridColumn: `${Math.min(both + 1, total)} / span ${blind}` }}
              >
                <span className="h-3 w-px flex-none bg-fg/70" />
                <span className="h-px flex-1 bg-fg/40" />
                <span className="tnum text-xl font-semibold leading-none tracking-tight text-fg sm:text-2xl">
                  {blind}
                </span>
                <span className="hidden whitespace-nowrap text-sm font-medium text-muted sm:inline">
                  Static-blind
                </span>
                <span className="h-px flex-1 bg-fg/40" />
                <span className="h-3 w-px flex-none bg-fg/70" />
              </div>
            </div>
          ) : null}
        </div>

        {/* key — form and label, never colour alone. Grouped by spacing, not
            by a rule: the legend belongs to the chart above it. */}
        <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-1.5">
          <KeyItem swatch={CELL.reactor}>
            Reactor only · <span className="tnum">{blind}</span>
          </KeyItem>
          <KeyItem swatch={CELL.both}>
            Caught by both · <span className="tnum">{both}</span>
          </KeyItem>
          <KeyItem swatch={CELL.escaped}>Hollow = walked past this detector</KeyItem>
        </div>

        {/* the nine, itemised — the panel's one divider */}
        <div className="mt-3.5 border-t border-line pt-3">
          <SectionLabel
            right={
              <span className="hidden sm:inline-flex">
                <StaticBlindBadge />
              </span>
            }
          >
            The {blind}, by signal type
          </SectionLabel>
          <div className="mt-2 grid grid-cols-2 gap-x-5 gap-y-3 sm:grid-cols-4 sm:gap-x-0 sm:gap-y-0 sm:divide-x sm:divide-line">
            {static_blind.by_type.map((t) => (
              <div key={t.type} className="min-w-0 sm:px-5 sm:first:pl-0 sm:last:pr-0">
                <div className="flex items-baseline gap-2">
                  <span className="tnum text-lg font-semibold leading-none tracking-tight text-fg">{t.reactor}</span>
                  <span className="truncate font-mono text-2xs text-fg">{t.type}</span>
                </div>
                <div className="mt-1 truncate text-2xs text-muted">
                  {signalMeta(t.type).label} · mcp-scan <span className="tnum">{t.static}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* ---- supporting telemetry + the line they should leave with -------
          One panel, not two: the three cost/latency figures are the evidence
          under the closing sentence, so they sit with it rather than in their
          own half-empty row of tiles. */}
      <div className="panel mt-3 p-4 sm:p-5">
        <div className="grid grid-cols-1 gap-y-2.5 sm:grid-cols-3 sm:gap-y-0 sm:divide-x sm:divide-line">
          <Telemetry
            label="Time to verdict"
            value={`${(time_to_verdict_ms.mean / 1000).toFixed(1)}s`}
            sub={`p50 ${(time_to_verdict_ms.p50 / 1000).toFixed(1)}s · p95 ${(time_to_verdict_ms.p95 / 1000).toFixed(1)}s`}
          />
          <Telemetry
            label="Cost / detonation"
            value={`$${cost_usd.mean.toFixed(3)}`}
            sub={`$${cost_usd.total.toFixed(2)} for the whole zoo`}
          />
          <Telemetry
            label="Red-team escape"
            value={`${redteam.escaped}/${redteam.mutations}`}
            sub={`${pct(redteam.rate)} of mutations slipped past`}
          />
        </div>

        <div className="mt-3 flex flex-wrap items-end justify-between gap-x-6 gap-y-1 border-t border-line pt-3">
          <p className="min-w-0 flex-1 text-base tracking-tight text-fg">
            <span className="tnum font-semibold">{static_blind.static_blind_catches}</span> of those catches are things
            a description scanner{" "}
            <span className="font-semibold underline decoration-fg/40 decoration-2 underline-offset-4">cannot</span>{" "}
            see. Not doesn&apos;t — can&apos;t.{" "}
            <span className="text-muted">
              VirusTotal never saw the agent supply chain coming. Static scanners read the label;{" "}
            </span>
            <span className="text-fg">we watch it behave.</span>
          </p>
          {/* Provenance, kept honest but quiet: which numbers these are, and when. */}
          <span className="tnum whitespace-nowrap text-2xs text-faint">
            {live ? "Live eval" : "Bundled fixture"} · {data.generated_at}
          </span>
        </div>
      </div>
    </div>
  );
}

// ---- pieces ---------------------------------------------------------------

function pct(x: number): string {
  return `${Math.round(x * 100)}%`;
}

/**
 * The cell vocabulary. Fill vs. hollow does the work first; after that it is
 * ink *weight*, not hue — so the strips survive a washed-out projector,
 * greyscale print and CVD alike.
 *
 * `reactor` is `--fg`: Reactor's own ink, the same weight as the primary
 * action, so a lit cell literally reads "Reactor caught this". `both` stays a
 * step down at `--faint` — two ordinal steps of one warm hue, ΔE 33/34, each
 * still ≥ 3.5:1 on its panel.
 */
type CellKind = "both" | "reactor" | "clean" | "escaped" | "blocked";

const CELL: Record<CellKind, string> = {
  both: "bg-faint",
  reactor: "bg-fg",
  clean: "bg-success/70",
  escaped: "border border-danger/70 bg-danger/10",
  blocked: "bg-danger",
};

interface Segment {
  n: number;
  kind: CellKind;
  title: string;
}

/**
 * One cell per artifact on a fixed `total`-column grid. Every strip on the page
 * uses the same geometry, so rows stack into a column-aligned comparison and a
 * dimension line can measure a segment exactly. Segments nest a sub-grid of the
 * same track width, so the 2px surface gap is uniform across the whole strip.
 *
 * `height` is a viewport-relative `clamp()` so the slide adapts to the screen
 * it is thrown onto: the cells get shorter, the 25 columns never change.
 */
function CellStrip({
  total,
  segments,
  height,
  label,
  className,
}: {
  total: number;
  segments: Segment[];
  height: string;
  label: string;
  className?: string;
}) {
  return (
    <div
      role="img"
      aria-label={label}
      className={cn("grid gap-x-0.5", className)}
      style={{ gridTemplateColumns: `repeat(${total}, minmax(0, 1fr))` }}
    >
      {segments.map((s, i) =>
        s.n > 0 ? (
          <div
            key={i}
            title={s.title}
            className="grid origin-left animate-bar-grow gap-x-0.5"
            style={{
              gridColumn: `span ${s.n}`,
              gridTemplateColumns: `repeat(${s.n}, minmax(0, 1fr))`,
              animationDelay: `${i * 90}ms`,
            }}
          >
            {Array.from({ length: s.n }, (_, c) => (
              <span key={c} className={cn("rounded-sm", height, CELL[s.kind])} />
            ))}
          </div>
        ) : null,
      )}
    </div>
  );
}

/** One of the three headline claims. The figure is the only mono-adjacent thing;
    its label and caption are plain sentence-case sans (DESIGN §1 Type). */
function Claim({
  lamp,
  label,
  value,
  unit,
  caption,
  footnote,
  strip,
}: {
  lamp: "neutral" | "success";
  label: string;
  value: string;
  unit?: string;
  caption: string;
  footnote: string;
  strip: React.ReactNode;
}) {
  return (
    <section className="panel flex flex-col p-4">
      <div className="flex items-center gap-2">
        <span
          className={cn("h-1.5 w-1.5 flex-none rounded-full", lamp === "success" ? "bg-success" : "bg-faint")}
          aria-hidden="true"
        />
        <span className="strip-label whitespace-nowrap">{label}</span>
        <span className="rule" aria-hidden="true" />
      </div>

      {/* Figure and caption share a baseline — the caption is the figure's unit,
          so it costs a column rather than a whole extra line. */}
      <div className="mt-2 flex min-w-0 items-baseline gap-x-1.5">
        <span className="tnum text-3xl font-semibold leading-none tracking-tight text-fg sm:text-4xl">{value}</span>
        {unit ? <span className="tnum font-mono text-base font-medium text-faint sm:text-lg">{unit}</span> : null}
        <span className="ml-1 truncate text-sm text-muted">{caption}</span>
      </div>

      <div className="mt-auto pt-2.5">
        {strip}
        <div className="tnum mt-1.5 text-2xs text-muted">{footnote}</div>
      </div>
    </section>
  );
}

/** The label above a detector's strip. Emphasis is the whole argument, and it now
    runs on the same weight ramp as the cells: Reactor at `--fg`, the scanner at
    `--muted` — the type hierarchy repeating the encoding rather than adding a hue. */
function DetectorRow({
  name,
  note,
  caught,
  total,
  emphasize,
}: {
  name: string;
  note: string;
  caught: number;
  total: number;
  emphasize?: boolean;
}) {
  return (
    <div className="mb-1.5 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
      <div className="flex min-w-0 items-baseline gap-2.5">
        {/* The detector's own name — a tool identifier, so it keeps mono. */}
        <span
          className={cn("font-mono text-sm font-semibold tracking-tight", emphasize ? "text-fg" : "text-muted")}
        >
          {name}
        </span>
        <span className="truncate text-2xs text-muted">{note}</span>
      </div>
      <div className="flex items-baseline gap-1.5 whitespace-nowrap">
        <span
          className={cn("tnum text-lg font-semibold tracking-tight", emphasize ? "text-fg" : "text-muted")}
        >
          {caught}
        </span>
        <span className="text-2xs text-muted">
          / <span className="tnum">{total}</span> caught
        </span>
      </div>
    </div>
  );
}

function KeyItem({ swatch, children }: { swatch: string; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-2 text-2xs text-muted">
      <span className={cn("h-3 w-2 rounded-sm", swatch)} aria-hidden="true" />
      {children}
    </span>
  );
}

/** A supporting figure. Label and value share a baseline so the row costs two
    short lines instead of a tile. */
function Telemetry({ label, value, sub }: { label: string; value: string; sub: string }) {
  return (
    <div className="sm:px-5 sm:first:pl-0 sm:last:pr-0">
      <div className="flex items-baseline gap-2">
        <span className="tnum text-lg font-semibold leading-none tracking-tight text-fg">{value}</span>
        <span className="truncate text-sm text-muted">{label}</span>
      </div>
      <div className="tnum mt-1 truncate text-2xs text-faint">{sub}</div>
    </div>
  );
}
