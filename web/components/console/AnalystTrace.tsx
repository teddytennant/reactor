"use client";

import { Brain } from "lucide-react";
import type { AnalystStep } from "@/lib/events";

/**
 * The analyst's investigative loop (SPEC §4.5). Rendered deliberately calm and
 * secondary: the analyst reads typed evidence only, never attacker prose, so it
 * is "ours" and boring by construction (SPEC §10 safeguards).
 */
export function AnalystTrace({ steps }: { steps: AnalystStep[] }) {
  if (steps.length === 0) return null;
  const model = steps[0]?.model ?? "analyst";
  return (
    <div className="px-4 py-3">
      <div className="strip-label mb-2 flex items-center gap-1.5">
        <Brain size={12} className="text-faint" />
        Analyst loop
        <span className="font-mono text-2xs normal-case tracking-normal text-faint">
          · {model} · reads typed evidence only
        </span>
      </div>
      <ol className="flex flex-col gap-1.5">
        {steps.map((s) => (
          <li key={s.step} className="animate-fade-in flex gap-2.5">
            <span className="mt-0.5 grid h-4 w-4 shrink-0 place-items-center rounded-full border border-line font-mono text-[9px] text-faint">
              {s.step}
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-xs leading-snug text-muted">{s.thought}</p>
              {(s.tool || s.result) && (
                <div className="mt-0.5 flex flex-wrap items-center gap-2 font-mono text-2xs">
                  {s.tool && <span className="text-accent">{s.tool}()</span>}
                  {s.result && <span className="text-faint">{s.result}</span>}
                </div>
              )}
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}
