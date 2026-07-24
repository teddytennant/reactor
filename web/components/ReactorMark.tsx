import { cn } from "@/lib/cn";

/**
 * The Reactor mark — the same brand mark used as the site favicon.
 *
 * Renders the packaged icon asset so the chrome wordmark and the tab icon
 * stay in lockstep. Sized for ~20px chrome; rounded slightly to match the
 * app's soft corners.
 */
export function ReactorMark({ size = 22, className }: { size?: number; className?: string }) {
  return (
    // eslint-disable-next-line @next/next/no-img-element -- small static brand mark; no layout shift math needed
    <img
      src="/icon-512.png"
      alt=""
      width={size}
      height={size}
      className={cn("block rounded-[5px]", className)}
      aria-hidden="true"
      draggable={false}
    />
  );
}

export function Wordmark({ className }: { className?: string }) {
  return (
    <div className={cn("flex select-none items-center", className)}>
      <span className="text-[15px] font-semibold leading-none tracking-[-0.015em] text-fg">
        Reactor
      </span>
    </div>
  );
}
