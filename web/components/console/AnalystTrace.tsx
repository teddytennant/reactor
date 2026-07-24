"use client";

import type { AnalystStep } from "@/lib/events";

/**
 * The analyst's investigative loop (SPEC §4.5). Rendered deliberately calm and
 * secondary: the analyst reads typed evidence only, never attacker prose, so it
 * is "ours" and boring by construction (SPEC §10 safeguards). A quiet numbered
 * gutter rail — sans prose for the thought, mono for the tool call and its
 * typed result. No color at all: nothing fired here.
 */
export function AnalystTrace({ steps }: { steps: AnalystStep[] }) {
  if (steps.length === 0) return null;
  const model = steps[0]?.model ?? "analyst";
  return (
    <div className="px-4 py-3">
      <div className="flex items-center gap-2.5">
        <span className="strip-label whitespace-nowrap">Analyst loop</span>
        <span className="rule" aria-hidden="true" />
        <span className="flex min-w-0 items-baseline gap-1.5 truncate">
          <span className="truncate font-mono text-2xs text-faint">{model}</span>
          <span className="whitespace-nowrap text-xs text-faint">
            · reads typed evidence only
          </span>
        </span>
      </div>

      <ol className="rail rail-tight mt-3">
        {steps.map((s) => (
          <li key={s.step} className="rail-row animate-fade-in py-2.5">
            <div className="rail-time">
              {s.step}
              <span className="rail-node" />
            </div>
            <div className="min-w-0">
              {s.thought && <p className="text-sm leading-relaxed text-muted">{s.thought}</p>}
              {(s.tool || s.result) && (
                <div className="mt-1.5 flex flex-wrap items-baseline gap-x-2 gap-y-1 font-mono text-2xs">
                  {s.tool && <span className="font-medium text-fg">{s.tool}()</span>}
                  {s.result && <span className="min-w-0 text-faint">{s.result}</span>}
                </div>
              )}
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}
