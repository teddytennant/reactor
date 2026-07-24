"use client";

import { CircleDollarSign, Eye, EyeOff, ShieldCheck, Target, Timer, Wand2 } from "lucide-react";
import { cn } from "@/lib/cn";
import type { Scorecard as ScorecardData } from "@/lib/scorecard";
import { signalMeta } from "@/lib/signals";

export function Scorecard({ data, live }: { data: ScorecardData; live: boolean }) {
  const { detection, false_quarantine, static_blind, redteam, time_to_verdict_ms, cost_usd, zoo } = data;

  return (
    <div className="mx-auto w-full max-w-[1100px]">
      {/* header */}
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <div className="text-2xs font-semibold uppercase tracking-[0.16em] text-accent">Scorecard</div>
          <h1 className="mt-1 text-2xl font-bold tracking-tight text-fg sm:text-3xl">
            The same zoo, through two detectors
          </h1>
          <p className="mt-1 text-sm text-muted">
            {zoo.malicious} malicious artifacts, {zoo.benign} real MCP servers.{" "}
            <span className="text-fg">Static reads the label; Reactor watches it behave.</span>
          </p>
        </div>
        <span
          className={cn(
            "inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-2xs font-semibold uppercase tracking-wide",
            live
              ? "border-success/35 bg-success/10 text-success"
              : "border-line bg-surface-2 text-faint",
          )}
        >
          <span className={cn("h-1.5 w-1.5 rounded-full", live ? "bg-success" : "bg-faint")} />
          {live ? "live eval" : "bundled fixture"}
        </span>
      </div>

      {/* hero numbers */}
      <div className="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <HeroStat
          Icon={Target}
          tone="accent"
          value={`${detection.caught}/${detection.total}`}
          label="detected"
          sub={`${pct(detection.rate)} detection rate`}
        />
        <HeroStat
          Icon={ShieldCheck}
          tone="success"
          value={`${false_quarantine.blocked}`}
          label="false blocks"
          sub={`on ${false_quarantine.total} real servers · ${pct(false_quarantine.rate)}`}
        />
        <HeroStat
          Icon={EyeOff}
          tone="danger"
          value={`${static_blind.static_blind_catches}`}
          label="static-blind"
          sub="catches a scanner provably can't see"
        />
      </div>

      {/* the centerpiece: static-blind comparison */}
      <div className="mt-4 rounded-2xl border border-line bg-surface p-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-[15px] font-semibold tracking-tight text-fg">
            Static-blind rate
          </h2>
          <span className="text-2xs uppercase tracking-wide text-faint">
            {zoo.malicious} malicious artifacts
          </span>
        </div>

        <div className="mt-4 flex flex-col gap-4">
          <CompareBar
            name="mcp-scan"
            note="static · reads descriptions"
            total={zoo.malicious}
            segments={[
              { value: static_blind.static_caught, kind: "both" },
              { value: static_blind.static_blind_catches, kind: "blind" },
              { value: static_blind.missed_by_both ?? zoo.malicious - static_blind.reactor_caught, kind: "missed" },
            ]}
            caughtLabel={`${static_blind.static_caught} caught`}
          />
          <CompareBar
            name="Reactor"
            note="runtime · watches behavior"
            total={zoo.malicious}
            emphasize
            segments={[
              { value: static_blind.static_caught, kind: "reactorBoth" },
              { value: static_blind.static_blind_catches, kind: "reactorBlind" },
              { value: static_blind.missed_by_both ?? zoo.malicious - static_blind.reactor_caught, kind: "missed" },
            ]}
            caughtLabel={`${static_blind.reactor_caught} caught`}
          />
        </div>

        {/* legend */}
        <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-line pt-3 text-2xs text-muted">
          <Legend swatch="bg-faint" label="caught by both" />
          <Legend swatch="bg-danger" label="static-blind — Reactor only" icon={<EyeOff size={11} className="text-danger" />} />
          <Legend swatch="bg-accent" label="Reactor catch" />
          <Legend swatch="bg-line-strong" label="missed by both" />
        </div>

        {/* the 9, by type */}
        <div className="mt-4">
          <div className="strip-label mb-2 flex items-center gap-1.5">
            <Eye size={12} className="text-faint" />
            The {static_blind.static_blind_catches} with no static signature
          </div>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {static_blind.by_type.map((t) => (
              <div key={t.type} className="rounded-lg border border-danger/25 bg-danger/[0.05] px-3 py-2">
                <div className="tnum text-lg font-semibold text-danger">{t.reactor}</div>
                <div className="font-mono text-2xs text-muted">{t.type}</div>
                <div className="text-2xs text-faint">{signalMeta(t.type).label}</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* secondary stats */}
      <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile Icon={Timer} value={`${(time_to_verdict_ms.mean / 1000).toFixed(1)}s`} label="mean time-to-verdict" sub={`p95 ${(time_to_verdict_ms.p95 / 1000).toFixed(0)}s`} />
        <StatTile Icon={CircleDollarSign} value={`$${cost_usd.mean.toFixed(3)}`} label="cost / detonation" sub={`$${cost_usd.total.toFixed(2)} total`} />
        <StatTile Icon={Wand2} value={`${redteam.escaped}/${redteam.mutations}`} label="red-team escape" sub={`${pct(redteam.rate)} slipped past`} />
        <StatTile Icon={Target} value={pct(detection.rate)} label="detection rate" sub={`${detection.caught} of ${detection.total}`} />
      </div>

      {/* tagline */}
      <div className="mt-5 rounded-2xl border border-line bg-surface-2 px-5 py-4 text-center">
        <p className="text-base font-medium tracking-tight text-fg sm:text-lg">
          {static_blind.static_blind_catches} of those catches are things a description scanner{" "}
          <span className="text-danger">cannot</span> see. Not doesn&apos;t — can&apos;t.
        </p>
        <p className="mt-1 text-xs text-muted">
          VirusTotal never saw the agent supply chain coming. Static scanners read the label.{" "}
          <span className="text-fg">We watch it behave.</span>
        </p>
      </div>
    </div>
  );
}

// ---- pieces ---------------------------------------------------------------

function pct(x: number): string {
  return `${Math.round(x * 100)}%`;
}

const HERO_TONE: Record<string, { text: string; soft: string; border: string }> = {
  accent: { text: "text-accent", soft: "bg-accent/10", border: "border-accent/25" },
  success: { text: "text-success", soft: "bg-success/10", border: "border-success/25" },
  danger: { text: "text-danger", soft: "bg-danger/10", border: "border-danger/25" },
};

function HeroStat({
  Icon,
  tone,
  value,
  label,
  sub,
}: {
  Icon: typeof Target;
  tone: "accent" | "success" | "danger";
  value: string;
  label: string;
  sub: string;
}) {
  const t = HERO_TONE[tone];
  return (
    <div className={cn("rounded-2xl border bg-surface p-5", t.border)}>
      <div className="flex items-center justify-between">
        <span className="text-2xs font-medium uppercase tracking-wide text-faint">{label}</span>
        <span className={cn("grid h-7 w-7 place-items-center rounded-lg", t.soft, t.text)}>
          <Icon size={15} />
        </span>
      </div>
      <div className={cn("mt-2 tnum text-4xl font-bold tracking-tight sm:text-5xl", t.text)}>{value}</div>
      <div className="mt-1 text-xs text-muted">{sub}</div>
    </div>
  );
}

type SegKind = "both" | "blind" | "missed" | "reactorBoth" | "reactorBlind";

const SEG_CLS: Record<SegKind, string> = {
  both: "bg-faint",
  blind: "bg-danger",
  missed: "bg-line-strong",
  reactorBoth: "bg-accent/70",
  reactorBlind: "bg-accent",
};

function CompareBar({
  name,
  note,
  total,
  segments,
  caughtLabel,
  emphasize,
}: {
  name: string;
  note: string;
  total: number;
  segments: { value: number; kind: SegKind }[];
  caughtLabel: string;
  emphasize?: boolean;
}) {
  return (
    <div>
      <div className="mb-1.5 flex items-baseline justify-between gap-2">
        <div className="flex items-baseline gap-2">
          <span className={cn("font-mono text-sm font-semibold", emphasize ? "text-accent" : "text-fg")}>
            {name}
          </span>
          <span className="text-2xs text-faint">{note}</span>
        </div>
        <span className={cn("tnum text-sm font-semibold", emphasize ? "text-accent" : "text-muted")}>
          {caughtLabel}
        </span>
      </div>
      <div className="flex h-9 w-full items-stretch gap-0.5 overflow-hidden rounded-lg">
        {segments.map((s, i) =>
          s.value > 0 ? (
            <div
              key={i}
              className={cn(
                "flex origin-left animate-bar-grow items-center justify-center rounded-[3px]",
                SEG_CLS[s.kind],
              )}
              style={{ width: `${(s.value / total) * 100}%`, animationDelay: `${i * 90}ms` }}
              title={`${s.value}`}
            >
              {s.value >= 2 && s.kind !== "missed" && (
                <span className="tnum text-2xs font-semibold text-white">{s.value}</span>
              )}
            </div>
          ) : null,
        )}
      </div>
    </div>
  );
}

function Legend({ swatch, label, icon }: { swatch: string; label: string; icon?: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      {icon ?? <span className={cn("h-2.5 w-2.5 rounded-[3px]", swatch)} />}
      {label}
    </span>
  );
}

function StatTile({
  Icon,
  value,
  label,
  sub,
}: {
  Icon: typeof Target;
  value: string;
  label: string;
  sub: string;
}) {
  return (
    <div className="rounded-xl border border-line bg-surface p-4">
      <Icon size={15} className="text-faint" />
      <div className="mt-2 tnum text-2xl font-bold tracking-tight text-fg">{value}</div>
      <div className="mt-0.5 text-2xs font-medium uppercase tracking-wide text-muted">{label}</div>
      <div className="text-2xs text-faint">{sub}</div>
    </div>
  );
}
