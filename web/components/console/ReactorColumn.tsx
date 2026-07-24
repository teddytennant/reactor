"use client";

import { useEffect, useRef } from "react";
import { Check, Loader2 } from "lucide-react";
import type { ConsoleState } from "@/lib/reducer";
import { ChamberHeader } from "./ChamberHeader";
import { SessionLadder } from "./SessionLadder";
import { ByteDiffCard } from "./ByteDiff";
import { WireLog } from "./WireLog";
import { TranscriptStrip } from "./TranscriptStrip";
import { EgressStrip } from "./EgressStrip";
import { SignalRow } from "./SignalRow";
import { AnalystTrace } from "./AnalystTrace";
import { VerdictBanner } from "./VerdictBanner";

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

  return (
    <section className="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-line bg-surface">
      <ChamberHeader chamber={state.chamber} />

      <div ref={scrollRef} className="flex min-h-0 flex-1 flex-col overflow-y-auto divide-y divide-line/70">
        {!started && (
          <div className="grid flex-1 place-items-center px-6 py-16 text-center">
            <div className="max-w-xs">
              <p className="text-sm text-muted">Chamber idle.</p>
              <p className="mt-1 text-xs text-faint">
                Select an artifact and detonate to point the sacrificial victim agent at it.
              </p>
            </div>
          </div>
        )}

        {started && state.sessions.length === 0 && <Provisioning state={state} />}

        {state.sessions.length > 0 && (
          <SessionLadder sessions={state.sessions} plannedSessions={plannedSessions} />
        )}

        {state.signals.length > 0 && (
          <div className="flex flex-col gap-2 px-4 py-3">
            <div className="strip-label">Signals</div>
            {state.signals.map((sig) => (
              <div key={sig.id} className="flex flex-col gap-2">
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
          <div className="flex items-center gap-2 px-4 py-3 text-2xs text-faint">
            <Loader2 size={12} className="animate-spin-slow" />
            detonating…
          </div>
        )}
      </div>
    </section>
  );
}

function Provisioning({ state }: { state: ConsoleState }) {
  return (
    <div className="px-4 py-3">
      <div className="strip-label mb-2">Provisioning chamber</div>
      <ol className="flex flex-col gap-1.5">
        {state.provisioning.map((p, i) => {
          const last = i === state.provisioning.length - 1;
          return (
            <li key={p.phase} className="flex animate-fade-in items-center gap-2.5 text-xs">
              {last ? (
                <Loader2 size={13} className="animate-spin-slow text-accent" />
              ) : (
                <Check size={13} className="text-success" />
              )}
              <span className={last ? "text-fg" : "text-muted"}>{p.message}</span>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
