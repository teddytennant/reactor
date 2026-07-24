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
import { AnalystTrace } from "./AnalystTrace";
import { VerdictBanner } from "./VerdictBanner";

/** Containment-panel edge tint (DESIGN §2.1): calm, then the verdict. */
type BezelState = "idle" | "running" | "blocked" | "allowed";

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

  // Auto-scroll to the newest evidence so the money shot lands in view.
  const tick =
    state.sessions.length +
    state.signals.length * 10 +
    state.diffs.length * 5 +
    state.wire.length +
    state.egress.length +
    (state.verdict ? 1000 : 0);
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
      {/* One contained surface: the header sits on the same fill as the body,
          separated by a hairline rather than by a nested box. */}
      <div className="flex min-h-0 flex-1 flex-col">
        <ChamberHeader
          chamber={state.chamber}
          artifactName={state.artifactName}
          detonationId={state.detonationId}
          sessionCount={state.sessions.length}
          plannedSessions={plannedSessions}
          status={status}
        />

        <div
          ref={scrollRef}
          className="flex min-h-0 flex-1 flex-col divide-y divide-line overflow-y-auto overflow-x-hidden"
        >
          {!started && <ChamberIdle />}

          {started && state.sessions.length === 0 && <Provisioning state={state} />}

          {state.sessions.length > 0 && (
            <SessionLadder sessions={state.sessions} plannedSessions={plannedSessions} />
          )}

          {state.signals.length > 0 && (
            <div className="flex flex-col gap-3 px-4 py-3">
              <SectionLabel
                right={
                  <span className="telemetry tnum whitespace-nowrap">
                    {state.signals.length} fired
                  </span>
                }
              >
                Signals
              </SectionLabel>
              {state.signals.map((sig) => (
                <div key={sig.id} className="flex flex-col gap-2.5">
                  <SignalRow signal={sig} />
                  {sig.type === "rug_pull" &&
                    state.diffs
                      .filter((d) => sig.evidence.includes(d.id))
                      .map((d) => <ByteDiffCard key={d.id} diff={d} />)}
                </div>
              ))}
              {/* any diff not already shown under a signal */}
              {state.diffs
                .filter((d) => !state.signals.some((s) => s.evidence.includes(d.id)))
                .map((d) => (
                  <ByteDiffCard key={d.id} diff={d} />
                ))}
            </div>
          )}

          {state.wire.length > 0 && <WireLog lines={state.wire} />}
          {state.transcript.length > 0 && <TranscriptStrip lines={state.transcript} />}
          {state.egress.length > 0 && <EgressStrip lines={state.egress} />}
          {state.analyst.length > 0 && <AnalystTrace steps={state.analyst} />}

          {state.verdict && (
            <div className="p-4">
              <VerdictBanner verdict={state.verdict} />
            </div>
          )}

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
        </div>
      </div>
    </section>
  );
}

/**
 * Resting chamber: one short line, sized to itself. No vertical centring and
 * no `flex-1` spacer — an empty instrument should read as compact and calm,
 * not as a hollow rectangle with a caption floating in the middle of it.
 */
function ChamberIdle() {
  return (
    <div className="px-4 py-3.5">
      <p className="max-w-[34rem] text-sm leading-relaxed text-muted">
        Select an artifact and detonate to point the sacrificial victim agent at it.
      </p>
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
