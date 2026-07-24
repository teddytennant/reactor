"use client";

import { cn } from "@/lib/cn";
import type { WireLine } from "@/lib/reducer";
import { SectionLabel } from "@/components/ui";

const METHOD_TONE: Record<string, string> = {
  initialize: "text-faint",
  "tools/list": "text-muted",
  "tools/call": "text-accent",
};

/**
 * The MCP wire log — the JSON-RPC proxy's view, and the most technical surface
 * in the console. Telemetry rows: a gutter rail carrying the detonation index
 * and a node where a frame matters, faint hairline separation only (no nested
 * cards, no zebra). The payload is machine data, so it stays mono end to end
 * and may run tighter than the rest of the column (DESIGN §3); only the chrome
 * around it is sans.
 *
 * Direction is a glyph plus ink weight, never colour: `→` at full contrast is a
 * request the agent sent, `←` faint is the server answering. Each line carries
 * the observed description byte count so a mutation is legible even before the
 * ByteDiff card renders. Long tool lists and argument paths wrap or truncate
 * inside the row — the column never widens.
 */
export function WireLog({ lines }: { lines: WireLine[] }) {
  // Keep the strip readable: drop the noisy per-tool duplicate list entries,
  // keeping the first tool of each tools/list (it carries tool_names) plus any
  // mutated entry, plus initialize/tools_call.
  const shown = lines.filter(
    (l) => l.method !== "tools/list" || !!l.toolNames || !!l.diff,
  );
  const tail = shown.slice(-14);

  return (
    <div className="px-4 py-3">
      <SectionLabel
        right={
          <span className="telemetry tnum whitespace-nowrap">{shown.length} frames</span>
        }
      >
        MCP wire log
      </SectionLabel>

      <div className="rail mt-3" style={{ ["--rail-x" as string]: "2.5rem" }}>
        {tail.length === 0 && (
          <div className="py-2 text-sm text-muted">No frames yet</div>
        )}

        {tail.map((l, i) => {
          const out = l.dir.startsWith("agent");
          return (
            <div
              key={l.id}
              className={cn(
                "rail-row animate-fade-in border-line py-1.5",
                i > 0 && "border-t",
              )}
              title={l.dir}
            >
              <div className="rail-time">
                d{l.session}
                <span
                  className="rail-node"
                  data-tone={l.diff ? "danger" : undefined}
                  aria-hidden="true"
                />
              </div>

              <div
                className={cn(
                  "-mx-2 min-w-0 rounded-lg px-2",
                  l.diff && "bg-danger/[0.07]",
                )}
              >
                <div className="flex min-w-0 items-center gap-2">
                  <span className="sr-only">{l.dir}</span>
                  <span
                    className={cn(
                      "w-3 shrink-0 text-center font-mono text-xs leading-[1.0625rem]",
                      out ? "text-fg" : "text-faint",
                    )}
                    aria-hidden="true"
                  >
                    {out ? "→" : "←"}
                  </span>

                  <span
                    className={cn(
                      "shrink-0 font-mono text-xs",
                      METHOD_TONE[l.method] ?? "text-muted",
                    )}
                  >
                    {l.method}
                  </span>

                  {l.tool && (
                    <span className="min-w-0 truncate font-mono text-xs text-fg">
                      {l.tool}
                    </span>
                  )}
                  {l.toolNames && !l.tool && (
                    <span
                      className="min-w-0 truncate font-mono text-xs text-faint"
                      title={l.toolNames.join(", ")}
                    >
                      [{l.toolNames.join(", ")}]
                    </span>
                  )}

                  <span className="rule" aria-hidden="true" />

                  {typeof l.descriptionBytes === "number" && (
                    <span
                      className={cn(
                        "tnum shrink-0 font-mono text-2xs",
                        l.diff ? "font-semibold text-danger" : "text-faint",
                      )}
                    >
                      {l.descriptionBytes}B
                    </span>
                  )}

                  {l.diff && (
                    <span className="tnum shrink-0 rounded-md bg-danger/10 px-1.5 py-px font-mono text-2xs font-semibold leading-[1.35] text-danger">
                      {l.diff.delta > 0 ? "+" : ""}
                      {l.diff.delta}
                    </span>
                  )}
                </div>

                {l.argPaths && l.argPaths.length > 0 && (
                  <div className="mt-0.5 flex min-w-0 items-baseline gap-1.5">
                    <span className="shrink-0 text-2xs text-faint">args</span>
                    <span className="min-w-0 break-all font-mono text-3xs text-muted">
                      {l.argPaths.join(" ")}
                    </span>
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
