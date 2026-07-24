import { cn } from "@/lib/cn";
import type { Tone } from "@/lib/signals";
import type { Severity } from "@/lib/events";
import { severityTone } from "@/lib/signals";

/**
 * The tone vocabulary used across the console. `Tone` (danger | warning |
 * success | neutral) comes from lib/signals; the UI layer adds two presentation
 * roles that never originate from a signal:
 *   accent — Reactor itself, interactive, the one primary action
 *   live   — streaming / in-flight telemetry (used very sparingly)
 */
export type UiTone = Tone | "accent" | "live";

export interface ToneStyles {
  /** Foreground text in this tone. */
  text: string;
  /** The soft tinted fill — chips, lit rows. Borderless by design. */
  soft: string;
  /** Faint edge tint in this tone. Use at low alpha; never as a drawn line. */
  border: string;
  /** Flat swatch fill. Prefer <Led> for status; this is for bars and ticks. */
  dot: string;
  /** Solid fill + its legible on-color. Reserve for one primary action. */
  solid: string;
  /** The value for `data-tone` on `.led` / `.rail-node`. */
  led: string;
  /** Soft, wide, low-opacity cast in this hue. No rim, no halo. */
  glow: string;
}

// Tone → utility classes. One place, so danger always reads the same red.
export const toneClasses: Record<UiTone, ToneStyles> = {
  danger: {
    text: "text-danger",
    soft: "bg-danger/10",
    border: "border-danger/30",
    dot: "bg-danger",
    solid: "bg-danger text-danger-fg",
    led: "danger",
    glow: "shadow-glow-danger",
  },
  warning: {
    text: "text-warning",
    soft: "bg-warning/10",
    border: "border-warning/30",
    dot: "bg-warning",
    solid: "bg-warning text-warning-fg",
    led: "warning",
    glow: "shadow-glow-warning",
  },
  success: {
    text: "text-success",
    soft: "bg-success/10",
    border: "border-success/30",
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
    border: "border-accent/30",
    dot: "bg-accent",
    solid: "bg-accent text-accent-fg",
    led: "accent",
    glow: "shadow-glow-accent",
  },
  live: {
    text: "text-live",
    soft: "bg-live/12",
    border: "border-live/30",
    dot: "bg-live",
    solid: "bg-live text-live-fg",
    led: "live",
    glow: "shadow-glow-live",
  },
};

/**
 * A small pill. Soft radius, filled, borderless — the reference's chip, not a
 * bordered badge. Pass `mono` for machine data (package names, ids, counts).
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
        "inline-flex items-center gap-1.5 rounded-lg px-2 py-0.5 text-xs font-medium",
        t.text,
        t.soft,
        mono && "font-mono text-2xs",
        className,
      )}
    >
      {children}
    </span>
  );
}

/**
 * Severity chip, e.g. CRITICAL. A genuine status stamp, so uppercase is earned
 * here — sans, not mono, and only lightly letterspaced.
 */
export function SeverityChip({ severity }: { severity: Severity | string }) {
  const tone = severityTone(severity);
  return (
    <Chip tone={tone} className="uppercase tracking-label-wide">
      {severity}
    </Chip>
  );
}

/**
 * The core selling point badge — a description scanner provably can't see it.
 * Deliberately neutral: accent is reserved for the one primary action and for
 * selection (DESIGN §1), so this claim earns its place by being legible, not by
 * being coloured. A soft neutral chip, sentence case, no dot.
 */
export function StaticBlindBadge() {
  return (
    <span
      title="A description-only scanner provably cannot produce this signal."
      className="inline-flex items-center rounded-lg bg-surface-3 px-2 py-0.5 text-xs font-medium text-muted"
    >
      Static-blind
    </span>
  );
}

/** Evidence event-id pills (wire:4:tools/list, egress:7, …). Machine data. */
export function EvidenceIds({ ids, tone = "neutral" }: { ids: string[]; tone?: UiTone }) {
  const t = toneClasses[tone];
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {ids.map((id) => (
        <code key={id} className={cn("code-chip tnum text-3xs", t.text, t.soft)}>
          {id}
        </code>
      ))}
    </div>
  );
}

/**
 * A status dot (DESIGN §2.2). Small, calm, flat — no neon core, no resting
 * halo. `pulse` adds a soft slow halo and is reserved for genuinely live or
 * streaming state. `neutral` renders as an inactive low-contrast dot, so state
 * is never carried by color alone.
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

/** Status dot. Same contract as before. */
export function StatusDot({ tone, pulse }: { tone: UiTone; pulse?: boolean }) {
  return <Led tone={tone} pulse={pulse} />;
}

/**
 * The soft containment panel (DESIGN §2.1) — the Reactor column's treatment.
 * A slightly lifted fill, generous radius, faint edge; on `state="blocked"` the
 * edge picks up a restrained danger tint. No corner ticks, no ring, no
 * ornament. `state` API unchanged.
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

/**
 * A section label: sentence-case sans, 13px, `--muted`, with an optional right
 * slot. Grouping comes from the shared panel fill and spacing, not from rules
 * and boxes.
 */
export function SectionLabel({ children, right }: { children: React.ReactNode; right?: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="strip-label">{children}</span>
      {right}
    </div>
  );
}
