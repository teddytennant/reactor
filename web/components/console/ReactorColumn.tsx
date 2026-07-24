"use client";

import { useEffect, useRef } from "react";
import { Check, Loader2 } from "lucide-react";
import type { ConsoleState } from "@/lib/reducer";
import { Led, SectionLabel } from "@/components/ui";
import { cn } from "@/lib/cn";
import { ChamberHeader, type ChamberStatus } from "./ChamberHeader";
import { SessionLadder } from "./SessionLadder";
import { ByteDiffCard } from "./ByteDiff";
import { WireLog } from "./WireLog";
import { TranscriptStrip } from "./TranscriptStrip";
import { EgressStrip } from "./EgressStrip";
import { SignalRow } from "./SignalRow";
import { FindingsDeck } from "./FindingsDeck";
import { AnalystTrace } from "./AnalystTrace";
import { VerdictBanner } from "./VerdictBanner";

/** Containment-panel edge tint (DESIGN §2.1): calm, then the verdict. */
type BezelState = "idle" | "running" | "blocked" | "allowed";

/**
 * The Reactor column, in three zones.
 *
 * The column used to be one long scroller with everything in it, which meant
 * the two events that carry the demo (DEMO §1 — the byte diff and the canary)
 * fired, scrolled, and were gone by the time the verdict landed. The audience
 * spent the climax looking at an egress list.
 *
 * So the column is now pinned at both ends and scrolls only in the middle:
 *
 *   1. **Chamber header** — what this is, what it is doing, and the nameplate.
 *   2. **Findings deck** — fired signals. Mounted *outside* the scroller, so
 *      once a signal fires it stays on screen for the rest of the run.
 *   3. **Telemetry** — the streams, then the verdict. The only thing that
 *      scrolls, and it is parked at its tail, so the verdict lands in view
 *      under a deck that cannot move.
 *
 * The verdict deliberately stays *inside* the scroller rather than pinning to
 * the bottom as a fourth zone: pinned, three fixed bands plus a scroller have
 * to divide a fixed-height column between them, and on a short viewport the
 * telemetry got squeezed to a single orphan line between two walls of colour.
 * Scrolled-to-tail costs nothing and survives any height.
 *
 * The payoff is that at the climax the frame reads as DEMO's wireframe draws
 * it: two red findings and BLOCKED, all at once, beside a left column still
 * saying CLEAN.
 */
export function ReactorColumn({
  state,
  plannedSessions,
  active,
}: {
  state: ConsoleState;
  plannedSessions: number;
  active: boolean;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const started = state.sessions.length > 0 || state.provisioning.length > 0;

  // Keep the telemetry stream at its newest row, and always ride it to the
  // bottom when the verdict lands. Following the tail is safe now that the
  // findings deck sits outside this scroller: the catch can no longer be
  // scrolled away by the record behind it.
  const tick =
    state.sessions.length +
    state.wire.length +
    state.transcript.length +
    state.egress.length +
    state.analyst.length;
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 240;
    if (nearBottom || state.verdict) {
      el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
    }
  }, [tick, state.verdict]);

  // The containment panel carries the chamber's state on its edge alone: a
  // faintly firmer line while detonating, then the verdict — a soft success
  // tint on ALLOWED, a restrained danger tint on BLOCKED. That is the entire
  // state treatment; a SUSPICIOUS verdict is not a block, so it rests at idle
  // rather than borrowing reserved red.
  const label = state.verdict?.label;
  const bezelState: BezelState =
    label === "MALICIOUS"
      ? "blocked"
      : label === "ALLOWED"
        ? "allowed"
        : active
          ? "running"
          : "idle";

  const status: ChamberStatus =
    label === "MALICIOUS"
      ? "blocked"
      : label === "ALLOWED"
        ? "allowed"
        : label
          ? "suspicious"
          : active
            ? "running"
            : "idle";

  return (
    <section
      aria-label="Reactor detonation chamber"
      data-state={bezelState}
      className="bezel flex min-h-0 flex-col overflow-hidden"
    >
      <ChamberHeader
        chamber={state.chamber}
        artifactName={state.artifactName}
        detonationId={state.detonationId}
        sessionCount={state.sessions.length}
        plannedSessions={plannedSessions}
        status={status}
      />

      {/* --- pinned: the catch -------------------------------------------- */}
      <FindingsDeck signals={state.signals} diffs={state.diffs} />

      {/* --- scrolling: the record ----------------------------------------
          Masked at the top edge so a mid-scroll row fades out instead of being
          sliced flat against the findings deck above it. The mask only applies
          while there is something scrolled past — at rest the fade would eat
          the first line of the idle copy — so it is switched by the presence
          of a run rather than left on permanently. */}
      <div
        ref={scrollRef}
        className={cn(
          "flex min-h-0 flex-1 flex-col divide-y divide-line overflow-y-auto overflow-x-hidden",
          started && "[mask-image:linear-gradient(to_bottom,transparent,#000_1.25rem)]",
        )}
      >
        {!started && <ChamberIdle />}

        {started && state.sessions.length === 0 && <Provisioning state={state} />}

        {state.sessions.length > 0 && (
          <SessionLadder sessions={state.sessions} plannedSessions={plannedSessions} />
        )}

        {/* The full forensic record behind the deck above: the gloss, the
            evidence ids, and the complete byte ledger with the added run
            rendered inside the whole description. The deck is the headline;
            this is what a security-literate reader reads second. */}
        {state.signals.length > 0 && (
          <div className="flex flex-col gap-3 px-4 py-3">
            <SectionLabel
              right={
                <span className="telemetry tnum whitespace-nowrap">
                  {state.signals.length} fired
                </span>
              }
            >
              Evidence record
            </SectionLabel>
            {state.signals.map((sig) => (
              <div key={sig.id} className="flex flex-col gap-2.5">
                <SignalRow signal={sig} />
                {sig.type === "rug_pull" &&
                  state.diffs
                    .filter((d) => sig.evidence?.includes(d.id))
                    .map((d) => <ByteDiffCard key={d.id} diff={d} />)}
              </div>
            ))}
            {/* any diff not already shown under a signal */}
            {state.diffs
              .filter((d) => !state.signals.some((s) => s.evidence?.includes(d.id)))
              .map((d) => (
                <ByteDiffCard key={d.id} diff={d} />
              ))}
          </div>
        )}

        {state.wire.length > 0 && <WireLog lines={state.wire} />}
        {state.transcript.length > 0 && <TranscriptStrip lines={state.transcript} />}
        {state.egress.length > 0 && <EgressStrip lines={state.egress} />}
        {state.analyst.length > 0 && <AnalystTrace steps={state.analyst} />}

        {active && !state.verdict && state.sessions.length > 0 && (
          <div className="flex items-center gap-2.5 px-4 py-3" role="status">
            {/* The pulsing lamp carries the in-flight state; the word beside
                it stays neutral so only one thing on the page is tinted. */}
            <Led tone="live" size="sm" pulse />
            <span className="whitespace-nowrap text-sm text-muted">Detonating</span>
            <span className="rule" aria-hidden="true" />
            <span className="telemetry tnum whitespace-nowrap">
              {state.sessions.length}/{plannedSessions}
            </span>
          </div>
        )}

        {/* --- the last beat ---------------------------------------------- */}
        {state.verdict && (
          <div className="p-4">
            <VerdictBanner verdict={state.verdict} />
          </div>
        )}
      </div>
    </section>
  );
}

