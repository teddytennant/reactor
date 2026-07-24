import { cn } from "@/lib/cn";
import type { Tone } from "@/lib/signals";
import type { Severity } from "@/lib/events";
import { severityTone } from "@/lib/signals";

/**
 * The tone vocabulary used across the console. `Tone` (danger | warning |
 * success | neutral) comes from lib/signals; the UI layer adds two presentation
 * roles that never originate from a signal:
 *   accent — Reactor itself, interactive, the right column
 *   live   — streaming / in-flight telemetry (used sparsely)
 */
export type UiTone = Tone | "accent" | "live";

export interface ToneStyles {
  /** Foreground text in this tone. */
  text: string;
  /** The soft tinted fill — chips, lit rows. */
  soft: string;
  /** Hairline border in this tone. */
  border: string;
  /** Flat swatch fill. Prefer <Led> for status; this is for bars and ticks. */
  dot: string;
  /** Solid fill + its legible on-color. */
  solid: string;
  /** The value for `data-tone` on `.led` / `.rail-node`. */
  led: string;
  /** Ring + halo box-shadow utility in this hue. */
  glow: string;
}

// Tone → utility classes. One place, so danger always reads the same red.
export const toneClasses: Record<UiTone, ToneStyles> = {
  danger: {
    text: "text-danger",
    soft: "bg-danger/10",
    border: "border-danger/35",
    dot: "bg-danger",
    solid: "bg-danger text-danger-fg",
    led: "danger",
    glow: "shadow-glow-danger",
  },
  warning: {
    text: "text-warning",
    soft: "bg-warning/10",
    border: "border-warning/35",
    dot: "bg-warning",
    solid: "bg-warning text-warning-fg",
    led: "warning",
    glow: "shadow-glow-warning",
  },
  success: {
    text: "text-success",
    soft: "bg-success/10",
    border: "border-success/35",
    dot: "bg-success",
    solid: "bg-success text-success-fg",
    led: "success",
    glow: "shadow-glow-success",
  },
  neutral: {
    text: "text-muted",
    soft: "bg-surface-2",
    border: "border-line",
    dot: "bg-faint",
    solid: "bg-surface-3 text-fg",
    led: "neutral",
    glow: "shadow-none",
  },
  accent: {
    text: "text-accent",
    soft: "bg-accent/10",
    border: "border-accent/35",
    dot: "bg-accent",
    solid: "bg-accent text-accent-fg",
    led: "accent",
    glow: "shadow-glow-accent",
  },
  live: {
    text: "text-live",
    soft: "bg-live/10",
    border: "border-live/35",
    dot: "bg-live",
    solid: "bg-live text-live-fg",
    led: "live",
    glow: "shadow-glow-live",
  },
};

/**
 * A small pill. Squared-off (2px radius), hairline, 11px. Mono by request —
 * anything the machine said should pass `mono`.
 */
export function Chip({
  children,
  tone = "neutral",
  className,
  mono,
}: {
  children: React.ReactNode;
  tone?: UiTone;
  className?: string;
  mono?: boolean;
}) {
  const t = toneClasses[tone];
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-[3px] border px-1.5 py-px text-2xs font-medium leading-[1.35]",
        t.text,
        t.soft,
        t.border,
        mono && "font-mono tracking-[0.04em]",
        className,
      )}
    >
      {children}
    </span>
  );
}

/** Severity chip, e.g. CRITICAL. Always mono — this is a machine verdict. */
export function SeverityChip({ severity }: { severity: Severity | string }) {
  const tone = severityTone(severity);
  return (
    <Chip tone={tone} mono className="uppercase tracking-label-wide">
      {severity}
    </Chip>
  );
}

/** The core selling point badge — a description scanner provably can't see it. */
export function StaticBlindBadge() {
  return (
    <span
      title="A description-only scanner provably cannot produce this signal."
      className="inline-flex items-center gap-1.5 rounded-[3px] border border-accent/40 bg-accent/10 px-1.5 py-px font-mono text-2xs font-semibold uppercase leading-[1.35] tracking-label text-accent"
    >
      <Led tone="accent" size="sm" />
      Static-blind
    </span>
  );
}

/** Evidence event-id pills (wire:4:tools/list, egress:7, …). */
export function EvidenceIds({ ids, tone = "neutral" }: { ids: string[]; tone?: UiTone }) {
  const t = toneClasses[tone];
  return (
    <div className="flex flex-wrap items-center gap-1">
      {ids.map((id) => (
        <code
          key={id}
          className={cn(
            "tnum rounded-[3px] border bg-transparent px-1 py-px font-mono text-3xs leading-[1.5]",
            t.text,
            t.border,
          )}
        >
          {id}
        </code>
      ))}
    </div>
  );
}

/**
 * The bloom-LED (DESIGN.md §2.2). A solid core with a real halo in its own hue;
 * `pulse` gives it the slow breath reserved for live things. `neutral` renders
 * as an unlit bead in a hairline socket, so state is never color alone.
 */
export function Led({
  tone,
  pulse,
  size = "md",
  className,
}: {
  tone: UiTone;
  pulse?: boolean;
  size?: "sm" | "md" | "lg";
  className?: string;
}) {
  return (
    <span
      className={cn("led", size === "sm" && "led-sm", size === "lg" && "led-lg", className)}
      data-tone={toneClasses[tone].led}
      data-pulse={pulse ? "true" : undefined}
    />
  );
}

/** Status LED. Same contract as before; now a real lamp instead of a flat dot. */
export function StatusDot({ tone, pulse }: { tone: UiTone; pulse?: boolean }) {
  return <Led tone={tone} pulse={pulse} />;
}

/**
 * The containment bezel (DESIGN.md §2.1) — inset ring + corner tick marks.
 * `state` drives the tick hue: idle accent, running live, blocked danger.
 * Put nothing else with a ::before/::after primitive on this element; nest
 * `.instrument-grid` / `.scan-sweep` on a child instead.
 */
export function Bezel({
  children,
  state = "idle",
  className,
  ...rest
}: {
  children?: React.ReactNode;
  state?: "idle" | "running" | "blocked" | "allowed";
  className?: string;
} & Omit<React.HTMLAttributes<HTMLDivElement>, "children" | "className">) {
  return (
    <div className={cn("bezel", className)} data-state={state} {...rest}>
      {children}
    </div>
  );
}

/** A mono section label with a fading hairline and an optional right slot. */
export function SectionLabel({ children, right }: { children: React.ReactNode; right?: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2.5">
      <span className="strip-label whitespace-nowrap">{children}</span>
      <span className="rule" aria-hidden="true" />
      {right}
    </div>
  );
}
