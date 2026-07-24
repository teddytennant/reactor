import { cn } from "@/lib/cn";

/**
 * The Reactor mark — a containment hexagon with a solid core.
 *
 * Tuned to read at 20px: a flat 1.6px stroke on a 24px grid with squared-off
 * vertices, a faint inner containment seam, and a solid accent core. Calm and
 * flat — no bloom, no glow (DESIGN §1/§2). Stroke inherits currentColor; the
 * core is the accent token.
 */
export function ReactorMark({ size = 22, className }: { size?: number; className?: string }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      className={className}
      aria-hidden="true"
    >
      {/* Containment hexagon. */}
      <path
        d="M12 2.15 20.45 7v10L12 21.85 3.55 17V7L12 2.15Z"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinejoin="round"
      />
      {/* Inner containment seam, broken at the three lower vertices — reads as
          a shield seam rather than a second outline. */}
      <path
        d="M12 6.3 16.7 9v1.9M16.7 15 12 17.7 7.3 15M7.3 10.9V9L12 6.3"
        stroke="currentColor"
        strokeWidth="1.15"
        strokeLinecap="round"
        strokeLinejoin="round"
        opacity="0.32"
      />
      {/* The core. Flat, solid, no bloom. */}
      <circle cx="12" cy="12" r="2.5" fill="rgb(var(--accent))" />
    </svg>
  );
}

export function Wordmark({ className }: { className?: string }) {
  return (
    <div className={cn("flex select-none items-center gap-2.5", className)}>
      <ReactorMark size={20} className="text-fg" />
      <span className="text-[15px] font-semibold leading-none tracking-[-0.015em] text-fg">
        Reactor
      </span>
    </div>
  );
}
