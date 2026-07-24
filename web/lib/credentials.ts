// Browser-side BYOK credentials for Daytona (chamber) and Fireworks (victim +
// analyst models). Keys live in localStorage only, are never sent to Vercel,
// and only leave the browser on POST /api/detonate (and upload) toward the
// engine origin. The engine must not echo them into reports or SSE.

export const CREDENTIALS_STORAGE_KEY = "reactor-credentials";
export const ONBOARDING_DONE_KEY = "reactor-onboarding-done";

export interface Credentials {
  daytonaApiKey: string;
  /** Optional override; empty → engine default (https://app.daytona.io/api). */
  daytonaApiUrl: string;
  fireworksApiKey: string;
}

export const EMPTY_CREDENTIALS: Credentials = {
  daytonaApiKey: "",
  daytonaApiUrl: "",
  fireworksApiKey: "",
};

export function loadCredentials(): Credentials {
  if (typeof window === "undefined") return { ...EMPTY_CREDENTIALS };
  try {
    const raw = localStorage.getItem(CREDENTIALS_STORAGE_KEY);
    if (!raw) return { ...EMPTY_CREDENTIALS };
    const parsed = JSON.parse(raw) as Partial<Credentials>;
    return {
      daytonaApiKey: typeof parsed.daytonaApiKey === "string" ? parsed.daytonaApiKey.trim() : "",
      daytonaApiUrl: typeof parsed.daytonaApiUrl === "string" ? parsed.daytonaApiUrl.trim() : "",
      fireworksApiKey:
        typeof parsed.fireworksApiKey === "string" ? parsed.fireworksApiKey.trim() : "",
    };
  } catch {
    return { ...EMPTY_CREDENTIALS };
  }
}

export function saveCredentials(c: Credentials): void {
  if (typeof window === "undefined") return;
  const next: Credentials = {
    daytonaApiKey: c.daytonaApiKey.trim(),
    daytonaApiUrl: c.daytonaApiUrl.trim(),
    fireworksApiKey: c.fireworksApiKey.trim(),
  };
  try {
    localStorage.setItem(CREDENTIALS_STORAGE_KEY, JSON.stringify(next));
  } catch {
    /* private mode */
  }
}

export function clearCredentials(): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.removeItem(CREDENTIALS_STORAGE_KEY);
  } catch {
    /* private mode */
  }
}

export function isOnboardingDone(): boolean {
  if (typeof window === "undefined") return true;
  try {
    return localStorage.getItem(ONBOARDING_DONE_KEY) === "1";
  } catch {
    return true;
  }
}

export function markOnboardingDone(): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(ONBOARDING_DONE_KEY, "1");
  } catch {
    /* private mode */
  }
}

/** Any key present — enough to attempt a live run with BYOK. */
export function hasAnyCredential(c: Credentials = loadCredentials()): boolean {
  return Boolean(c.daytonaApiKey || c.fireworksApiKey);
}

export function hasDaytona(c: Credentials = loadCredentials()): boolean {
  return Boolean(c.daytonaApiKey);
}

export function hasFireworks(c: Credentials = loadCredentials()): boolean {
  return Boolean(c.fireworksApiKey);
}

/**
 * Shape the engine accepts under DetonateRequest.credentials (and mirrored on
 * upload as headers). Empty fields are omitted so a partial BYOK still works
 * with whatever the engine operator configured server-side.
 */
export function credentialsPayload(c: Credentials = loadCredentials()): {
  daytona_api_key?: string;
  daytona_api_url?: string;
  fireworks_api_key?: string;
} | undefined {
  const out: {
    daytona_api_key?: string;
    daytona_api_url?: string;
    fireworks_api_key?: string;
  } = {};
  if (c.daytonaApiKey) out.daytona_api_key = c.daytonaApiKey;
  if (c.daytonaApiUrl) out.daytona_api_url = c.daytonaApiUrl;
  if (c.fireworksApiKey) out.fireworks_api_key = c.fireworksApiKey;
  return out.daytona_api_key || out.fireworks_api_key ? out : undefined;
}

/** Headers form — used by upload (multipart) where a JSON body is not available. */
export function credentialsHeaders(c: Credentials = loadCredentials()): Record<string, string> {
  const h: Record<string, string> = {};
  if (c.daytonaApiKey) h["X-Reactor-Daytona-Key"] = c.daytonaApiKey;
  if (c.daytonaApiUrl) h["X-Reactor-Daytona-Url"] = c.daytonaApiUrl;
  if (c.fireworksApiKey) h["X-Reactor-Fireworks-Key"] = c.fireworksApiKey;
  return h;
}

/** Mask a key for display: keep a short prefix, hide the rest. */
export function maskKey(key: string): string {
  const k = key.trim();
  if (!k) return "";
  if (k.length <= 8) return "••••••••";
  return `${k.slice(0, 4)}…${k.slice(-4)}`;
}