/**
 * The resting chamber. An empty instrument should read as *armed and waiting*,
 * not as a hollow rectangle with a caption floating in it — so instead of one
 * sentence the idle state shows the containment loadout: what gets planted,
 * what watches, and the guarantee that makes any of this safe to run.
 */
const CONTAINMENT: { k: string; v: string }[] = [
  { k: "Bait", v: "Decoy repo, plausible credentials and canary tokens, planted on disk" },
  { k: "Canary", v: "One token seeded only into the victim's system prompt — never written" },
  { k: "Wire", v: "A stdio proxy between victim and artifact, logging every MCP frame" },
  { k: "Sink", v: "Contained HTTP and DNS endpoint — egress is observed, never delivered" },
  { k: "Syscalls", v: "The artifact runs under strace; writes and connects become typed events" },
];

function ChamberIdle() {
  return (
    <div className="px-4 py-3.5">
      <p className="max-w-[36rem] text-sm leading-relaxed text-muted">
        Detonate to install the artifact in a disposable chamber and point a sacrificial victim
        agent at it. <span className="text-fg">The host never executes the artifact.</span>
      </p>

      <div className="mt-4">
        <SectionLabel>Chamber loadout</SectionLabel>
        <dl className="rail rail-tight mt-2.5">
          {CONTAINMENT.map((c, i) => (
            <div key={c.k} className="rail-row py-1.5">
              <div className="rail-time" aria-hidden="true">
                {String(i + 1).padStart(2, "0")}
                <span className="rail-node" />
              </div>
              <div className="flex min-w-0 flex-wrap items-baseline gap-x-2">
                <dt className="shrink-0 font-mono text-2xs text-fg">{c.k}</dt>
                <dd className="min-w-0 text-sm leading-relaxed text-muted">{c.v}</dd>
              </div>
            </div>
          ))}
        </dl>
      </div>
    </div>
  );
}

/** Provisioning checklist as a gutter rail — steps accrue, they don't stack. */
function Provisioning({ state }: { state: ConsoleState }) {
  return (
    <div className="px-4 py-3">
      <SectionLabel
        right={
          <span className="telemetry tnum whitespace-nowrap">
            {state.provisioning.length} steps
          </span>
        }
      >
        Provisioning chamber
      </SectionLabel>
      <ol className="rail rail-tight mt-3">
        {state.provisioning.map((p, i) => {
          const last = i === state.provisioning.length - 1;
          return (
            <li key={p.phase} className="rail-row animate-tick-in py-2">
              <div className="rail-time">
                {String(i + 1).padStart(2, "0")}
                <span
                  className="rail-node"
                  data-tone={last ? "live" : undefined}
                  aria-hidden="true"
                />
              </div>
              <div className="flex min-w-0 items-center gap-2.5">
                <span className="flex w-3.5 shrink-0 justify-center" aria-hidden="true">
                  {last ? (
                    <Loader2 size={13} className="animate-spin-slow text-live" />
                  ) : (
                    <Check size={13} className="text-success" />
                  )}
                </span>
                <span className={cn("min-w-0 truncate text-sm", last ? "text-fg" : "text-muted")}>
                  {p.message}
                </span>
              </div>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
