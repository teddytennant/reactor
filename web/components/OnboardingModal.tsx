"use client";

import { useEffect, useId, useState } from "react";
import Link from "next/link";
import { Check, ExternalLink } from "lucide-react";
import { cn } from "@/lib/cn";
import {
  EMPTY_CREDENTIALS,
  loadCredentials,
  markOnboardingDone,
  saveCredentials,
  type Credentials,
} from "@/lib/credentials";
import { DEFAULT_LOCAL_ENGINE, engineOrigin, setEngineOrigin } from "@/lib/engine";

export interface OnboardingModalProps {
  open: boolean;
  onClose: () => void;
  /** Fired after save or skip so the console can pick the keys up. */
  onChange?: (c: Credentials) => void;
}

/**
 * First run only. Asks for the two keys a live detonation needs — Daytona (the
 * disposable chamber) and Fireworks (victim + analyst models) — and where the
 * engine is listening. Keys stay in localStorage and only leave the browser
 * toward the engine on detonate/upload, never to the Vercel host.
 *
 * Everything else, and every later edit, lives on /settings. This dialog is
 * deliberately the short version of that page.
 */
export function OnboardingModal({ open, onClose, onChange }: OnboardingModalProps) {
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

  if (!open) return null;

  const save = () => {
    saveCredentials(draft);
    // Blank means "use the default" rather than "use same-origin", which for a
    // deployed static export would be the 404 page.
    setEngineOrigin(engineDraft.trim() || DEFAULT_LOCAL_ENGINE);
    markOnboardingDone();
    onChange?.(draft);
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

  const set =
    (field: keyof Credentials) =>
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setDraft((d) => ({ ...d, [field]: e.target.value }));
    };

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-bg/70 p-4 backdrop-blur-sm sm:items-center"
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="panel relative flex w-full max-w-lg flex-col overflow-hidden shadow-panel"
      >
        <div className="px-6 pb-5 pt-6">
          <h2 id={titleId} className="text-lg font-semibold tracking-tight text-fg">
            Bring your own keys
          </h2>
          <p className="mt-1.5 text-sm leading-relaxed text-muted">
            Reactor runs detonations in your Daytona sandboxes and drives victim + analyst models
            on Fireworks. Keys stay in this browser.
          </p>
        </div>

        <div className="flex flex-col gap-4 px-6 pb-5">
          <Field
            label="Daytona API key"
            hint="Disposable sandbox per detonation"
            htmlFor="onb-daytona-key"
            docs="https://www.daytona.io/docs"
          >
            <input
              id="onb-daytona-key"
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
            label="Fireworks API key"
            hint="Victim agent + analyst model"
            htmlFor="onb-fireworks-key"
            docs="https://fireworks.ai/account/api-keys"
          >
            <input
              id="onb-fireworks-key"
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
              htmlFor="onb-engine-url"
            >
              <input
                id="onb-engine-url"
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
            the engine falls back to the sim victim and deterministic analyst. All of this is
            editable later in{" "}
            <Link
              href="/settings"
              className="focus-ring rounded text-muted underline underline-offset-4 hover:text-fg"
            >
              Settings
            </Link>
            .
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2 border-t border-line px-6 py-4">
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
