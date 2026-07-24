"use client";

import { Check, ShieldCheck, Terminal } from "lucide-react";
import { cn } from "@/lib/cn";
import type { ScanLine, ScanResult } from "@/lib/events";

export function ScanColumn({
  artifactName,
  scanLines,
  result,
  active,
}: {
  artifactName: string;
  scanLines: ScanLine[];
  result: ScanResult | null;
  active: boolean;
}) {
  const body = scanLines.filter((l) => !l.done);
  return (
    <section className="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-line bg-surface">
      {/* header */}
      <div className="flex items-center justify-between border-b border-line bg-surface-2 px-4 py-3">
        <div>
          <div className="text-2xs font-semibold uppercase tracking-[0.14em] text-faint">
            Industry standard
          </div>
          <div className="mt-0.5 flex items-center gap-1.5 text-[13px] font-medium text-fg">
            <Terminal size={14} className="text-muted" />
            mcp-scan
            <span className="text-faint">·</span>
            <span className="font-normal text-muted">Snyk / ex-Invariant Labs</span>
          </div>
        </div>
        <span className="rounded-md border border-line px-2 py-1 font-mono text-2xs text-faint">
          static
        </span>
      </div>

      {/* terminal body */}
      <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto px-4 py-3.5 font-mono text-[12.5px] leading-relaxed">
        <div className="text-muted">
          <span className="text-faint">$ </span>
          mcp-scan {artifactName}
        </div>
        {body.length === 0 && active && (
          <div className="text-faint">
            scanning tool descriptions
            <span className="animate-pulse-dot">…</span>
          </div>
        )}
        {body.map((line, i) => {
          const isResult = line.stream === "result";
          return (
            <div
              key={i}
              className={cn(
                "flex animate-fade-slide-up items-start gap-2",
                isResult ? "text-fg" : "text-faint",
              )}
            >
              {isResult ? (
                <Check size={14} className="mt-px shrink-0 text-success" />
              ) : (
                <span className="w-3.5 shrink-0" />
              )}
              <span className="whitespace-pre-wrap">{line.text}</span>
            </div>
          );
        })}
      </div>

      {/* verdict footer */}
      <div className="border-t border-line p-4">
        {result ? (
          <div className="flex animate-fade-slide-up items-center gap-3 rounded-xl border border-success/30 bg-success/[0.07] px-4 py-3.5">
            <div className="grid h-10 w-10 place-items-center rounded-lg bg-success/15 text-success">
              <ShieldCheck size={22} />
            </div>
            <div>
              <div className="text-lg font-semibold tracking-tight text-success">CLEAN</div>
              <div className="text-xs text-muted tnum">
                {result.issues} issues found · read {countTools(scanLines)} tool descriptions
              </div>
            </div>
          </div>
        ) : (
          <div className="flex h-[62px] items-center gap-3 rounded-xl border border-dashed border-line px-4 text-xs text-faint">
            {active ? "awaiting scanner result…" : "idle"}
          </div>
        )}
      </div>
    </section>
  );
}

function countTools(lines: ScanLine[]): number {
  return lines.filter((l) => l.stream === "result" && !l.done).length || 3;
}
