// Thin client for the engine HTTP API (docs/CONTRACT.md).
//
// Locally, paths are same-origin `/api/*` and next.config.mjs rewrites them to
// the Go engine. On Vercel, NEXT_PUBLIC_ENGINE_URL points the browser at the
// engine origin directly (lib/engine.ts) so SSE is not proxied through the edge.
//
// BYOK credentials (Daytona + Fireworks) ride on detonate as a JSON field and
// on upload as X-Reactor-* headers. They never touch the Vercel host beyond the
// static JS bundle.

import type {
  Artifact,
  DetonateResponse,
  Health,
  ScanResult,
} from "./events";
import { credentialsHeaders, credentialsPayload, loadCredentials } from "./credentials";
import { engineURL } from "./engine";

const TIMEOUT_MS = 2500;

// Ingest is not a lookup: POST /api/detonate with a repo clones it inline
// (CloneTimeout 90s), and an upload is re-extracted before the id comes back.
// 2.5s would abort work the engine is legitimately still doing.
const INGEST_TIMEOUT_MS = 100_000;

/** MaxUploadBytes, engine default (docs/CONTRACT.md). One file per upload. */
export const MAX_UPLOAD_BYTES = 64 * 1024 * 1024;

async function getJSON<T>(path: string, timeout = TIMEOUT_MS): Promise<T | null> {
  try {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), timeout);
    const res = await fetch(engineURL(path), {
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

// ---- ingest: upload + detonate --------------------------------------------

/**
 * The engine reports failure as `text/plain` on every endpoint, written for a
 * person to read and already free of host paths (CONTRACT.md). So the rule for
 * the ingest calls is: read the body, show the body. This only invents a
 * sentence when the engine said nothing, or when something that is not the
 * engine (a proxy, a dev-server error page) answered instead.
 */
function engineMessage(raw: string, status: number): string {
  const text = raw.trim();
  const usable = text && !text.startsWith("<") && !text.startsWith("{");
  if (usable) return text.length > 400 ? `${text.slice(0, 400)}…` : text;
  switch (status) {
    case 404:
      return "that upload is gone — uploads are kept for two hours, so send the file again";
    case 413:
      return "that is over the size ceiling (64 MiB per upload)";
    case 415:
      return "unsupported archive type — send a zip, tar or tar.gz";
    case 504:
      return "the clone timed out after 90s";
    default:
      return `the engine refused the request (HTTP ${status})`;
  }
}

const UNREACHABLE = "the engine is not reachable — upload and clone need a live engine";

/** POST /api/upload response (CONTRACT.md § Artifact ingest). */
export interface UploadResponse {
  upload_id: string;
  name: string;
  kind: string;
  archive: string; // zip | tar | tar.gz
  sha256: string;
  size_bytes: number;
  files: number;
  unpacked_bytes: number;
  skipped_entries: number;
  source: string;
  install?: string;
  expires_ms: number;
  artifact: Artifact;
}

export interface UploadOutcome {
  upload: UploadResponse | null;
  /** The engine's own words. null when the caller aborted. */
  error: string | null;
  status: number;
  aborted?: boolean;
}

/**
 * POST /api/upload — one archive as multipart/form-data.
 *
 * XHR rather than fetch because a 64 MiB upload deserves a real progress
 * reading, and `fetch` still cannot report request progress. Nothing throws:
 * callers get the engine's message and decide.
 */
export function uploadArtifact(
  file: File,
  opts: {
    fields?: Record<string, string | undefined>;
    onProgress?: (fraction: number) => void;
    signal?: AbortSignal;
  } = {},
): Promise<UploadOutcome> {
  return new Promise((resolve) => {
    const form = new FormData();
    for (const [k, v] of Object.entries(opts.fields ?? {})) {
      if (v) form.append(k, v);
    }
    // The filename is display-only to the engine; the field name is free.
    form.append("file", file, file.name);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", engineURL("/api/upload"));
    xhr.responseType = "text";
    xhr.setRequestHeader("accept", "application/json, text/plain");
    for (const [k, v] of Object.entries(credentialsHeaders(loadCredentials()))) {
      xhr.setRequestHeader(k, v);
    }

    if (opts.onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && e.total > 0) opts.onProgress?.(e.loaded / e.total);
      };
    }
    xhr.onload = () => {
      const body = typeof xhr.responseText === "string" ? xhr.responseText : "";
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve({ upload: JSON.parse(body) as UploadResponse, error: null, status: xhr.status });
        } catch {
          resolve({
            upload: null,
            error: "the engine answered with something this build could not read",
            status: xhr.status,
          });
        }
        return;
      }
      resolve({ upload: null, error: engineMessage(body, xhr.status), status: xhr.status });
    };
    xhr.onerror = () => resolve({ upload: null, error: UNREACHABLE, status: 0 });
    xhr.ontimeout = () => resolve({ upload: null, error: "the upload timed out", status: 0 });
    xhr.onabort = () => resolve({ upload: null, error: null, status: 0, aborted: true });

    if (opts.signal) {
      if (opts.signal.aborted) {
        resolve({ upload: null, error: null, status: 0, aborted: true });
        return;
      }
      opts.signal.addEventListener("abort", () => xhr.abort(), { once: true });
    }
    xhr.send(form);
  });
}

/**
 * The four ways to name what should be detonated: a zoo id, a full inline
 * artifact, a staged upload, or an https repository url (CONTRACT.md).
 */
export interface DetonateBody {
  artifact_id?: string;
  artifact?: Artifact;
  upload_id?: string;
  repo?: string;
  ref?: string;
  sessions?: number;
  network?: boolean;
}

export interface DetonateOutcome {
  id: string | null;
  /** The engine's own words, or null on success. */
  error: string | null;
  status: number;
}

/**
 * POST /api/detonate, surfacing the refusal. Ingest requests are the reason
 * this exists: a bad archive entry, an ssh remote or an expired upload is a
 * thing the person can fix, so the engine's sentence has to reach them rather
 * than being swallowed into a silent fixture replay.
 *
 * Visitor BYOK credentials (localStorage) are attached under `credentials` so
 * the engine can provision a Daytona sandbox and call Fireworks under their
 * account without the host operator holding those keys.
 */
export async function detonateWithError(body: DetonateBody): Promise<DetonateOutcome> {
  const ingest = Boolean(body.repo || body.upload_id);
  const creds = credentialsPayload(loadCredentials());
  try {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), ingest ? INGEST_TIMEOUT_MS : TIMEOUT_MS);
    const res = await fetch(engineURL("/api/detonate"), {
      method: "POST",
      headers: {
        "content-type": "application/json",
        ...credentialsHeaders(loadCredentials()),
      },
      body: JSON.stringify({
        sessions: 5,
        network: false,
        ...body,
        ...(creds ? { credentials: creds } : {}),
      }),
      signal: ctrl.signal,
    });
    clearTimeout(t);
    const text = await res.text();
    if (!res.ok) return { id: null, error: engineMessage(text, res.status), status: res.status };
    try {
      const data = JSON.parse(text) as DetonateResponse;
      return data.detonation_id
        ? { id: data.detonation_id, error: null, status: res.status }
        : { id: null, error: "the engine did not return a detonation id", status: res.status };
    } catch {
      return {
        id: null,
        error: "the engine answered with something this build could not read",
        status: res.status,
      };
    }
  } catch {
    return { id: null, error: UNREACHABLE, status: 0 };
  }
}

/** POST /api/detonate. Returns the detonation id, or null on failure. */
export async function detonate(body: DetonateBody): Promise<string | null> {
  return (await detonateWithError(body)).id;
}
