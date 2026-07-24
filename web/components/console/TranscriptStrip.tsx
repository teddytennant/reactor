"use client";

import { cn } from "@/lib/cn";
import type { TranscriptLine } from "@/lib/reducer";

const ACTION_LABEL: Record<string, string> = {
  tool_call: "tool_call",
  final: "final",
  assistant_message: "message",
  tool_result: "result",
  refusal: "refusal",
};

/**
 * The victim transcript — what the agent *chose* to do versus its benign task.
 * Prose from an untrusted model, so it is set as quoted evidence: the strip
 * hangs off one hairline quote rail, never a card, and stays calm and
 * secondary. The task and the model's own words are sans prose; the action
 * kind, the tool name and the argument paths are mono machine data. Red is
 * reserved for fired signals and BLOCKED, so the measurable hijack reads amber
 * here.
 */
export function TranscriptStrip({ lines }: { lines: TranscriptLine[] }) {
  const task = lines.find((l) => l.task)?.task;
  const shown = lines.filter((l) => l.action === "tool_call" || l.action === "final");
  const tail = shown.slice(-8);

  return (
    <div className="px-4 py-3">
      <div className="flex items-center gap-2.5">
        <span className="strip-label whitespace-nowrap">Victim transcript</span>
        <span className="rule" aria-hidden="true" />
        <span className="whitespace-nowrap text-xs text-faint">untrusted output</span>
      </div>

      <div className="mt-3 border-l border-line pl-4">
        {task && (
          <div className="pb-3">
            <div className="text-xs text-faint">Task</div>
            <p className="mt-1 text-sm leading-relaxed text-fg">“{task}”</p>
          </div>
        )}

        <div className={cn("flex flex-col", task && "border-t border-line/60 pt-1")}>
          {tail.length === 0 && (
            <div className="py-2 text-sm text-faint">no actions yet</div>
          )}
          {tail.map((l) => {
            const off = !l.onTask;
            return (
              <div key={l.id} className="animate-fade-in py-2">
                <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
                  <span className="tnum w-6 shrink-0 font-mono text-2xs text-faint">
                    d{l.session}
                  </span>
                  <span
                    className={cn(
                      "shrink-0 font-mono text-2xs",
                      off ? "text-warning" : "text-muted",
                    )}
                  >
                    {ACTION_LABEL[l.action] ?? l.action}
                  </span>
                  {l.tool && (
                    <span className="font-mono text-2xs font-medium text-fg">{l.tool}</span>
                  )}
                  <span
                    className={cn(
                      "shrink-0 text-xs",
                      off ? "font-medium text-warning" : "text-faint",
                    )}
                  >
                    {off ? "off-task" : "on-task"}
                  </span>
                  {l.argPaths?.map((p) => (
                    <code key={p} className="code-chip bg-warning/10 text-warning">
                      {p}
                    </code>
                  ))}
                  {l.argCanaries?.map((c) => (
                    <code key={c} className="code-chip bg-warning/10 font-medium text-warning">
                      {c}
                    </code>
                  ))}
                  {l.deviation && <span className="text-xs text-faint">· {l.deviation}</span>}
                </div>
                {l.text && (
                  <p className="mt-1.5 pl-8 text-sm leading-relaxed text-muted">“{l.text}”</p>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
