"use client";

import { ShieldCheck, ShieldX } from "lucide-react";
import { cn } from "@/lib/cn";
import type { Verdict } from "@/lib/events";
import { familyLabel } from "@/lib/signals";
import { EvidenceIds } from "@/components/ui";

function displayLabel(label: string): string {
  if (label === "MALICIOUS") return "BLOCKED";
  if (label === "ALLOWED") return "ALLOWED";
  if (label === "SUSPICIOUS") return "SUSPECT";
  return label;
}

export function VerdictBanner({ verdict }: { verdict: Verdict }) {
  const blocked = verdict.label === "MALICIOUS" || verdict.label === "SUSPICIOUS";
  const label = displayLabel(verdict.label);
  const tone = blocked
    ? { border: "border-danger/45", bg: "bg-danger/[0.08]", text: "text-danger", chip: "bg-danger text-white", glow: "shadow-glow-danger" }
    : { border: "border-success/40", bg: "bg-success/[0.07]", text: "text-success", chip: "bg-success text-white", glow: "shadow-glow-success" };
  const Icon = blocked ? ShieldX : ShieldCheck;

  return (
    <div className={cn("animate-verdict-in rounded-2xl border p-4 sm:p-5", tone.border, tone.bg, tone.glow)}>
      <div className="flex items-center gap-3.5">
        <span className={cn("grid h-12 w-12 shrink-0 place-items-center rounded-xl", blocked ? "bg-danger/15" : "bg-success/15", tone.text)}>
          <Icon size={26} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className={cn("text-2xl font-bold tracking-tight sm:text-[26px]", tone.text)}>
              {label}
            </span>
            <span
              className={cn(
                "rounded-md px-2 py-0.5 text-xs font-semibold uppercase tracking-wide",
                tone.chip,
              )}
            >
              {familyLabel(verdict.family)}
            </span>
          </div>
          <p className="mt-1.5 text-sm leading-snug text-muted">{verdict.explanation}</p>
        </div>
      </div>

      <div className="mt-3.5 flex flex-col gap-2 border-t border-line/70 pt-3">
        <div className="flex items-start justify-between gap-3">
          <span className="text-2xs uppercase tracking-wide text-faint">Evidence</span>
          <EvidenceIds ids={verdict.evidence} tone={blocked ? "danger" : "success"} />
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 font-mono text-2xs text-faint">
          <span>
            verdict in <span className="text-muted tnum">{(verdict.time_to_verdict_ms / 1000).toFixed(1)}s</span>
          </span>
          <span>·</span>
          <span>
            <span className="text-muted tnum">{verdict.sessions}</span> detonations
          </span>
          <span>·</span>
          <span>
            <span className="text-muted tnum">${verdict.cost_usd.toFixed(3)}</span>
          </span>
          {verdict.analyst && (
            <>
              <span>·</span>
              <span>
                analyst <span className="text-muted">{verdict.analyst}</span>
                {verdict.fallback && <span className="text-warning"> (deterministic)</span>}
              </span>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
