"use client";

import { useEffect, useId, useState } from "react";
import { Check, ExternalLink, KeyRound, X } from "lucide-react";
import { cn } from "@/lib/cn";
import {
  EMPTY_CREDENTIALS,
  clearCredentials,
  hasDaytona,
  hasFireworks,
  loadCredentials,
  markOnboardingDone,
  saveCredentials,
  type Credentials,
} from "@/lib/credentials";
import { DEFAULT_LOCAL_ENGINE, engineOrigin, setEngineOrigin } from "@/lib/engine";

type Mode = "onboarding" | "settings";

export interface CredentialsModalProps {
  open: boolean;
  mode: Mode;
  onClose: () => void;
  /** Fired after save or skip so the console can refresh badges. */
  onChange?: (c: Credentials) => void;
}

/**
 * First-run onboarding + settings. Asks for Daytona (disposable chamber) and
 * Fireworks (victim + analyst models). Keys stay in localStorage and only leave
 * the browser toward the engine on detonate/upload — never to Vercel.
 */
export function CredentialsModal({ open, mode, onClose, onChange }: CredentialsModalProps) {
  const titleId = useId();
  const [draft, setDraft] = useState<Credentials>(EMPTY_CREDENTIALS);
  const [engineDraft, setEngineDraft] = useState("");
  const [showKeys, setShowKeys] = useState(false);
  const [savedFlash, setSavedFlash] = useState(false);

  useEffect(() => {
    if (!open) return;
    setDraft(loadCredentials());
    setEngineDraft(engineOrigin());
    setShowKeys(false);
    setSavedFlash(false);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  const isOnboarding = mode === "onboarding";

  const commit = (next: Credentials, done: boolean) => {
    saveCredentials(next);
    // Blank means "use the default" rather than "use same-origin", which for a
    // deployed static export would be the 404 page.
    setEngineOrigin(engineDraft.trim() || DEFAULT_LOCAL_ENGINE);
    if (done) markOnboardingDone();
    onChange?.(next);
  };

  const save = () => {
    commit(draft, true);
    setSavedFlash(true);
    window.setTimeout(() => {
      setSavedFlash(false);
      onClose();
    }, 480);
  };

  const skip = () => {
    markOnboardingDone();
    onChange?.(loadCredentials());
    onClose();
  };

  const clear = () => {
    clearCredentials();
    const empty = { ...EMPTY_CREDENTIALS };
    setDraft(empty);
    onChange?.(empty);
  };

  const set =
    (field: keyof Credentials) =>
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setDraft((d) => ({ ...d, [field]: e.target.value }));
    };

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-bg/70 p-4 backdrop-blur-sm sm:items-center"
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !isOnboarding) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="panel relative flex w-full max-w-lg flex-col overflow-hidden shadow-panel"
      >
        <div className="flex items-start gap-4 px-6 pb-5 pt-6">
          <div className="min-w-0 flex-1">
            <h2 id={titleId} className="text-lg font-semibold tracking-tight text-fg">
              {isOnboarding ? "Bring your own keys" : "API keys"}
            </h2>
            <p className="mt-1.5 text-sm leading-relaxed text-muted">
              {isOnboarding
                ? "Reactor runs detonations in your Daytona sandboxes and drives victim + analyst models on Fireworks. Keys stay in this browser."
                : "Stored only in this browser’s localStorage. Sent to the engine on detonate — never to the Vercel frontend host."}
            </p>
          </div>
          {!isOnboarding && (
            <button
              type="button"
              onClick={onClose}
              aria-label="Close"
              className="focus-ring -mr-2 -mt-2 grid h-8 w-8 shrink-0 place-items-center rounded-lg text-faint transition-colors hover:bg-surface-2 hover:text-fg"
            >
              <X size={16} aria-hidden="true" />
            </button>
          )}
        </div>

        <div className="flex flex-col gap-4 px-6 pb-5">
          <Field
            label="Daytona API key"
            hint="Disposable sandbox per detonation"
            htmlFor="cred-daytona-key"
            docs="https://www.daytona.io/docs"
          >
            <input
              id="cred-daytona-key"
              type={showKeys ? "text" : "password"}
              autoComplete="off"
              spellCheck={false}
              placeholder="dtn_…"
              value={draft.daytonaApiKey}
              onChange={set("daytonaApiKey")}
              className="field-input font-mono text-sm"
            />
          </Field>

          <Field
            label="Daytona API URL"
            hint="Leave blank for the default cloud API"
            htmlFor="cred-daytona-url"
          >
            <input
              id="cred-daytona-url"
              type="url"
              autoComplete="off"
              spellCheck={false}
              placeholder="https://app.daytona.io/api"
              value={draft.daytonaApiUrl}
              onChange={set("daytonaApiUrl")}
              className="field-input font-mono text-sm"
            />
          </Field>

          <Field
            label="Fireworks API key"
            hint="Victim agent + analyst model"
            htmlFor="cred-fireworks-key"
            docs="https://fireworks.ai/account/api-keys"
          >
            <input
              id="cred-fireworks-key"
              type={showKeys ? "text" : "password"}
              autoComplete="off"
              spellCheck={false}
              placeholder="fw_…"
              value={draft.fireworksApiKey}
              onChange={set("fireworksApiKey")}
              className="field-input font-mono text-sm"
            />
          </Field>

          <label className="group flex w-fit cursor-pointer select-none items-center gap-2.5 pt-0.5 text-sm text-muted transition-colors hover:text-fg">
            <input
              type="checkbox"
              checked={showKeys}
              onChange={(e) => setShowKeys(e.target.checked)}
              className="peer sr-only"
            />
            <span
              aria-hidden="true"
              className={cn(
                "grid h-[18px] w-[18px] shrink-0 place-items-center rounded-md border transition-colors",
                "peer-focus-visible:ring-2 peer-focus-visible:ring-fg/55 peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-surface",
                showKeys
                  ? "border-fg bg-fg text-bg"
                  : "border-line-strong bg-surface-2 text-transparent group-hover:border-faint",
              )}
            >
              <Check size={12} strokeWidth={3} />
            </span>
            Show keys
          </label>

          <div className="border-t border-line pt-4">
            <Field
              label="Engine URL"
              hint="Your own machine — nothing is hosted"
              htmlFor="cred-engine-url"
            >
              <input
                id="cred-engine-url"
                type="url"
                autoComplete="off"
                spellCheck={false}
                placeholder={DEFAULT_LOCAL_ENGINE}
                value={engineDraft}
                onChange={(e) => setEngineDraft(e.target.value)}
                className="field-input font-mono text-sm"
              />
            </Field>
            <p className="mt-2 text-sm leading-relaxed text-faint">
              Reactor detonates in disposable sandboxes and streams evidence for minutes at a
              time, so the engine runs on your machine, not ours. Clone the repo and start it:
            </p>
            <pre className="mt-2 overflow-x-auto rounded-lg bg-surface-2 px-3 py-2 font-mono text-xs text-muted">
              make build &amp;&amp; ./bin/reactor serve
            </pre>
          </div>

          <p className="text-sm leading-relaxed text-faint">
            Without an engine you can still explore the bundled replay demo. With no Fireworks key
            the engine falls back to the sim victim and deterministic analyst.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2 border-t border-line px-6 py-4">
          {isOnboarding ? (
            <>
              <button
                type="button"
                onClick={save}
                className="focus-ring inline-flex items-center gap-2 rounded-xl bg-fg px-4 py-2.5 text-sm font-semibold text-bg transition hover:opacity-90"
              >
                {savedFlash ? "Saved" : "Save & continue"}
              </button>
              <button
                type="button"
                onClick={skip}
                className="focus-ring inline-flex items-center gap-2 rounded-xl bg-surface-2 px-3.5 py-2.5 text-sm font-medium text-muted transition hover:bg-surface-3 hover:text-fg"
              >
                Skip — try the replay
              </button>
            </>
          ) : (
            <>
              <button
                type="button"
                onClick={save}
                className="focus-ring inline-flex items-center gap-2 rounded-xl bg-fg px-4 py-2.5 text-sm font-semibold text-bg transition hover:opacity-90"
              >
                {savedFlash ? "Saved" : "Save"}
              </button>
              <button
                type="button"
                onClick={onClose}
                className="focus-ring inline-flex items-center gap-2 rounded-xl bg-surface-2 px-3.5 py-2.5 text-sm font-medium text-muted transition hover:bg-surface-3 hover:text-fg"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={clear}
                className="focus-ring ml-auto inline-flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium text-muted transition hover:bg-danger/10 hover:text-danger"
              >
                Clear keys
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function Field({
  label,
  hint,
  htmlFor,
  docs,
  children,
}: {
  label: string;
  hint?: string;
  htmlFor: string;
  docs?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-baseline gap-x-2">
        <label htmlFor={htmlFor} className="text-sm font-medium text-fg">
          {label}
        </label>
        {hint && <span className="text-xs text-faint">{hint}</span>}
        {docs && (
          <a
            href={docs}
            target="_blank"
            rel="noreferrer"
            className="focus-ring ml-auto inline-flex items-center gap-1 rounded text-xs text-muted underline-offset-4 transition-colors hover:text-fg hover:underline"
          >
            Get a key
            <ExternalLink size={11} aria-hidden="true" />
          </a>
        )}
      </div>
      {children}
    </div>
  );
}

/** Compact status chip for the top bar: which BYOK pieces are present. */
export function CredentialsBadge({
  credentials,
  onClick,
}: {
  credentials: Credentials;
  onClick: () => void;
}) {
  const d = hasDaytona(credentials);
  const f = hasFireworks(credentials);
  const label =
    d && f ? "Keys set" : d ? "Daytona only" : f ? "Fireworks only" : "Add keys";

  return (
    <button
      type="button"
      onClick={onClick}
      title="Daytona + Fireworks API keys (stored in this browser)"
      className={cn(
        "focus-ring hidden items-center gap-1.5 rounded-xl px-2.5 py-1.5 text-xs font-medium transition-colors sm:inline-flex",
        d || f
          ? "bg-success/10 text-success hover:bg-success/15"
          : "bg-surface-2 text-muted hover:bg-surface-3 hover:text-fg",
      )}
    >
      <KeyRound size={13} strokeWidth={1.75} aria-hidden="true" />
      {label}
    </button>
  );
}
