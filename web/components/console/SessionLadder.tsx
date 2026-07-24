"use client";

import { Check, CircleAlert } from "lucide-react";
import { cn } from "@/lib/cn";
import type { SessionView } from "@/lib/reducer";

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

  return (
    <div className="px-4 py-3">
      <div className="strip-label mb-2">Session ladder · fresh sandbox each detonation</div>
      <div className="flex flex-col gap-1">
        {rows.map((s, i) => (
          <SessionRow key={i} n={i + 1} s={s} />
        ))}
      </div>
    </div>
  );
}

function SessionRow({ n, s }: { n: number; s: SessionView | null }) {
  const status = s?.status ?? "pending";
  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-lg border px-3 py-2 transition-all duration-200",
        status === "dirty"
          ? "animate-fade-slide-up border-danger/35 bg-danger/[0.06]"
          : status === "clean"
            ? "animate-fade-slide-up border-line bg-surface-2"
            : status === "running"
              ? "animate-fade-slide-up border-accent/40 bg-accent/[0.05]"
              : "border-dashed border-line/70 opacity-45",
      )}
    >
      <StatusIcon status={status} />
      <span className="w-[74px] font-mono text-[13px] text-fg">
        deton. <span className="tnum">{n}</span>
      </span>
      <span className="flex-1 truncate text-xs">
        {status === "dirty" ? (
          <span className="font-medium text-danger">hijacked — oracle fired</span>
        ) : status === "clean" ? (
          <span className="text-muted">
            clean · <span className="tnum">{s?.toolCount ?? 3}</span> tools ·{" "}
            <span className="tnum">{s?.baitCount ?? 0}</span> bait
          </span>
        ) : status === "running" ? (
          <span className="text-accent">running…</span>
        ) : (
          <span className="text-faint">queued</span>
        )}
      </span>
    </div>
  );
}

function StatusIcon({ status }: { status: string }) {
  if (status === "dirty") {
    return (
      <span className="grid h-5 w-5 place-items-center rounded-full bg-danger/15 text-danger">
        <CircleAlert size={13} />
      </span>
    );
  }
  if (status === "clean") {
    return (
      <span className="grid h-5 w-5 place-items-center rounded-full bg-success/15 text-success">
        <Check size={13} />
      </span>
    );
  }
  if (status === "running") {
    return (
      <span className="relative grid h-5 w-5 place-items-center">
        <span className="absolute h-5 w-5 animate-ping rounded-full bg-accent/25" />
        <span className="h-2 w-2 rounded-full bg-accent" />
      </span>
    );
  }
  return <span className="h-5 w-5 rounded-full border border-dashed border-line" />;
}
