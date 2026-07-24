"use client";

import { Cpu, FlaskConical, TriangleAlert } from "lucide-react";
import type { ChamberInfo } from "@/lib/events";

export function ChamberHeader({ chamber }: { chamber: ChamberInfo | null }) {
  return (
    <div className="border-b border-line bg-surface-2 px-4 py-3">
      <div className="flex items-center justify-between gap-2">
        <div className="text-2xs font-semibold uppercase tracking-[0.14em] text-accent">Reactor</div>
        {chamber?.simulated ? (
          <span className="inline-flex items-center gap-1 rounded-md border border-warning/40 bg-warning/10 px-1.5 py-0.5 text-2xs font-semibold uppercase tracking-wide text-warning">
            <TriangleAlert size={11} />
            Simulated victim
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 rounded-md border border-line px-1.5 py-0.5 text-2xs font-medium text-faint">
            <FlaskConical size={11} />
            Detonation chamber
          </span>
        )}
      </div>

      <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 font-mono text-[12.5px]">
        {chamber ? (
          <>
            <span className="text-muted">
              Daytona sandbox <span className="text-fg">{chamber.sandbox_id}</span>
            </span>
            <span className="text-faint">·</span>
            <span className="inline-flex items-center gap-1 text-muted">
              <Cpu size={12} className="text-faint" />
              {chamber.gpu}
            </span>
          </>
        ) : (
          <span className="text-faint">chamber not provisioned</span>
        )}
      </div>

      {chamber && (
        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 font-mono text-2xs text-faint">
          <span>
            victim <span className="text-muted">{shortModel(chamber.model)}</span>
          </span>
          <span>·</span>
          <span>{chamber.served}</span>
          <span>·</span>
          <span>t={chamber.temp}</span>
          <span>·</span>
          <span>seed {chamber.seed}</span>
          <span>·</span>
          <span title={`revision ${chamber.revision}`}>rev {chamber.revision.slice(0, 7)}</span>
        </div>
      )}
    </div>
  );
}

function shortModel(model: string): string {
  const i = model.lastIndexOf("/");
  return i >= 0 ? model.slice(i + 1) : model;
}
