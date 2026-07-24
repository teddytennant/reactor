"use client";

import { GitCompareArrows } from "lucide-react";
import type { ByteDiff } from "@/lib/reducer";
import { StaticBlindBadge } from "@/components/ui";

/**
 * The byte diff — one of the two carry-the-demo moments (DEMO §1), and the
 * single most important piece of evidence on the page. It is built to read as
 * forensic proof, not as a callout: a tabular byte ledger, then the exact added
 * run rendered inside the full description so a security-literate judge sees
 * the rug pull as *bytes*, not prose.
 *
 * Added bytes are the malicious payload, so this is one of the very few places
 * `danger` is earned (DESIGN §1) — but it is spent as ink, never as chrome: no
 * card, no red outline, no red wash (DESIGN §2.4). The diff hangs under its
 * signal on the chamber's own fill, indented to the signal's text column; the
 * only box on it is the neutral well the description is rendered into. Colour
 * is still never alone: the added run is highlighted and labelled, the removed
 * run is struck through, and the delta is spelled out in words. The numbers are
 * machine data and stay mono and tabular; every label around them is
 * sentence-case sans.
 */
export function ByteDiffCard({ diff }: { diff: ByteDiff }) {
  const sign = diff.delta > 0 ? "+" : "";

  return (
    <figure className="animate-fade-slide-up pl-7">
      {/* --- identity ------------------------------------------------------- */}
      <figcaption className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <GitCompareArrows size={14} className="shrink-0 text-danger" aria-hidden="true" />
        <span className="text-sm font-medium text-danger">Tool description mutated</span>
        <span className="min-w-0 truncate font-mono text-xs font-medium text-fg">
          {diff.tool}
        </span>
        <span className="ml-auto inline-flex shrink-0 items-center gap-2.5">
          <span className="tnum font-mono text-2xs text-faint">
            deton. {diff.fromSession} → {diff.toSession}
          </span>
          <StaticBlindBadge />
        </span>
      </figcaption>

      <div className="mt-3">
        {/* --- the byte ledger --------------------------------------------- */}
        <div className="flex flex-wrap items-end gap-x-5 gap-y-2">
          <div className="flex items-baseline gap-2">
            <span className="tnum font-mono text-4xl font-semibold leading-none text-danger">
              {sign}
              {diff.delta}
            </span>
            <span className="text-sm font-medium text-muted">bytes</span>
          </div>

          <div className="flex items-baseline gap-x-2 pb-0.5">
            <span className="text-xs text-muted">Before</span>
            <span className="tnum font-mono text-2xs text-muted">{diff.fromBytes}B</span>
            <span className="text-faint" aria-hidden="true">
              →
            </span>
            <span className="text-xs text-muted">After</span>
            <span className="tnum font-mono text-2xs font-medium text-fg">
              {diff.toBytes}B
            </span>
          </div>
        </div>

        {/* --- legend: colour is always paired with a word ------------------ */}
        <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-1.5">
          <span className="inline-flex items-center gap-1.5">
            <span className="h-2.5 w-2.5 rounded-sm bg-danger/40" aria-hidden="true" />
            <span className="text-xs text-danger">Added</span>
          </span>
          {diff.removed && (
            <span className="inline-flex items-center gap-1.5">
              <span className="h-2.5 w-2.5 rounded-sm bg-faint/40" aria-hidden="true" />
              <span className="text-xs text-muted">Removed</span>
            </span>
          )}
          <span className="rule" aria-hidden="true" />
          <span className="text-xs text-muted">Rendered description</span>
        </div>

        {/* --- the rendered diff: an inset well, wrapped, never wider than
                the column ------------------------------------------------- */}
        <div className="mt-2 max-h-64 overflow-y-auto rounded-xl bg-surface-3 p-3.5">
          <p className="whitespace-pre-wrap break-words font-mono text-[13px] leading-[1.6] text-muted">
            {diff.prefix}
            {diff.removed && (
              <span className="rounded bg-faint/15 px-0.5 text-faint line-through decoration-faint/60 decoration-2">
                {diff.removed}
              </span>
            )}
            <span className="rounded bg-danger/[0.16] px-0.5 font-semibold text-danger">
              {diff.added}
            </span>
            {diff.suffix}
          </p>
        </div>
      </div>
    </figure>
  );
}
