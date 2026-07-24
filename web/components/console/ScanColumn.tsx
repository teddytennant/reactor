"use client";

import { Check, ShieldCheck, Terminal } from "lucide-react";
import { cn } from "@/lib/cn";
import type { ScanLine, ScanResult } from "@/lib/events";
import { Led } from "@/components/ui";

/**
 * The mcp-scan column (DESIGN §3). Deliberately the quieter half: flat panel,
 * no bezel, no bloom, no texture, lower-contrast ink. The header states what
 * the tool is and what it does not do — reading descriptions, never running
 * the code — because that contrast is the entire reason this column sits next
 * to Reactor. Its only in-flight affordance is the quiet lamp in the footer
 * (DESIGN §2.5); there is no edge sweep, no glow and no ornament.
 */
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
    <section className="panel-flat flex min-h-0 flex-col overflow-hidden bg-surface/70">
      {/* header — the column's whole argument in two lines, no eyebrow, no
          vendor trivia, no floating status chip */}
      <div className="shrink-0 border-b border-line px-5 py-4">
        <span className="inline-flex items-center gap-2 font-mono text-sm font-medium text-fg">
          <Terminal size={14} className="text-faint" aria-hidden="true" />
          mcp-scan
        </span>
        <p className="mt-1.5 text-sm leading-relaxed text-muted">
          A static scanner. It reads the artifact&rsquo;s tool descriptions and never runs the
          code.
        </p>
      </div>

      {/* terminal body — a gutter rail of console output, not a bulleted list */}
      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
        <div className="rail rail-tight">
          <div className="rail-row py-1.5">
            <div className="rail-time" aria-hidden="true">
              $
            </div>
            <div className="break-words font-mono text-xs text-muted">
              mcp-scan {artifactName}
            </div>
          </div>

          {body.length === 0 && active && (
            <div className="rail-row py-1.5">
              <div className="rail-time" aria-hidden="true">
                ··
              </div>
              <div className="font-mono text-xs text-faint">
                scanning tool descriptions
                <span className="animate-pulse-dot">…</span>
              </div>
            </div>
          )}

          {body.map((line, i) => {
            const isResult = line.stream === "result";
            return (
              <div key={i} className="rail-row animate-tick-in py-1.5">
                <div className="rail-time" aria-hidden="true">
                  {String(i + 1).padStart(2, "0")}
                  {isResult && <span className="rail-node" />}
                </div>
                <div
                  className={cn(
                    "flex min-w-0 items-start gap-1.5 font-mono text-xs",
                    isResult ? "text-muted" : "text-faint",
                  )}
                >
                  {isResult && (
                    <Check size={12} className="mt-[3px] shrink-0 text-success" aria-hidden="true" />
                  )}
                  <span className="min-w-0 whitespace-pre-wrap break-words">{line.text}</span>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* verdict footer — the payoff the right column is about to demolish */}
      <div className="shrink-0 border-t border-line px-5 py-4">
        {result ? (
          // No box. CLEAN is a genuine status stamp so it earns uppercase, but
          // it reads as green *ink* at label scale — a quiet, confident verdict
          // — with spacing and the icon doing the separating (DESIGN §2.4).
          <div className="flex animate-fade-slide-up items-center gap-3">
            <ShieldCheck size={20} className="shrink-0 text-success" aria-hidden="true" />
            <div className="flex min-w-0 flex-wrap items-baseline gap-x-2.5 gap-y-0.5">
              <span className="text-sm font-semibold uppercase tracking-label-wide text-success">
                CLEAN
              </span>
              <span className="text-sm text-muted">
                <span className="tnum">{result.issues}</span> issues found · read{" "}
                <span className="tnum">{countTools(scanLines)}</span> tool descriptions
              </span>
            </div>
          </div>
        ) : (
          <div className="flex items-center gap-2.5 text-sm text-muted">
            <Led tone={active ? "live" : "neutral"} size="sm" pulse={active} />
            {active ? "Awaiting scanner result…" : "Idle"}
          </div>
        )}
      </div>
    </section>
  );
}

function countTools(lines: ScanLine[]): number {
  return lines.filter((l) => l.stream === "result" && !l.done).length || 3;
}
