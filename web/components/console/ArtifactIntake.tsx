"use client";

import { useCallback, useId, useRef, useState } from "react";
import { AlertTriangle, Check, FileUp, Info, Link2, Loader2, ShieldOff, X } from "lucide-react";
import { cn } from "@/lib/cn";
import type { Artifact } from "@/lib/events";
import { MAX_UPLOAD_BYTES, uploadArtifact, type UploadResponse } from "@/lib/api";

/**
 * What the intake hands back to the console. Three ways in, one shape out:
 *
 *   upload — a staged archive; detonate with `{ upload_id }`
 *   repo   — an https git url; detonate with `{ repo, ref }`
 *   spec   — a command a README told you to run; the existing inline-artifact
 *            path, `{ artifact }`, unchanged
 *
 * `artifact` is always the thing to show the person: for an upload it is the
 * artifact the engine itself parsed out of the archive; for the other two it is
 * a display stand-in built from what they typed.
 */
export interface IntakeTarget {
  source: "upload" | "repo" | "spec";
  artifact: Artifact;
  uploadId?: string;
  upload?: UploadResponse;
  repo?: string;
  ref?: string;
}

/** An ingested artifact is one the engine had to stage — upload or clone. */
export function isIngested(t: IntakeTarget | null): boolean {
  return t?.source === "upload" || t?.source === "repo";
}

const ACCEPT = ".zip,.tar,.tar.gz,.tgz,application/zip,application/x-tar,application/gzip";

const KIND_LABEL: Record<string, string> = {
  mcp_server: "MCP server",
  skill: "Agent skill",
  zip: "Archive",
};

function kindLabel(kind: string): string {
  return KIND_LABEL[kind] ?? (kind || "unknown kind");
}

function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n < 1024) return `${n} B`;
  const kib = n / 1024;
  if (kib < 1024) return `${kib < 10 ? kib.toFixed(1) : Math.round(kib)} KiB`;
  const mib = kib / 1024;
  return `${mib < 10 ? mib.toFixed(1) : Math.round(mib)} MiB`;
}

function formatExpiry(expiresMs: number): string {
  const left = expiresMs - Date.now();
  if (!Number.isFinite(left) || left <= 0) return "expired";
  const mins = Math.round(left / 60000);
  if (mins < 60) return `staged for ${mins} more min`;
  return `staged for ${Math.floor(mins / 60)}h ${mins % 60}m`;
}

// ---- what did they type? ---------------------------------------------------

type Parsed =
  | { kind: "empty" }
  | { kind: "repo"; url: string; label: string }
  | { kind: "spec"; command: string; name: string }
  | { kind: "refused"; message: string };

// A command pasted out of a README, rather than a url.
const RUNNER = /^(npx|npm|pnpm|yarn|bunx|bun|uvx|uv|node|python3?|deno)\s/i;

function packageOf(command: string): string {
  const parts = command.trim().split(/\s+/).slice(1);
  return parts.find((p) => !p.startsWith("-")) ?? command;
}

function repoLabel(url: string): string {
  try {
    const u = new URL(url);
    const parts = u.pathname.replace(/\.git$/, "").split("/").filter(Boolean);
    if (parts.length >= 2) return `${parts[0]}/${parts[1]}`;
    return `${u.hostname}${u.pathname}`.replace(/\/$/, "");
  } catch {
    return url;
  }
}

/**
 * Decide between the clone path and the inline-artifact path from what the
 * person actually typed. The two refusals here are the ones worth catching in
 * the browser: they are policy, not a typo, so there is no point spending a
 * round trip to hear the engine say the same thing.
 */
