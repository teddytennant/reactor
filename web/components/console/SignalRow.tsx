"use client";

import {
  Bomb,
  Copy,
  Fingerprint,
  FileWarning,
  GitCompareArrows,
  Radio,
  ShieldCheck,
  Split,
  TimerReset,
  TriangleAlert,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/cn";
import type { SignalView } from "@/lib/reducer";
import { signalMeta } from "@/lib/signals";
import { EvidenceIds, SeverityChip, StaticBlindBadge, toneClasses } from "@/components/ui";

const ICONS: Record<string, LucideIcon> = {
  rug_pull: GitCompareArrows,
  context_exfil: Fingerprint,
  canary_exfil: Fingerprint,
  canary_read: FileWarning,
  task_deviation: Split,
  conditional_trigger: TimerReset,
  shadowing: Copy,
  install_hook: Bomb,
  analyst_injection: TriangleAlert,
  sleeper_beacon: Radio,
  benign_profile: ShieldCheck,
};

export function SignalRow({ signal }: { signal: SignalView }) {
  const meta = signalMeta(signal.type);
  const t = toneClasses[meta.tone === "success" ? "success" : meta.tone];
  const Icon = ICONS[signal.type] ?? TriangleAlert;

  return (
    <div
      className={cn(
        "animate-signal-snap rounded-xl border p-3.5",
        t.border,
        t.soft,
      )}
    >
      <div className="flex items-start gap-3">
        <span className={cn("mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-lg", t.soft, t.text)}>
          <Icon size={17} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="font-mono text-[13px] font-semibold text-fg">{signal.type}</span>
            <span className={cn("text-xs font-medium", t.text)}>{meta.label}</span>
            <SeverityChip severity={signal.severity} />
            {signal.static_blind && <StaticBlindBadge />}
          </div>
          <p className="mt-1 font-mono text-xs leading-relaxed text-muted">{signal.summary}</p>
          <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1">
            <span className="text-2xs uppercase tracking-wide text-faint">
              {signal.family} · deton. {signal.session}
            </span>
            <EvidenceIds ids={signal.evidence} tone={meta.tone === "success" ? "success" : meta.tone} />
          </div>
        </div>
      </div>
    </div>
  );
}
