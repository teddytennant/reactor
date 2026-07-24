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

/**
 * The last beat of the demo, and the only element on the page that legitimately
 * floats — a soft, wide, low-opacity elevation, not a hue halo. Decisive by
 * scale and by one earned hue: an icon in a tinted tile, one very large word,
 * the family beside it, then the evidence. BLOCKED and ALLOWED never rely on
 * color alone — the icon, the tile and the word carry it.
 */
export function VerdictBanner({ verdict }: { verdict: Verdict }) {
  const blocked = verdict.label === "MALICIOUS" || verdict.label === "SUSPICIOUS";
  const label = displayLabel(verdict.label);
  const Icon = blocked ? ShieldX : ShieldCheck;
  const toneText = blocked ? "text-danger" : "text-success";

  return (
    <div
      className={cn(
        "panel-float animate-verdict-in relative overflow-hidden",
        blocked ? "border-danger/30 bg-danger/[0.05]" : "border-success/30 bg-success/[0.04]",
      )}
    >
      <div className="px-5 py-5">
        <div className="flex items-center gap-2.5">
          <span className="strip-label whitespace-nowrap">Verdict</span>
          <span className="rule" aria-hidden="true" />
        </div>

        <div className="mt-3.5 flex flex-wrap items-center gap-x-3.5 gap-y-2">
          <span
            aria-hidden="true"
            className={cn(
              "flex h-11 w-11 shrink-0 items-center justify-center rounded-xl",
              blocked ? "bg-danger/15" : "bg-success/15",
              toneText,
            )}
          >
            <Icon size={22} strokeWidth={2} />
          </span>
          <span
            className={cn(
              "text-3xl font-semibold leading-none tracking-tight sm:text-4xl",
              toneText,
            )}
          >
            {label}
          </span>
          <span className="flex items-baseline gap-2 text-sm text-muted">
            <span className="text-faint" aria-hidden="true">
              ·
            </span>
            {familyLabel(verdict.family)}
          </span>
        </div>

        <p className="mt-3.5 max-w-prose text-base leading-relaxed text-muted">
          {verdict.explanation}
        </p>

        <div className="mt-4 flex flex-col gap-2 border-t border-line pt-3.5">
          {verdict.evidence?.length ? (
            <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
              <span className="text-xs text-faint">Evidence</span>
              <EvidenceIds ids={verdict.evidence} />
            </div>
          ) : null}
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs text-faint">
            <span>
              verdict in{" "}
              <span className="tnum font-mono text-2xs text-muted">
                {(verdict.time_to_verdict_ms / 1000).toFixed(1)}s
              </span>
            </span>
            <span aria-hidden="true">·</span>
            <span>
              <span className="tnum font-mono text-2xs text-muted">{verdict.sessions}</span>{" "}
              detonations
            </span>
            <span aria-hidden="true">·</span>
            <span className="tnum font-mono text-2xs text-muted">
              ${verdict.cost_usd.toFixed(3)}
            </span>
            {verdict.analyst && (
              <>
                <span aria-hidden="true">·</span>
                <span>
                  analyst <span className="font-mono text-2xs text-muted">{verdict.analyst}</span>
                  {verdict.fallback && <span className="text-warning"> (deterministic)</span>}
                </span>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
