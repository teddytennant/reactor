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
import {
  EvidenceIds,
  SeverityChip,
  StaticBlindBadge,
  toneClasses,
  type UiTone,
} from "@/components/ui";

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

/**
 * A fired oracle signal — the catch. Sans carries the row: the signal name, its
 * plain-English gloss and the finding are prose; only the type slug and the
 * evidence ids are mono machine data. Two stamps sit on the title line: the
 * severity chip, so danger ranks visibly above warning, and — when the oracle
 * is one a description scanner provably cannot produce — the static-blind
 * badge, which is the whole argument for Reactor existing.
 *
 * Not a card (DESIGN §2.4): signals sit on the chamber's own fill and are
 * separated by spacing and one hairline. No outline, no left rail, no wash —
 * that was three treatments saying the same thing. Danger is carried by the
 * ink alone: the icon and the signal name go red, everything else stays neutral
 * prose, so warning and success signals read as clearly subordinate.
 */
export function SignalRow({ signal }: { signal: SignalView }) {
  const meta = signalMeta(signal.type);
  const tone: UiTone = meta.tone === "success" ? "success" : meta.tone;
  const t = toneClasses[tone];
  const Icon = ICONS[signal.type] ?? TriangleAlert;
  const alarm = meta.tone === "danger";

  return (
    <article className="hairline animate-fade-slide-up pt-4">
      <div className="flex items-start gap-3">
        <Icon size={16} strokeWidth={1.9} className={cn("mt-0.5 shrink-0", t.text)} />

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
            <h3
              className={cn(
                "text-base font-semibold tracking-tight",
                alarm ? "text-danger" : "text-fg",
              )}
            >
              {meta.label}
            </h3>
            <SeverityChip severity={signal.severity} />
            {signal.static_blind && <StaticBlindBadge />}
          </div>

          <div className="tnum mt-1.5 font-mono text-2xs text-faint">
            {signal.type} · {signal.family}
            {signal.session !== undefined && <> · deton. {signal.session}</>}
          </div>

          <p className="mt-2 text-sm leading-relaxed text-muted">{meta.gloss}</p>
          <p
            className={cn(
              "mt-1.5 text-sm leading-relaxed",
              alarm ? "text-fg" : "text-muted",
            )}
          >
            {signal.summary}
          </p>

          <div className="mt-3 flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
            <span className="text-xs text-faint">Evidence</span>
            <EvidenceIds ids={signal.evidence} />
          </div>
        </div>
      </div>
    </article>
  );
}
