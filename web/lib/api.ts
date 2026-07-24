// Thin client for the engine HTTP API (docs/CONTRACT.md). All calls go to
// same-origin /api/* which next.config.mjs proxies to the Go engine on :8787.
// Every call is defensive: the engine may not be running, in which case the UI
// falls back to bundled fixtures (replay mode). Nothing here throws to the
// render tree — callers get null/false and decide.

import type {
  Artifact,
  DetonateResponse,
  Health,
  ScanResult,
} from "./events";

const TIMEOUT_MS = 2500;

async function getJSON<T>(path: string, timeout = TIMEOUT_MS): Promise<T | null> {
  try {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), timeout);
    const res = await fetch(path, {
      signal: ctrl.signal,
      headers: { accept: "application/json" },
      cache: "no-store",
    });
    clearTimeout(t);
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

/** Probe the engine. null means unreachable -> replay mode. */
export function getHealth(): Promise<Health | null> {
  return getJSON<Health>("/api/health", 1800);
}

export function getArtifacts(): Promise<Artifact[] | null> {
  return getJSON<Artifact[]>("/api/artifacts");
}

export function getScan(detonationId: string): Promise<ScanResult | null> {
  return getJSON<ScanResult>(`/api/scan?detonation=${encodeURIComponent(detonationId)}`);
}

export function getScorecard<T>(): Promise<T | null> {
  return getJSON<T>("/api/scorecard");
}

/** POST /api/detonate. Returns the detonation id, or null on failure. */
export async function detonate(body: {
  artifact_id?: string;
  artifact?: Artifact;
  sessions?: number;
  network?: boolean;
}): Promise<string | null> {
  try {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), TIMEOUT_MS);
    const res = await fetch("/api/detonate", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ sessions: 5, network: false, ...body }),
      signal: ctrl.signal,
    });
    clearTimeout(t);
    if (!res.ok) return null;
    const data = (await res.json()) as DetonateResponse;
    return data.detonation_id ?? null;
  } catch {
    return null;
  }
}
