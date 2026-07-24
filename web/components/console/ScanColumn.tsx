"use client";

import { Check, EyeOff, ShieldCheck, Terminal } from "lucide-react";
import { cn } from "@/lib/cn";
import type { ScanLine, ScanResult } from "@/lib/events";
import { Led, SectionLabel } from "@/components/ui";

/**
 * The mcp-scan column (DESIGN §3). Deliberately the quieter half: flat panel,
 * no bezel, no bloom, no texture, lower-contrast ink. The header states what
 * the tool is and what it does not do — reading descriptions, never running
 * the code — because that contrast is the entire reason this column sits next
 * to Reactor. Its only in-flight affordance is the quiet lamp in the footer
 * (DESIGN §2.5); there is no edge sweep, no glow and no ornament.
 *
 * The column has one hard job at the climax: hold a confident CLEAN while the
 * chamber next to it says BLOCKED. So the result is a real stamp rather than a
 * footnote — green ink at heading scale, one step below the verdict's, which is
 * the correct rank. It is *wrong*, not *timid*, and the demo only works if it
 * looks like a scanner that means it.
 *
 * The space under six lines of scanner output is filled by the argument rather
 * than left hollow: the four things reading a description once structurally
 * cannot show. Every entry names a real oracle the chamber fires (SPEC §4.4,
 * `static_blind`), so the left column states its own limits and the right
 * column then demonstrates them.
 */
const BLIND_SPOTS: { title: string; why: string }[] = [
  {
    title: "Whether the description changes later",
    why: "A scan reads one snapshot. A rug pull only exists across repetition.",
  },
  {
    title: "What the agent does with its own context",
    why: "Secrets in the model's prompt are not files, so nothing on disk to scan.",
  },
  {
    title: "Behavior behind an input nobody sent",
    why: "A conditional trigger stays dormant until its magic argument arrives.",
  },
  {
    title: "What the package does at install time",
    why: "Install hooks run before the first tool description is ever read.",
  },
];

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

        {result && <BlindSpots />}
      </div>

      {/* verdict footer — the payoff the right column is about to demolish */}
      <div className="shrink-0 border-t border-line px-5 py-4">
        {result ? (
          // No box, no wash. CLEAN is a genuine status stamp, so it earns both
          // uppercase and scale — it has to hold its own across the gutter from
          // BLOCKED, one rank below it, or the side-by-side has no tension.
          <div className="flex animate-fade-slide-up items-center gap-3.5">
            <span
              aria-hidden="true"
              className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-success/15 text-success"
            >
              <ShieldCheck size={22} strokeWidth={2} />
            </span>
            <div className="flex min-w-0 flex-col gap-0.5">
              <span className="text-2xl font-semibold leading-none tracking-tight text-success">
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

/**
 * What the scan above did not — and could not — check. Kept in the quiet half's
 * own register: muted ink, no red, no chips, no alarm. It is not an accusation
 * that mcp-scan is bad, it is the structural boundary of reading a label, and
 * the right column spends the next ninety seconds crossing it.
 */
function BlindSpots() {
  return (
    <div className="animate-fade-in mt-6">
      <SectionLabel
        right={
          <span className="telemetry tnum whitespace-nowrap">{BLIND_SPOTS.length} not checked</span>
        }
      >
        Outside a static scan
      </SectionLabel>

      <ul className="mt-2.5 flex flex-col">
        {BLIND_SPOTS.map((b) => (
          <li key={b.title} className="hairline flex items-start gap-2.5 py-2.5 first:border-t-0">
            <EyeOff size={13} className="mt-1 shrink-0 text-faint" aria-hidden="true" />
            <div className="min-w-0">
              <p className="text-sm font-medium leading-snug text-muted">{b.title}</p>
              <p className="mt-0.5 text-sm leading-relaxed text-faint">{b.why}</p>
            </div>
          </li>
        ))}
      </ul>

      <p className="mt-3 text-sm leading-relaxed text-faint">
        Not a flaw in the scanner. Reading a description once cannot show any of these — which is
        what the chamber on the right is for.
      </p>
    </div>
  );
}

function countTools(lines: ScanLine[]): number {
  return lines.filter((l) => l.stream === "result" && !l.done).length || 3;
}
