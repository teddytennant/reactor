"use client";

import { ArrowRight, Radio } from "lucide-react";
import { cn } from "@/lib/cn";
import type { WireLine } from "@/lib/reducer";

const METHOD_TONE: Record<string, string> = {
  initialize: "text-faint",
  "tools/list": "text-muted",
  "tools/call": "text-accent",
};

/**
 * The MCP wire log — the JSON-RPC proxy's view. Compact by design: each line
 * carries the observed description byte count so a mutation is legible even
 * before the ByteDiff card renders. Newest lines stream in at the bottom.
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
      <div className="strip-label mb-2 flex items-center gap-1.5">
        <Radio size={12} className="text-faint" />
        MCP wire log
      </div>
      <div className="flex flex-col gap-0.5 font-mono text-2xs">
        {tail.length === 0 && <div className="text-faint">no frames yet</div>}
        {tail.map((l) => (
          <div
            key={l.id}
            className={cn(
              "flex animate-fade-in items-center gap-2 rounded px-1.5 py-1",
              l.diff && "bg-danger/[0.06]",
            )}
          >
            <span className="w-9 shrink-0 text-faint">d{l.session}</span>
            <span className="w-3 shrink-0 text-faint">
              {l.dir.startsWith("agent") ? "→" : "←"}
            </span>
            <span className={cn("shrink-0", METHOD_TONE[l.method] ?? "text-muted")}>{l.method}</span>
            {l.tool && <span className="text-fg">{l.tool}</span>}
            {l.toolNames && !l.tool && (
              <span className="text-faint">[{l.toolNames.join(", ")}]</span>
            )}
            {typeof l.descriptionBytes === "number" && (
              <span
                className={cn(
                  "ml-auto tnum shrink-0",
                  l.diff ? "font-medium text-danger" : "text-faint",
                )}
              >
                {l.descriptionBytes}B
                {l.diff && (
                  <span className="ml-1 inline-flex items-center gap-0.5">
                    <ArrowRight size={9} className="inline" />+{l.diff.delta}
                  </span>
                )}
              </span>
            )}
            {l.argPaths && l.argPaths.length > 0 && (
              <span className="ml-auto shrink-0 text-danger/80">{l.argPaths.join(" ")}</span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
