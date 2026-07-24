// Where the browser talks to the Go engine (docs/CONTRACT.md).
//
// There is no hosted engine. Reactor's control plane spawns process trees,
// tails collector logs and holds a multi-minute SSE stream per detonation, so
// it cannot live on a static host — see DEPLOY.md. Instead the *visitor* runs
// `./bin/reactor serve` on their own machine and the deployed console talks to
// it on loopback. Their keys, their Daytona account, their artifacts; nothing
// but the UI comes from us.
//
// Resolution order:
//   1. localStorage override      — set in the settings modal, no rebuild
//   2. NEXT_PUBLIC_ENGINE_URL     — baked at build time, for a shared engine
//   3. http://127.0.0.1:8787      — static export default (the visitor's own)
//   4. "" (same-origin /api/*)    — `next dev`, where next.config.mjs rewrites

const BUILD_ORIGIN = normalizeOrigin(process.env.NEXT_PUBLIC_ENGINE_URL || "");

/** Set by next.config.mjs whenever `output: "export"` is on. */
const IS_STATIC_EXPORT = process.env.NEXT_PUBLIC_STATIC_EXPORT === "1";

/** The engine's own default listen address (cmd/reactor: -addr). */
export const DEFAULT_LOCAL_ENGINE = "http://127.0.0.1:8787";

export const ENGINE_STORAGE_KEY = "reactor-engine-url";

/** Trim whitespace and any trailing slash; "" stays "". */
export function normalizeOrigin(raw: string): string {
  return (raw || "").trim().replace(/\/+$/, "");
}

/**
 * What the engine origin is when the visitor has not overridden it.
 *
 * In a static export there are no rewrites, so a same-origin `/api/*` would hit
 * the static 404 — the default has to be the visitor's own loopback engine. In
 * `next dev` the opposite is true: next.config.mjs proxies `/api/*`, so
 * same-origin is correct and an absolute origin would bypass the rewrite.
 */
export function defaultEngineOrigin(): string {
  if (BUILD_ORIGIN) return BUILD_ORIGIN;
  return IS_STATIC_EXPORT ? DEFAULT_LOCAL_ENGINE : "";
}

/** Absolute origin of the engine, or "" to stay same-origin. */
export function engineOrigin(): string {
  if (typeof window === "undefined") return defaultEngineOrigin();
  try {
    const stored = window.localStorage.getItem(ENGINE_STORAGE_KEY);
    // An explicitly stored empty string means "same-origin", which is a real
    // choice (console and engine behind one reverse proxy), so honour it.
    if (stored !== null) return normalizeOrigin(stored);
  } catch {
    /* private mode — fall through to the default */
  }
  return defaultEngineOrigin();
}

export function setEngineOrigin(raw: string): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(ENGINE_STORAGE_KEY, normalizeOrigin(raw));
  } catch {
    /* private mode */
  }
}

/** Drop the override so the build-time / loopback default applies again. */
export function clearEngineOrigin(): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.removeItem(ENGINE_STORAGE_KEY);
  } catch {
    /* private mode */
  }
}

/** Resolve an `/api/...` path against the configured engine (or same-origin). */
export function engineURL(path: string): string {
  const p = path.startsWith("/") ? path : `/${path}`;
  const origin = engineOrigin();
  return origin ? `${origin}${p}` : p;
}

/** True when the UI is wired to a remote engine (not same-origin). */
export function isRemoteEngine(): boolean {
  return Boolean(engineOrigin());
}

// ---- loopback / mixed-content situation ------------------------------------
//
// An https:// page fetching http://127.0.0.1 is mixed content, and browsers
// genuinely disagree about it. Chromium treats loopback as a potentially
// trustworthy origin and allows it; WebKit blocks it outright. Rather than
// maintain a browser allowlist that will drift, the console *probes* the engine
// and uses these helpers only to explain a failure it already observed.

/** True when the configured engine is a plain-http loopback address. */
export function isLoopbackEngine(origin: string = engineOrigin()): boolean {
  if (!origin.startsWith("http://")) return false;
  let host: string;
  try {
    host = new URL(origin).hostname;
  } catch {
    return false;
  }
  return host === "127.0.0.1" || host === "localhost" || host === "::1" || host === "[::1]";
}

/** True when the page is https:// but the engine is plain-http loopback. */
export function isMixedContentRisk(): boolean {
  if (typeof window === "undefined") return false;
  return window.location.protocol === "https:" && isLoopbackEngine();
}

/**
 * WebKit — Safari on macOS, and every browser on iOS (Chrome and Firefox there
 * are WebKit underneath, so they inherit the same block). This is the one
 * engine we can say definitively will not reach a loopback engine from https.
 */
export function isWebKit(
  ua: string = typeof navigator === "undefined" ? "" : navigator.userAgent,
): boolean {
  if (!ua) return false;
  if (/iPhone|iPad|iPod|CriOS|FxiOS|EdgiOS/i.test(ua)) return true;
  // Desktop Safari: "Safari" present, none of the Chromium/Gecko markers.
  return /Safari/i.test(ua) && !/Chrome|Chromium|Edg|OPR|Brave|Firefox|Android/i.test(ua);
}

/**
 * Why the engine is unreachable, as something a person can act on. Only ever
 * called after a probe has already failed — this names the likely cause, it
 * does not predict one.
 */
export function unreachableReason(): "webkit" | "mixed-content" | "no-engine" {
  if (isMixedContentRisk() && isWebKit()) return "webkit";
  if (isMixedContentRisk()) return "mixed-content";
  return "no-engine";
}
