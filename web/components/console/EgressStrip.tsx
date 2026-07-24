"use client";

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
 * reaches the real internet. A gutter-rail timeline: benign traffic to an
 * allowlisted host is a quiet bead, the line carrying a canary gets a danger
 * node and the caught token on its own line underneath — the only red in the
 * strip, so it is unmistakable without any badge around it. Hosts, methods and
 * byte counts are mono machine data; the sentence explaining the catch is sans
 * prose. "contained" stays neutral: this is the sink doing its job, not an
 * error.
 */
export function EgressStrip({ lines }: { lines: EgressLine[] }) {
  const caught = lines.filter((l) => l.malicious).length;

  return (
    <div className="px-4 py-3">
      <div className="flex items-center gap-2.5">
        <span className="strip-label whitespace-nowrap">Contained egress · mock sink</span>
        <span className="rule" aria-hidden="true" />
        <span
          className={cn(
            "tnum whitespace-nowrap text-xs",
            caught > 0 ? "font-medium text-danger" : "text-faint",
          )}
        >
          {caught > 0 ? `${caught} caught` : `${lines.length} observed`}
        </span>
      </div>

      <div className="rail rail-tight mt-3">
        {lines.length === 0 && (
          <div className="rail-row py-2">
            <div className="rail-time">
              <span className="rail-node" />
            </div>
            <div className="text-sm text-faint">no egress observed</div>
          </div>
        )}
        {lines.map((l) => {
          const canary = l.canaries && l.canaries.length > 0 ? l.canaries[0] : null;
          const kind = l.canaryKinds && l.canaryKinds.length > 0 ? l.canaryKinds[0] : null;
          return l.malicious ? (
            <div key={l.id} className="rail-row animate-fade-in py-2">
              <div className="rail-time">
                d{l.session}
                <span className="rail-node" data-tone="danger" />
              </div>
              <div className="min-w-0">
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1 font-mono text-2xs">
                  <span className="shrink-0 font-medium text-danger">
                    {OP_LABEL[l.op] ?? l.op}
                  </span>
                  <span className="text-fg">{l.method}</span>
                  <span className="text-muted">{l.host}</span>
                  {l.urlPath && <span className="text-faint">{l.urlPath}</span>}
                  {typeof l.bodyBytes === "number" && (
                    <span className="tnum ml-auto shrink-0 text-faint">{l.bodyBytes}B</span>
                  )}
                  <span className="shrink-0 font-sans text-xs text-faint">contained</span>
                </div>

                {canary && (
                  <div className="mt-1.5 flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <code className="rounded-md bg-danger/10 px-1.5 py-0.5 font-mono text-2xs font-semibold text-danger">
                      {canary}
                    </code>
                    <span className="text-xs leading-relaxed text-muted">
                      {kind === "context"
                        ? "→ sink · canary was never on disk, only in the agent's system prompt"
                        : "→ sink"}
                    </span>
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div key={l.id} className="rail-row animate-fade-in py-2">
              <div className="rail-time">
                d{l.session}
                <span className="rail-node" />
              </div>
              <div className="flex min-w-0 items-baseline gap-2 font-mono text-2xs">
                <span className="shrink-0 text-muted">{OP_LABEL[l.op] ?? l.op}</span>
                {l.method && <span className="shrink-0 text-faint">{l.method}</span>}
                <span className="truncate text-muted">{l.host}</span>
                <span className="ml-auto shrink-0 font-sans text-xs text-faint">allowlisted</span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
