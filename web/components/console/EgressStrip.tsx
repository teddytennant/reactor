"use client";

import { Globe, Radar } from "lucide-react";
import { cn } from "@/lib/cn";
import type { EgressLine } from "@/lib/reducer";

const OP_LABEL: Record<string, string> = {
  egress_http: "http",
  egress_dns: "dns",
  connect: "connect",
  file_read: "read",
  file_open: "open",
};

/**
 * Behavioral / contained-egress strip. Everything hits the mock sink, nothing
 * reaches the real internet. Benign traffic to an allowlisted host reads calm;
 * the one line carrying a canary is the gasp — a token that was never on disk.
 */
export function EgressStrip({ lines }: { lines: EgressLine[] }) {
  return (
    <div className="px-4 py-3">
      <div className="strip-label mb-2 flex items-center gap-1.5">
        <Radar size={12} className="text-faint" />
        Contained egress · mock sink
      </div>
      <div className="flex flex-col gap-1 font-mono text-2xs">
        {lines.length === 0 && <div className="text-faint">no egress observed</div>}
        {lines.map((l) => {
          const canary = l.canaries && l.canaries.length > 0 ? l.canaries[0] : null;
          const kind = l.canaryKinds && l.canaryKinds.length > 0 ? l.canaryKinds[0] : null;
          return l.malicious ? (
            <div
              key={l.id}
              className="animate-signal-snap rounded-lg border border-danger/40 bg-danger/[0.07] p-2.5"
            >
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <Globe size={12} className="text-danger" />
                <span className="text-danger">{OP_LABEL[l.op] ?? l.op}</span>
                <span className="text-fg">{l.method}</span>
                <span className="text-muted">{l.host}</span>
                {l.urlPath && <span className="text-faint">{l.urlPath}</span>}
                {typeof l.bodyBytes === "number" && (
                  <span className="ml-auto text-faint tnum">{l.bodyBytes}B</span>
                )}
              </div>
              {canary && (
                <div className="mt-1.5 flex flex-wrap items-center gap-2">
                  <code className="rounded-md bg-danger/15 px-2 py-0.5 text-xs font-semibold text-danger">
                    {canary}
                  </code>
                  <span className="text-2xs text-danger/85">
                    {kind === "context"
                      ? "→ sink · canary was never on disk, only in the agent's system prompt"
                      : "→ sink"}
                  </span>
                </div>
              )}
            </div>
          ) : (
            <div key={l.id} className="flex animate-fade-in items-center gap-2 rounded px-1.5 py-1">
              <span className="w-9 shrink-0 text-faint">d{l.session}</span>
              <span className="w-3 shrink-0 text-success/70">✓</span>
              <span className="shrink-0 text-muted">{OP_LABEL[l.op] ?? l.op}</span>
              {l.method && <span className="text-faint">{l.method}</span>}
              <span className="truncate text-faint">{l.host}</span>
              <span className="ml-auto shrink-0 rounded bg-surface-2 px-1 text-2xs text-faint">
                allowlisted
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
