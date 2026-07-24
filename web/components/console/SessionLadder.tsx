"use client";

import { Check, CircleAlert } from "lucide-react";
import { cn } from "@/lib/cn";
import type { SessionView } from "@/lib/reducer";
import { Led, SectionLabel } from "@/components/ui";

type RowStatus = "pending" | "running" | "clean" | "dirty";

/**
 * The session ladder — detonations accruing down a gutter rail (DESIGN §2.3).
 * The rail hairline plus the mono index carries the structure, so the rows need
 * no cards and no zebra: only spacing, a node marker where something happened,
 * and a `tick-in` entrance so sessions *accrue* instead of popping.
 *
 * Type follows the split: the detonation index is machine data and stays mono;
 * the status words are chrome and are sentence-case sans. CLEAN is the one
 * exception — a genuine status stamp, so uppercase is earned there.
 */
export function SessionLadder({
  sessions,
  plannedSessions,
}: {
  sessions: SessionView[];
  plannedSessions: number;
}) {
  // Show the planned ladder (1..N) so the passage of time reads as the argument;
  // rows fill in as detonations tick.
  const rows = Array.from({ length: plannedSessions }, (_, i) => {
    const n = i + 1;
    return sessions.find((s) => s.n === n) ?? null;
  });
  const landed = sessions.filter((s) => s.status !== "running").length;

  return (
    <div className="px-4 py-3">
      <SectionLabel
        right={
          <span className="telemetry tnum whitespace-nowrap">
            {landed}/{plannedSessions}
          </span>
        }
      >
        Session ladder
      </SectionLabel>
      <p className="mt-1 text-xs text-muted">Fresh sandbox each detonation</p>

      <div className="rail rail-tight mt-3">
        {rows.map((s, i) => (
          <SessionRow key={i} n={i + 1} s={s} />
        ))}
      </div>
    </div>
  );
}

function SessionRow({ n, s }: { n: number; s: SessionView | null }) {
  const status: RowStatus = (s?.status as RowStatus) ?? "pending";
  const node =
    status === "dirty"
      ? "danger"
      : status === "running"
        ? "live"
        : status === "clean"
          ? undefined
          : null;

  return (
    <div className={cn("rail-row py-2", status !== "pending" && "animate-tick-in")}>
      <div className="rail-time">
        {String(n).padStart(2, "0")}
        {node !== null && (
          <span className="rail-node" data-tone={node} aria-hidden="true" />
        )}
      </div>

      <div
        className={cn(
          "-mx-2 flex min-w-0 items-center gap-2.5 rounded-lg px-2",
          status === "dirty" && "bg-danger/[0.07]",
        )}
      >
        <span className="flex w-3.5 shrink-0 justify-center" aria-hidden="true">
          {status === "dirty" ? (
            <CircleAlert size={13} className="text-danger" />
          ) : status === "clean" ? (
            <Check size={13} className="text-success" />
          ) : status === "running" ? (
            <Led tone="live" size="sm" pulse />
          ) : (
            <span className="h-1.5 w-1.5 rounded-full border border-dashed border-line-strong" />
          )}
        </span>

        <span className="shrink-0 font-mono text-xs text-faint">
          deton. <span className="tnum text-fg">{n}</span>
        </span>

        {status === "dirty" ? (
          <span className="truncate text-sm font-medium text-danger">
            Hijacked · oracle fired
          </span>
        ) : status === "clean" ? (
          <>
            <span className="text-2xs font-semibold uppercase tracking-label-wide text-success">
              clean
            </span>
            <span className="rule" aria-hidden="true" />
            <span className="telemetry tnum shrink-0 whitespace-nowrap">
              <span className="tnum">{s?.toolCount ?? 3}</span> tools ·{" "}
              <span className="tnum">{s?.baitCount ?? 0}</span> bait
            </span>
          </>
        ) : status === "running" ? (
          <>
            <span className="shrink-0 text-sm text-live">Running</span>
            <span
              className="sweep-bg animate-sweep h-px flex-1 rounded-full"
              aria-hidden="true"
            />
          </>
        ) : (
          <span className="text-sm text-faint">Queued</span>
        )}
      </div>
    </div>
  );
}
