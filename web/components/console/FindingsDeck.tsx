"use client";

import type { ReactNode } from "react";
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
import type { ByteDiff, SignalView } from "@/lib/reducer";
import { signalMeta } from "@/lib/signals";
import { SeverityChip, StaticBlindBadge, toneClasses, type UiTone } from "@/components/ui";

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
 * The findings deck — the two events that carry the entire demo (DEMO §1): the
 * byte diff and the canary.
 *
 * It exists because those two moments used to fire into a scrolling telemetry
 * column and were dragged out of view by the streams behind them, so at the
 * climax the audience was looking at an egress list while the verdict landed.
 * DEMO's own wireframe pins them: signals sit between the chamber's nameplate
 * and the verdict and *stay there*. This component is that band. It is mounted
 * outside the scroller by `ReactorColumn`, so once a signal fires it is on
 * screen until the run is reset.
 *
 * Deliberately a **headline, not the evidence**: one line of the fact, plus the
 * one artifact of it that reads from the back of a room — the added byte run,
 * or the canary and where it went. The full forensic record (the byte ledger,
 * the rendered description, the evidence ids) stays in the stream below, where
 * a security-literate reader goes second.
 *
 * Danger is carried by ink, never by chrome (DESIGN §1): the icon and the
 * signal name go red, and the added bytes are highlighted because they *are*
 * the payload. No red outlines, no washes, no cards inside cards.
 */
export function FindingsDeck({
  signals,
  diffs,
}: {
  signals: SignalView[];
  diffs: ByteDiff[];
}) {
  if (signals.length === 0) return null;

  const blind = signals.filter((s) => s.static_blind).length;
  // The band's wash follows what actually fired. A clean artifact fires
  // `benign_profile` and nothing else, and tinting that deck red would put
  // reserved danger behind a green success row (DESIGN §1).
  const alarmed = signals.some((s) => signalMeta(s.type).tone === "danger");

  return (
    <section
      aria-label="Fired signals"
      className={cn(
        "shrink-0 border-b border-line",
        alarmed ? "bg-danger/[0.035]" : "bg-success/[0.03]",
      )}
    >
      <div className="flex items-center gap-3 px-4 pb-1 pt-3">
        <span className="strip-label whitespace-nowrap">Findings</span>
        <span className="rule" aria-hidden="true" />
        <span className="telemetry tnum whitespace-nowrap">
          {signals.length} fired
          {blind > 0 && <> · {blind} static-blind</>}
        </span>
      </div>

      {/* Bounded, and tightly: a live run against the poisoned server fires
          five oracles, and an unbounded deck pushed the verdict banner off the
          bottom of the chamber — trading one buried money shot for another.
          The cap holds the first two findings, which are the two the demo is
          built on, and the rest scroll. Masked at the bottom edge so the cut
          row fades instead of being sliced flat. */}
      <div className="max-h-[15rem] divide-y divide-line overflow-y-auto [mask-image:linear-gradient(to_top,transparent,#000_1.25rem)]">
        {signals.map((sig) => (
          <Finding
            key={sig.id}
            signal={sig}
            diff={diffs.find((d) => sig.evidence.includes(d.id))}
          />
        ))}
      </div>
    </section>
  );
}

function Finding({ signal, diff }: { signal: SignalView; diff?: ByteDiff }) {
  const meta = signalMeta(signal.type);
  const tone: UiTone = meta.tone;
  const t = toneClasses[tone];
  const Icon = ICONS[signal.type] ?? TriangleAlert;
  const alarm = meta.tone === "danger";
  const payload = renderPayload(signal, diff);

  return (
    <article className="animate-signal-snap px-4 py-3">
      <div className="flex items-start gap-2.5">
        <Icon size={15} strokeWidth={2} className={cn("mt-0.5 shrink-0", t.text)} />

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
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
            {signal.session !== undefined && (
              <span className="telemetry tnum ml-auto whitespace-nowrap">
                deton. {signal.session}
              </span>
            )}
          </div>

          {/* The oracle's own summary sentence, but only when nothing better
              is available. Where a payload renders — the added bytes, the
              canary and its destination — the summary is a prose restatement
              of the same fact three lines above it, and printing both cost the
              deck ~50px of height that the telemetry stream needed more. */}
          {!payload && <p className="mt-1 text-sm leading-relaxed text-muted">{signal.summary}</p>}

          {/* The one artifact of the finding that has to read from the back of
              the room. Everything else about it is in the stream below. */}
          {payload}
        </div>
      </div>
    </article>
  );
}

/** `detail` is typed as unknown on the wire; read it defensively. */
function str(d: Record<string, unknown> | undefined, k: string): string | undefined {
  const v = d?.[k];
  return typeof v === "string" && v ? v : undefined;
}

/**
 * The finding's headline artifact, or `null` when the signal type carries no
 * one thing worth enlarging. A plain function rather than a component so the
 * caller can ask whether there *is* a payload and drop the redundant summary
 * line when there is.
 */
function renderPayload(signal: SignalView, diff?: ByteDiff): ReactNode {
  const d = signal.detail;

  // Rug pull — the added run, as bytes. The delta is the whole proof that a
  // description scanner reading one snapshot cannot produce this.
  const added = diff?.added ?? str(d, "added");
  if (signal.type === "rug_pull" && added) {
    const delta = diff?.delta ?? (typeof d?.delta_bytes === "number" ? d.delta_bytes : undefined);
    const tool = diff?.tool ?? str(d, "tool");
    return (
      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-2">
        {delta !== undefined && (
          <span className="inline-flex shrink-0 items-baseline gap-1.5">
            <span className="tnum font-mono text-2xl font-semibold leading-none text-danger">
              +{delta}
            </span>
            <span className="text-sm text-muted">bytes</span>
          </span>
        )}
        {tool && (
          <span className="inline-flex shrink-0 items-baseline gap-1.5">
            <span className="text-sm text-muted">in</span>
            <span className="code-chip text-xs">{tool}</span>
          </span>
        )}
        <p className="min-w-0 basis-full rounded-xl bg-surface-3 px-3 py-2 font-mono text-xs leading-relaxed text-faint">
          …<span className="rounded bg-danger/[0.16] px-0.5 font-semibold text-danger">{added}</span>
        </p>
      </div>
    );
  }

  // Context / credential exfiltration — the canary and where it went. The
  // sentence under it is the demo's gasp line, and it is a fact: the token was
  // seeded into the victim's system prompt and never written to disk.
  const canary = str(d, "canary");
  if (canary) {
    const host = str(d, "host");
    const context = str(d, "kind") === "context";
    return (
      <div className="mt-2 flex flex-col gap-1.5">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="rounded-md bg-danger/[0.14] px-1.5 py-0.5 font-mono text-xs font-semibold text-danger">
            {canary}
          </span>
          <span className="text-faint" aria-hidden="true">
            →
          </span>
          {host && <span className="font-mono text-xs text-fg">{host}</span>}
          <span className="telemetry whitespace-nowrap">contained at the sink</span>
        </div>
        {context && (
          <p className="text-sm leading-relaxed text-fg">
            That token was never on disk. It lived only in the agent&rsquo;s system prompt — the
            server talked it out of it.
          </p>
        )}
      </div>
    );
  }

  return null;
}
