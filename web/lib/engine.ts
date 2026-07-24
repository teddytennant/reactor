// Where the browser talks to the Go engine (docs/CONTRACT.md).
//
// Local dev: leave NEXT_PUBLIC_ENGINE_URL unset (or pointed only at next.config
// rewrites). The UI hits same-origin `/api/*`, and next.config.mjs proxies to
// the engine on :8787.
//
// Vercel / reactor.teddytennant.com: set NEXT_PUBLIC_ENGINE_URL to the public
// engine origin (e.g. https://engine.example.com). The browser then calls the
// engine directly — CORS is open on the engine, and SSE cannot reliably stream
// through Vercel's proxy for a multi-minute detonation.

const RAW = (process.env.NEXT_PUBLIC_ENGINE_URL || "").trim().replace(/\/$/, "");

/**
 * Absolute origin of a remote engine, or "" when the UI should stay on
 * same-origin `/api/*` (local rewrite).
 */
export function engineOrigin(): string {
  return RAW;
}

/** Resolve an `/api/...` path against the configured engine (or same-origin). */
export function engineURL(path: string): string {
  const p = path.startsWith("/") ? path : `/${path}`;
  return RAW ? `${RAW}${p}` : p;
}

/** True when the UI is wired to a remote engine (production / staged demo). */
export function isRemoteEngine(): boolean {
  return Boolean(RAW);
}
