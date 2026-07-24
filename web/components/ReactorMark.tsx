import { cn } from "@/lib/cn";

/**
 * The Reactor mark — a containment hexagon with a lit core.
 *
 * Tuned to read at 20px: the hexagon is a flat 1.6px stroke on a 24px grid with
 * squared-off vertices, the containment ring is a broken hexagon (three arcs of
 * a second, smaller hex) so it never mushes into the outer shape, and the core
 * is a solid accent disc sitting inside its own bloom. Stroke inherits
 * currentColor; the core is always the accent token.
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
      {/* Inner containment ring, broken at the three lower vertices — reads as
          a shield seam rather than a second outline. */}
      <path
        d="M12 6.3 16.7 9v1.9M16.7 15 12 17.7 7.3 15M7.3 10.9V9L12 6.3"
        stroke="rgb(var(--accent))"
        strokeWidth="1.15"
        strokeLinecap="round"
        strokeLinejoin="round"
        opacity="0.55"
      />
      {/* Core bloom + core. */}
      <circle cx="12" cy="12" r="4.1" fill="rgb(var(--accent))" opacity="0.16" />
      <circle cx="12" cy="12" r="2.3" fill="rgb(var(--accent))" />
    </svg>
  );
}

export function Wordmark({ className }: { className?: string }) {
  return (
    <div className={cn("flex select-none items-center gap-2", className)}>
      <ReactorMark size={20} className="text-fg" />
      <span className="text-[15px] font-semibold leading-none tracking-[-0.025em] text-fg">
        Reactor
      </span>
    </div>
  );
}
