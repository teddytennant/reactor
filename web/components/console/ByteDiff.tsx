"use client";

import { GitCompareArrows } from "lucide-react";
import type { ByteDiff } from "@/lib/reducer";
import { StaticBlindBadge } from "@/components/ui";

/**
 * The byte diff — one of the two carry-the-demo moments (DEMO §1). Deliberately
 * large and unmissable: the exact added substring is highlighted inside the full
 * description so a security-literate judge sees the rug pull as bytes, not prose.
 */
export function ByteDiffCard({ diff }: { diff: ByteDiff }) {
  return (
    <div className="animate-signal-snap rounded-xl border border-danger/40 bg-danger/[0.05] p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <GitCompareArrows size={15} className="text-danger" />
          <span className="font-mono text-[13px] font-medium text-fg">{diff.tool}</span>
          <span className="text-xs text-muted">description mutated</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="font-mono text-2xs text-faint">
            deton. {diff.fromSession} → {diff.toSession}
          </span>
          <StaticBlindBadge />
        </div>
      </div>

      {/* the delta, big */}
      <div className="mt-3 flex items-baseline gap-2">
        <span className="tnum text-3xl font-semibold leading-none tracking-tight text-danger">
          +{diff.delta}
        </span>
        <span className="text-sm font-medium text-danger/80">bytes</span>
        <span className="ml-1 font-mono text-2xs text-faint">
          {diff.fromBytes}B → {diff.toBytes}B
        </span>
      </div>

      {/* rendered diff — prefix muted, added run highlighted */}
      <div className="mt-3 overflow-x-auto rounded-lg border border-line bg-bg/60 p-3">
        <div className="min-w-max font-mono text-[13.5px] leading-relaxed">
          <span className="text-muted">{diff.prefix}</span>
          {diff.removed && (
            <span className="rounded bg-faint/15 px-0.5 text-faint line-through decoration-faint/60">
              {diff.removed}
            </span>
          )}
          <span className="rounded bg-danger/20 px-0.5 font-medium text-danger ring-1 ring-inset ring-danger/30">
            {diff.added}
          </span>
          <span className="text-muted">{diff.suffix}</span>
        </div>
      </div>

      <div className="mt-2.5 flex items-center gap-2">
        <span className="font-mono text-2xs text-faint">added</span>
        <code className="rounded-md bg-danger/15 px-2 py-1 font-mono text-xs font-medium text-danger">
          {diff.added.trim()}
        </code>
      </div>
    </div>
  );
}
