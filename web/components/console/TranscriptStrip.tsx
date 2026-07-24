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
 * on_task:false lines are the measurable hijack; they read red.
 */
export function TranscriptStrip({ lines }: { lines: TranscriptLine[] }) {
  const task = lines.find((l) => l.task)?.task;
  const shown = lines.filter((l) => l.action === "tool_call" || l.action === "final");
  const tail = shown.slice(-8);

  return (
    <div className="px-4 py-3">
      <div className="strip-label mb-2">Victim transcript</div>
      {task && (
        <div className="mb-2 text-xs text-muted">
          task <span className="text-fg">“{task}”</span>
        </div>
      )}
      <div className="flex flex-col gap-1 font-mono text-2xs">
        {tail.length === 0 && <div className="text-faint">no actions yet</div>}
        {tail.map((l) => {
          const off = !l.onTask;
          return (
            <div
              key={l.id}
              className={cn(
                "flex animate-fade-in flex-wrap items-center gap-x-2 gap-y-1 rounded px-1.5 py-1",
                off && "bg-danger/[0.06]",
              )}
            >
              <span className="w-9 shrink-0 text-faint">d{l.session}</span>
              <span className={cn("shrink-0", off ? "text-danger" : "text-muted")}>
                {ACTION_LABEL[l.action] ?? l.action}
              </span>
              {l.tool && <span className="text-fg">{l.tool}</span>}
              <span
                className={cn(
                  "shrink-0 rounded px-1 text-2xs",
                  off ? "bg-danger/15 text-danger" : "bg-success/15 text-success",
                )}
              >
                {off ? "off-task" : "on-task"}
              </span>
              {l.argPaths?.map((p) => (
                <code key={p} className="text-danger/80">
                  {p}
                </code>
              ))}
              {l.argCanaries?.map((c) => (
                <code key={c} className="rounded bg-danger/15 px-1 text-danger">
                  {c}
                </code>
              ))}
              {l.deviation && <span className="text-faint">· {l.deviation}</span>}
            </div>
          );
        })}
      </div>
    </div>
  );
}