export function parseIntakeValue(raw: string): Parsed {
  const v = raw.trim();
  if (!v) return { kind: "empty" };
  if (RUNNER.test(v)) return { kind: "spec", command: v, name: packageOf(v) };
  if (/^(git@|ssh:\/\/|git\+ssh:\/\/|git:\/\/)/i.test(v)) {
    return {
      kind: "refused",
      message:
        "Reactor clones over https only. An ssh remote authenticates as whoever runs the engine, so it is refused by design — paste the https:// url instead.",
    };
  }
  if (/^file:\/\//i.test(v)) {
    return {
      kind: "refused",
      message: "A local path cannot be cloned. Zip the directory and drop it above instead.",
    };
  }
  if (/^https?:\/\//i.test(v)) return { kind: "repo", url: v, label: repoLabel(v) };
  if (/^(www\.)?[a-z0-9-]+(\.[a-z0-9-]+)+\/\S+$/i.test(v)) {
    const url = `https://${v.replace(/^www\./i, "")}`;
    return { kind: "repo", url, label: repoLabel(url) };
  }
  // A scoped or bare npm package name — the thing a README tells you to npx.
  if (v.startsWith("@") || !v.includes("/")) {
    return { kind: "spec", command: `npx -y ${v}`, name: v };
  }
  if (/^[\w.-]+\/[\w.-]+$/.test(v)) {
    const url = `https://github.com/${v.replace(/\.git$/, "")}`;
    return { kind: "repo", url, label: repoLabel(url) };
  }
  return {
    kind: "refused",
    message: "Paste an https GitHub url, or a command like npx -y @acme/notes-mcp.",
  };
}

// ---- the intake ------------------------------------------------------------

/**
 * Primary artifact intake: drop a file, or point at a repository. The zoo below
 * it is the sample rack, not the front door.
 *
 * Both ingest paths genuinely need a live engine — there is nothing to fake, so
 * when the console is running on bundled fixtures the whole block says so and
 * turns itself off rather than failing at the click.
 */
export function ArtifactIntake({
  mode,
  target,
  onTarget,
  disabled,
}: {
  mode: "probing" | "live" | "replay";
  target: IntakeTarget | null;
  onTarget: (t: IntakeTarget | null) => void;
  disabled: boolean;
}) {
  const live = mode === "live";
  const off = !live || disabled;

  const [dragging, setDragging] = useState(false);
  const [progress, setProgress] = useState<number | null>(null);
  const [pendingName, setPendingName] = useState("");
  const [fileError, setFileError] = useState<string | null>(null);
  const [urlError, setUrlError] = useState<string | null>(null);
  const [status, setStatus] = useState("");
  const [url, setUrl] = useState("");
  const [gitRef, setGitRef] = useState("");

  const abortRef = useRef<AbortController | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const fileId = useId();
  const fileHintId = useId();
  const urlId = useId();
  const refId = useId();

  const uploaded = target?.source === "upload" ? target.upload : undefined;
  const parsed = parseIntakeValue(url);
  // Once what is typed *is* what is armed, the summary below says it all — the
  // hint would just be the same sentence twice.
  const armedSame =
    (target?.source === "repo" &&
      parsed.kind === "repo" &&
      target.repo === parsed.url &&
      (target.ref ?? "") === gitRef.trim()) ||
    (target?.source === "spec" &&
      parsed.kind === "spec" &&
      target.artifact.source === parsed.command);

  const startUpload = useCallback(
    async (file: File) => {
      setFileError(null);
      setUrlError(null);
      if (file.size > MAX_UPLOAD_BYTES) {
        const msg = `${file.name} is ${formatBytes(file.size)} — the ceiling is 64 MiB per upload.`;
        setFileError(msg);
        setStatus(`Upload refused. ${msg}`);
        return;
      }
      onTarget(null);
      setPendingName(file.name);
      setProgress(0);
      setStatus(`Uploading ${file.name}…`);

      const ctrl = new AbortController();
      abortRef.current = ctrl;
      const res = await uploadArtifact(file, {
        onProgress: (f) => setProgress(f),
        signal: ctrl.signal,
      });
      abortRef.current = null;
      setProgress(null);

      if (res.aborted) {
        setStatus("Upload canceled.");
        return;
      }
      if (!res.upload) {
        const msg = res.error ?? "the upload failed";
        setFileError(msg);
        setStatus(`Upload refused. ${msg}`);
        return;
      }
      const u = res.upload;
      onTarget({
        source: "upload",
        uploadId: u.upload_id,
        upload: u,
        artifact: u.artifact,
      });
      setStatus(
        `${u.name} staged: ${kindLabel(u.kind)}, ${u.files} files, ${formatBytes(u.size_bytes)}. Ready to detonate.`,
      );
    },
    [onTarget],
  );

  const takeFiles = useCallback(
    (files: FileList | null) => {
      if (off || !files || files.length === 0) return;
      if (files.length > 1) {
        setFileError("One archive per upload — send the files as a single zip or tar.");
        setStatus("Upload refused. One archive per upload.");
        return;
      }
      void startUpload(files[0]);
    },
    [off, startUpload],
  );

  const cancelUpload = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
  }, []);

  const useTyped = useCallback(() => {
    if (off) return;
    const p = parseIntakeValue(url);
    if (p.kind === "empty") return;
    if (p.kind === "refused") {
      setUrlError(p.message);
      setStatus(`Refused. ${p.message}`);
      return;
    }
    setUrlError(null);
    setFileError(null);
    if (p.kind === "repo") {
      const ref = gitRef.trim();
      onTarget({
        source: "repo",
        repo: p.url,
        ref: ref || undefined,
        artifact: { id: "", kind: "", name: p.label, source: p.url, sha256: "" },
      });
      setStatus(`${p.label} armed. It is cloned when you detonate, at ${ref || "the default branch"}.`);
      return;
    }
    onTarget({
      source: "spec",
      artifact: { id: "", kind: "mcp_server", name: p.name, source: p.command, sha256: "" },
    });
    setStatus(`${p.name} armed. The chamber will run ${p.command}.`);
  }, [off, url, gitRef, onTarget, setStatus]);

  const clearTarget = useCallback(() => {
    onTarget(null);
    setStatus("Cleared.");
  }, [onTarget]);

  const uploading = progress !== null;
  const boxState: "idle" | "dragging" | "uploading" | "error" | "success" = uploading
    ? "uploading"
    : dragging
      ? "dragging"
      : fileError
        ? "error"
        : uploaded
          ? "success"
          : "idle";

  return (
    <div className="flex max-w-3xl flex-col gap-3">
      {/* engine provenance — the one thing that decides whether intake works */}
      {!live && (
        <p className="flex items-start gap-2 text-sm leading-relaxed text-muted">
          <Info size={14} className="mt-1 shrink-0 text-faint" aria-hidden="true" />
          <span>
            {mode === "probing"
              ? "Looking for a live engine — upload and clone need one."
              : "Upload and repository intake need a live engine; the console is on bundled fixtures right now. Try a sample artifact below."}
          </span>
        </p>
      )}

      {/* ---- drop zone ---- */}
      <div className={cn("relative", off && "opacity-55")}>
        <input
          id={fileId}
          ref={inputRef}
          type="file"
          accept={ACCEPT}
          disabled={off}
          aria-label="Upload an artifact archive: zip, tar or tar.gz, up to 64 MiB"
          aria-describedby={fileHintId}
          className="peer sr-only"
          onChange={(e) => {
            takeFiles(e.target.files);
            e.target.value = "";
          }}
        />
        <div
          onDragEnter={(e) => {
            if (off) return;
            e.preventDefault();
            setDragging(true);
          }}
          onDragOver={(e) => {
            if (off) return;
            e.preventDefault();
            if (!dragging) setDragging(true);
          }}
          onDragLeave={(e) => {
            if (e.currentTarget.contains(e.relatedTarget as Node | null)) return;
            setDragging(false);
          }}
          onDrop={(e) => {
            if (off) return;
            e.preventDefault();
            setDragging(false);
            takeFiles(e.dataTransfer?.files ?? null);
          }}
          className={cn(
            "rounded-2xl border border-dashed transition-colors duration-200 ease-instrument",
            "peer-focus-visible:ring-2 peer-focus-visible:ring-accent/60 peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-bg",
            boxState === "dragging"
              ? "border-fg/40 bg-surface-2"
              : boxState === "error"
                ? "border-danger/40 bg-surface-2/40"
                : boxState === "success"
                  ? "border-line-strong bg-surface-2/60"
                  : "border-line-strong bg-surface-2/40",
          )}
        >
          {boxState === "uploading" && (
            <div className="flex flex-col gap-2.5 px-4 py-4">
              <div className="flex min-w-0 items-center gap-2.5">
                <Loader2 size={14} className="shrink-0 animate-spin-slow text-muted" aria-hidden="true" />
                <span className="min-w-0 flex-1 truncate font-mono text-sm text-fg">{pendingName}</span>
                <span className="tnum shrink-0 text-sm text-muted">
                  {Math.round((progress ?? 0) * 100)}%
                </span>
                <button
                  type="button"
                  onClick={cancelUpload}
                  className="focus-ring shrink-0 rounded-lg px-2 py-1 text-sm font-medium text-muted transition-colors duration-200 ease-instrument hover:bg-surface-3 hover:text-fg"
                >
                  Cancel
                </button>
              </div>
              <div className="h-1 overflow-hidden rounded-full bg-surface-3">
                <div
                  className="h-full rounded-full bg-fg/60 transition-[width] duration-200 ease-instrument"
                  style={{ width: `${Math.round((progress ?? 0) * 100)}%` }}
                />
              </div>
              <p className="text-sm leading-relaxed text-muted">
                Hashed as it arrives, then unpacked in a dry run so a hostile entry is refused
                before any chamber sees it.
              </p>
            </div>
          )}

          {boxState === "success" && uploaded && (
            <div className="flex flex-col gap-2.5 px-4 py-3.5">
              <div className="flex min-w-0 items-center gap-2.5">
                <Check size={14} className="shrink-0 text-fg" aria-hidden="true" />
                <span className="min-w-0 flex-1 truncate font-mono text-sm font-medium text-fg">
                  {uploaded.name}
                </span>
                <button
                  type="button"
                  onClick={() => inputRef.current?.click()}
                  className="focus-ring shrink-0 rounded-lg px-2 py-1 text-sm font-medium text-muted transition-colors duration-200 ease-instrument hover:bg-surface-3 hover:text-fg"
                >
                  Replace
                </button>
                <button
                  type="button"
                  onClick={clearTarget}
                  aria-label="Clear the staged upload"
                  className="focus-ring shrink-0 rounded-lg p-1 text-muted transition-colors duration-200 ease-instrument hover:bg-surface-3 hover:text-fg"
                >
                  <X size={14} aria-hidden="true" />
                </button>
              </div>

              {/* what the engine actually parsed — not what the filename claimed */}
              <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 text-sm">
                <Fact label="kind">{kindLabel(uploaded.kind)}</Fact>
                <Fact label="archive">{uploaded.archive}</Fact>
                <Fact label="size" mono>
                  {formatBytes(uploaded.size_bytes)}
                </Fact>
                <Fact label="files" mono>
                  {uploaded.files}
                </Fact>
                <Fact label="sha256" mono>
                  {uploaded.sha256.slice(0, 12)}…
                </Fact>
              </div>

              {uploaded.source && (
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1 text-sm text-muted">
                  <span className="text-faint">runs</span>
                  <code className="code-chip">{uploaded.source}</code>
                  {uploaded.install && (
                    <>
                      <span className="text-faint">after</span>
                      <code className="code-chip">{uploaded.install}</code>
                    </>
                  )}
                </div>
              )}

              <p className="text-sm leading-relaxed text-muted">
                {uploaded.skipped_entries > 0 && (
                  <>
                    <span className="tnum">{uploaded.skipped_entries}</span> entries refused
                    (symlinks, hardlinks or device nodes) and left out of the chamber ·{" "}
                  </>
                )}
                {formatExpiry(uploaded.expires_ms)} · detonate it as often as you like
              </p>
            </div>
          )}

          {(boxState === "idle" || boxState === "dragging" || boxState === "error") && (
            <label
              htmlFor={fileId}
              className={cn(
                "flex flex-col items-center gap-1.5 px-5 py-7 text-center",
                off ? "cursor-not-allowed" : "cursor-pointer",
              )}
            >
              {boxState === "error" ? (
                <AlertTriangle size={18} className="text-danger" aria-hidden="true" />
              ) : (
                <FileUp size={18} className="text-faint" aria-hidden="true" />
              )}
              <span className="text-base text-fg">
                {boxState === "dragging" ? (
                  "Release to stage the archive"
                ) : boxState === "error" ? (
                  "Reactor refused that file"
                ) : (
                  <>
                    Drop an artifact archive, or{" "}
                    <span className="underline decoration-line-strong underline-offset-4">
                      browse
                    </span>
                  </>
                )}
              </span>
              <span id={fileHintId} className="text-sm leading-relaxed text-muted">
                zip, tar or tar.gz · up to 64 MiB · staged for two hours, then swept
              </span>
            </label>
          )}
        </div>
      </div>

      {fileError && (
        <p role="alert" className="flex items-start gap-2 text-sm leading-relaxed text-danger">
          <AlertTriangle size={14} className="mt-1 shrink-0" aria-hidden="true" />
          <span>{fileError}</span>
        </p>
      )}

      {/* ---- url / spec ---- */}
      <div className={cn("flex flex-col gap-2", off && "opacity-55")}>
        <label htmlFor={urlId} className="strip-label">
          Or point at a repository
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-[16rem] flex-1">
            <Link2
              size={14}
              aria-hidden="true"
              className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint"
            />
            <input
              id={urlId}
              type="text"
              value={url}
              disabled={off}
              spellCheck={false}
              autoComplete="off"
              placeholder="https://github.com/owner/repo — or npx -y @acme/notes-mcp"
              onChange={(e) => {
                setUrl(e.target.value);
                if (urlError) setUrlError(null);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  useTyped();
                }
              }}
              className="focus-ring w-full rounded-xl bg-surface-2 py-2 pl-9 pr-3 font-mono text-sm text-fg transition-colors duration-200 ease-instrument placeholder:font-sans placeholder:text-faint hover:bg-surface-2/80 disabled:cursor-not-allowed"
            />
          </div>
          {parsed.kind !== "spec" && (
            <input
              id={refId}
              type="text"
              value={gitRef}
              disabled={off}
              spellCheck={false}
              autoComplete="off"
              aria-label="Branch, tag or commit (optional)"
              placeholder="branch or tag"
              onChange={(e) => setGitRef(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  useTyped();
                }
              }}
              className="focus-ring w-36 rounded-xl bg-surface-2 px-3 py-2 font-mono text-sm text-fg transition-colors duration-200 ease-instrument placeholder:font-sans placeholder:text-faint disabled:cursor-not-allowed"
            />
          )}
          <button
            type="button"
            onClick={useTyped}
            disabled={off || parsed.kind === "empty" || armedSame}
            className="focus-ring shrink-0 rounded-xl bg-surface-2 px-3.5 py-2 text-sm font-medium text-fg transition-colors duration-200 ease-instrument hover:bg-surface-3 disabled:pointer-events-none disabled:opacity-40"
          >
            Use
          </button>
        </div>

        {urlError ? (
          <p role="alert" className="flex items-start gap-2 text-sm leading-relaxed text-danger">
            <AlertTriangle size={14} className="mt-1 shrink-0" aria-hidden="true" />
            <span>{urlError}</span>
          </p>
        ) : armedSame ? null : (
          <p className="text-sm leading-relaxed text-muted">
            {parsed.kind === "repo" ? (
              <>
                Clones <span className="font-mono text-fg">{parsed.label}</span> at{" "}
                <span className="font-mono text-fg">{gitRef.trim() || "the default branch"}</span>{" "}
                when you detonate.
              </>
            ) : parsed.kind === "spec" ? (
              <>
                Runs <code className="code-chip">{parsed.command}</code> inside the chamber —
                nothing is uploaded or cloned.
              </>
            ) : parsed.kind === "refused" ? (
              parsed.message
            ) : (
              <>
                Public https repositories only — ssh, <span className="font-mono">git@</span> and
                urls carrying credentials are refused by design. Shallow clone, 90s ceiling, no
                submodules or hooks.
              </>
            )}
          </p>
        )}
      </div>

      {/* ---- armed repo / spec ---- */}
      {target && target.source !== "upload" && (
        <div className="flex flex-col gap-1.5 rounded-xl bg-surface-2/50 px-3.5 py-3">
          <div className="flex min-w-0 items-center gap-2.5">
            <Check size={14} className="shrink-0 text-fg" aria-hidden="true" />
            <span className="min-w-0 flex-1 truncate font-mono text-sm font-medium text-fg">
              {target.artifact.name}
            </span>
            <button
              type="button"
              onClick={clearTarget}
              aria-label="Clear the armed source"
              className="focus-ring shrink-0 rounded-lg p-1 text-muted transition-colors duration-200 ease-instrument hover:bg-surface-3 hover:text-fg"
            >
              <X size={14} aria-hidden="true" />
            </button>
          </div>
          {target.source === "repo" ? (
            <>
              <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 text-sm">
                <Fact label="clone" mono>
                  {target.repo}
                </Fact>
                <Fact label="ref" mono>
                  {target.ref ?? "default branch"}
                </Fact>
              </div>
              <p className="text-sm leading-relaxed text-muted">
                Cloned when you detonate — depth 1, no submodules, no hooks, and{" "}
                <span className="font-mono">.git</span> removed before the tree reaches a chamber.
                Kind and entrypoint come from the tree, not from you.
              </p>
            </>
          ) : (
            <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 text-sm">
              <Fact label="runs" mono>
                {target.artifact.source}
              </Fact>
              <Fact label="kind">{kindLabel(target.artifact.kind)}</Fact>
            </div>
          )}
        </div>
      )}

      {/* ---- the honest footnote about the left column ---- */}
      {isIngested(target) && (
        <p className="flex items-start gap-2 text-sm leading-relaxed text-muted">
          <ShieldOff size={14} className="mt-1 shrink-0 text-faint" aria-hidden="true" />
          <span>
            No static baseline for this one. <span className="font-mono">mcp-scan</span> has to
            install and launch a server on the host to read its tool descriptions, and Reactor will
            not do that with a stranger&rsquo;s code — the scan column reports{" "}
            <span className="font-mono">unavailable</span>. Sample artifacts still get the real
            side-by-side.
          </span>
        </p>
      )}

      {/* Upload progress, refusals and arming, announced once each. */}
      <span className="sr-only" role="status" aria-live="polite">
        {status}
      </span>
    </div>
  );
}

/** A machine fact: faint sans label, then the value. Mono for machine data. */
function Fact({
  label,
  children,
  mono,
}: {
  label: string;
  children: React.ReactNode;
  mono?: boolean;
}) {
  return (
    <span className="inline-flex min-w-0 items-baseline gap-1.5">
      <span className="shrink-0 text-faint">{label}</span>
      <span className={cn("min-w-0 truncate text-fg", mono && "tnum font-mono text-2xs")}>
        {children}
      </span>
    </span>
  );
}
