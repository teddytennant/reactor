"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import {
  AlertTriangle,
  Check,
  CheckCircle2,
  ExternalLink,
  Loader2,
  Monitor,
  Moon,
  RotateCcw,
  Sun,
  XCircle,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { TopBar } from "@/components/TopBar";
import { getHealth } from "@/lib/api";
import type { Health } from "@/lib/events";
import {
  CREDENTIALS_STORAGE_KEY,
  EMPTY_CREDENTIALS,
  ONBOARDING_DONE_KEY,
  clearCredentials,
  loadCredentials,
  maskKey,
  saveCredentials,
  type Credentials,
} from "@/lib/credentials";
import {
  DEFAULT_LOCAL_ENGINE,
  ENGINE_STORAGE_KEY,
  clearEngineOrigin,
  defaultEngineOrigin,
  engineOrigin,
  isLoopbackEngine,
  normalizeOrigin,
  setEngineOrigin,
} from "@/lib/engine";
import {
  DEFAULT_PREFS,
  MAX_SESSIONS,
  MIN_SESSIONS,
  PREFS_STORAGE_KEY,
  THEME_STORAGE_KEY,
  applyTheme,
  clampSessions,
  clearPrefs,
  loadPrefs,
  loadTheme,
  savePrefs,
  saveTheme,
  type Prefs,
  type ThemeChoice,
} from "@/lib/prefs";

/**
 * Everything the console lets you change, in one place.
 *
 * Reactor has no server of its own — the frontend is a static export and the
 * engine is the visitor's own process — so every setting here is localStorage
 * on this device and nothing on this page is ever sent to the Vercel host.
 *
 * Two save models, deliberately: the theme applies the moment you pick it
 * (you are looking at the result), everything else is a draft until Save, so a
 * half-typed API key or engine URL never lands mid-keystroke.
 */
export default function SettingsPage() {
  const [creds, setCreds] = useState<Credentials>(EMPTY_CREDENTIALS);
  const [engineDraft, setEngineDraft] = useState("");
  const [prefs, setPrefs] = useState<Prefs>(DEFAULT_PREFS);
  const [theme, setTheme] = useState<ThemeChoice>("dark");
  const [showKeys, setShowKeys] = useState(false);
  const [saved, setSaved] = useState(false);

  // What is currently on disk — the thing "dirty" and "Revert" compare against.
  const [baseline, setBaseline] = useState<{
    creds: Credentials;
    engine: string;
    prefs: Prefs;
  } | null>(null);

  const hydrate = useCallback(() => {
    const c = loadCredentials();
    const e = engineOrigin();
    const p = loadPrefs();
    setCreds(c);
    setEngineDraft(e);
    setPrefs(p);
    setBaseline({ creds: c, engine: e, prefs: p });
  }, []);

  useEffect(() => {
    hydrate();
    setTheme(loadTheme());
  }, [hydrate]);

  const dirty = useMemo(() => {
    if (!baseline) return false;
    return (
      baseline.creds.daytonaApiKey !== creds.daytonaApiKey.trim() ||
      baseline.creds.daytonaApiUrl !== creds.daytonaApiUrl.trim() ||
      baseline.creds.fireworksApiKey !== creds.fireworksApiKey.trim() ||
      baseline.engine !== normalizeOrigin(engineDraft) ||
      baseline.prefs.sessions !== clampSessions(prefs.sessions) ||
      baseline.prefs.network !== prefs.network ||
      baseline.prefs.autoOpenZoo !== prefs.autoOpenZoo
    );
  }, [baseline, creds, engineDraft, prefs]);

  const save = () => {
    saveCredentials(creds);
    // Blank means "use the default", not "same-origin" — for a static export
    // same-origin /api/* is the 404 page.
    const nextEngine = engineDraft.trim();
    if (nextEngine) setEngineOrigin(nextEngine);
    else clearEngineOrigin();
    savePrefs(prefs);
    hydrate();
    setSaved(true);
    window.setTimeout(() => setSaved(false), 1600);
  };

  const revert = () => {
    if (!baseline) return;
    setCreds(baseline.creds);
    setEngineDraft(baseline.engine);
    setPrefs(baseline.prefs);
  };

  const pickTheme = (choice: ThemeChoice) => {
    setTheme(choice);
    saveTheme(choice);
    applyTheme(choice);
  };

  const setCred =
    (field: keyof Credentials) =>
    (e: React.ChangeEvent<HTMLInputElement>) =>
      setCreds((c) => ({ ...c, [field]: e.target.value }));

  return (
    <div className="flex min-h-dvh flex-col bg-bg">
      <TopBar />

      <main className="console-veil mx-auto w-full max-w-3xl flex-1 px-4 pb-32 pt-6 sm:px-6">
        <header className="mb-7 max-w-2xl">
          <h1 className="text-xl font-semibold tracking-tight text-fg">Settings</h1>
          <p className="mt-2 text-base leading-relaxed text-muted">
            Everything below is stored in this browser only. Reactor has no accounts and no
            backend of its own — the console is a static page and the engine is your own process.
          </p>
        </header>

        <div className="flex flex-col gap-4">
          <Section
            title="Appearance"
            desc="Applies immediately, and is remembered on this device."
          >
            <Row label="Theme" hint="System follows your OS; Reactor opens dark by default.">
              <Segmented
                value={theme}
                onChange={pickTheme}
                options={[
                  { value: "system", label: "System", icon: Monitor },
                  { value: "light", label: "Light", icon: Sun },
                  { value: "dark", label: "Dark", icon: Moon },
                ]}
              />
            </Row>
          </Section>

          <EngineSection
            value={engineDraft}
            onChange={setEngineDraft}
            onUseDefault={() => setEngineDraft(defaultEngineOrigin())}
          />

          <Section
            title="API keys"
            desc="Sent to your engine on detonate and upload — never to the host that serves this page."
          >
            <Field
              label="Daytona API key"
              hint="One disposable sandbox per detonation"
              htmlFor="set-daytona-key"
              docs="https://www.daytona.io/docs"
              stored={baseline?.creds.daytonaApiKey}
            >
              <input
                id="set-daytona-key"
                type={showKeys ? "text" : "password"}
                autoComplete="off"
                spellCheck={false}
                placeholder="dtn_…"
                value={creds.daytonaApiKey}
                onChange={setCred("daytonaApiKey")}
                className="field-input font-mono text-sm"
              />
            </Field>

            <Field
              label="Daytona API URL"
              hint="Leave blank for the default cloud API"
              htmlFor="set-daytona-url"
            >
              <input
                id="set-daytona-url"
                type="url"
                autoComplete="off"
                spellCheck={false}
                placeholder="https://app.daytona.io/api"
                value={creds.daytonaApiUrl}
                onChange={setCred("daytonaApiUrl")}
                className="field-input font-mono text-sm"
              />
            </Field>

            <Field
              label="Fireworks API key"
              hint="Victim agent + analyst model"
              htmlFor="set-fireworks-key"
              docs="https://fireworks.ai/account/api-keys"
              stored={baseline?.creds.fireworksApiKey}
            >
              <input
                id="set-fireworks-key"
                type={showKeys ? "text" : "password"}
                autoComplete="off"
                spellCheck={false}
                placeholder="fw_…"
                value={creds.fireworksApiKey}
                onChange={setCred("fireworksApiKey")}
                className="field-input font-mono text-sm"
              />
            </Field>

            <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
              <Checkbox checked={showKeys} onChange={setShowKeys}>
                Show keys
              </Checkbox>
              <button
                type="button"
                onClick={() => {
                  clearCredentials();
                  setCreds({ ...EMPTY_CREDENTIALS });
                  setBaseline((b) => (b ? { ...b, creds: { ...EMPTY_CREDENTIALS } } : b));
                }}
                className="focus-ring inline-flex items-center gap-2 rounded-xl px-3 py-1.5 text-sm font-medium text-muted transition hover:bg-danger/10 hover:text-danger"
              >
                Clear keys
              </button>
            </div>

            <p className="text-sm leading-relaxed text-faint">
              With no Fireworks key the engine falls back to its sim victim and deterministic
              analyst; with no Daytona key it uses whatever chamber driver it can reach locally.
            </p>
          </Section>

          <Section
            title="Detonation defaults"
            desc="What the console asks the engine for when you hit Detonate."
          >
            <Row
              label="Sessions per detonation"
              hint="Each one gets a fresh chamber. The demo run is five."
            >
              <Stepper
                value={prefs.sessions}
                min={MIN_SESSIONS}
                max={MAX_SESSIONS}
                onChange={(n) => setPrefs((p) => ({ ...p, sessions: n }))}
              />
            </Row>

            <Row
              label="Allow network egress"
              hint="Off means the chamber is cut off and exfil attempts die at the sink."
            >
              <Switch
                checked={prefs.network}
                onChange={(v) => setPrefs((p) => ({ ...p, network: v }))}
                label="Allow the chamber outbound network"
              />
            </Row>

            <Row
              label="Open the sample rack"
              hint="Expand the artifact zoo on load instead of starting folded."
            >
              <Switch
                checked={prefs.autoOpenZoo}
                onChange={(v) => setPrefs((p) => ({ ...p, autoOpenZoo: v }))}
                label="Open the sample artifact rack by default"
              />
            </Row>

            {prefs.network && (
              <p className="flex items-start gap-2 text-sm leading-relaxed text-warning">
                <AlertTriangle size={14} className="mt-1 shrink-0" aria-hidden="true" />
                <span>
                  A networked chamber can reach the real internet with a stranger&rsquo;s code
                  inside it. Egress evidence stays honest either way — this only decides whether
                  the attempt is allowed to land.
                </span>
              </p>
            )}
          </Section>

          <LocalDataSection onCleared={hydrate} />

          <Section title="About" desc="Where the rest of Reactor lives.">
            <div className="flex flex-col gap-2 text-sm">
              <LinkRow href="https://github.com/teddytennant/reactor" external>
                Source, engine and docs on GitHub
              </LinkRow>
              <LinkRow href="/scorecard">
                Offline scorecard — 25 real servers, static-blind column
              </LinkRow>
            </div>
          </Section>
        </div>
      </main>

      {/* Save bar. Only present when something is actually pending, so the page
          never sits there implying unsaved work that does not exist. */}
      {(dirty || saved) && (
        <div className="sticky bottom-0 z-20 border-t border-line bg-bg/90 backdrop-blur-md">
          <div className="mx-auto flex max-w-3xl flex-wrap items-center gap-3 px-4 py-3 sm:px-6">
            <span className="text-sm text-muted">
              {dirty ? "Unsaved changes" : "Saved to this browser"}
            </span>
            <div className="ml-auto flex items-center gap-2">
              {dirty && (
                <button
                  type="button"
                  onClick={revert}
                  className="focus-ring inline-flex items-center gap-2 rounded-xl bg-surface-2 px-3.5 py-2 text-sm font-medium text-muted transition hover:bg-surface-3 hover:text-fg"
                >
                  <RotateCcw size={14} aria-hidden="true" />
                  Revert
                </button>
              )}
              <button
                type="button"
                onClick={save}
                disabled={!dirty}
                className="focus-ring inline-flex items-center gap-2 rounded-xl bg-fg px-4 py-2 text-sm font-semibold text-bg transition hover:opacity-90 disabled:pointer-events-none disabled:opacity-40"
              >
                {saved && !dirty ? (
                  <>
                    <Check size={14} aria-hidden="true" />
                    Saved
                  </>
                ) : (
                  "Save"
                )}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ---- engine ---------------------------------------------------------------

/**
 * The engine origin, plus the only honest way to know whether it is right:
 * actually probe it. The probe reports what answered — drivers and analyst —
 * rather than a green dot, because "reachable" and "able to detonate" are two
 * different facts.
 */
function EngineSection({
  value,
  onChange,
  onUseDefault,
}: {
  value: string;
  onChange: (v: string) => void;
  onUseDefault: () => void;
}) {
  const [probing, setProbing] = useState(false);
  const [health, setHealth] = useState<Health | null | undefined>(undefined);
  // The probe reads the *stored* origin via lib/api, so a typed-but-unsaved URL
  // would be tested against the old one. Say so instead of lying about it.
  const [probedOrigin, setProbedOrigin] = useState("");
  // Both notes below read localStorage / window.location, which the prerendered
  // HTML cannot know. Gate them on mount so hydration has nothing to disagree
  // with (this page is part of a static export).
  const [mounted, setMounted] = useState(false);
  const alive = useRef(true);
  useEffect(() => {
    setMounted(true);
    return () => void (alive.current = false);
  }, []);

  const test = async () => {
    setProbing(true);
    setProbedOrigin(engineOrigin() || "same-origin /api");
    const h = await getHealth();
    if (!alive.current) return;
    setHealth(h);
    setProbing(false);
  };

  const stale = mounted && normalizeOrigin(value) !== engineOrigin();
  const mixed =
    mounted &&
    window.location.protocol === "https:" &&
    isLoopbackEngine(normalizeOrigin(value) || defaultEngineOrigin());

  return (
    <Section
      title="Engine"
      desc="Reactor's control plane runs on your machine, not ours — it spawns sandboxes and holds multi-minute event streams."
    >
      <Field label="Engine URL" hint="Blank restores the default" htmlFor="set-engine-url">
        <input
          id="set-engine-url"
          type="url"
          autoComplete="off"
          spellCheck={false}
          placeholder={DEFAULT_LOCAL_ENGINE}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="field-input font-mono text-sm"
        />
      </Field>

      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={test}
          disabled={probing}
          className="focus-ring inline-flex items-center gap-2 rounded-xl bg-surface-2 px-3.5 py-2 text-sm font-medium text-fg transition hover:bg-surface-3 disabled:pointer-events-none disabled:opacity-40"
        >
          {probing ? (
            <Loader2 size={14} className="animate-spin-slow" aria-hidden="true" />
          ) : (
            <CheckCircle2 size={14} className="text-faint" aria-hidden="true" />
          )}
          Test connection
        </button>
        <button
          type="button"
          onClick={onUseDefault}
          className="focus-ring inline-flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium text-muted transition hover:bg-surface-2 hover:text-fg"
        >
          Use default
        </button>
        {stale && (
          <span className="text-sm text-faint">Save first — the probe uses the stored URL.</span>
        )}
      </div>

      {health !== undefined && !probing && (
        <div
          role="status"
          className={cn(
            "rounded-xl px-4 py-3 text-sm leading-relaxed",
            health ? "bg-success/10 text-success" : "bg-danger/10 text-danger",
          )}
        >
          {health ? (
            <>
              <span className="inline-flex items-center gap-2 font-medium">
                <CheckCircle2 size={14} aria-hidden="true" />
                Engine answered at <span className="font-mono">{probedOrigin}</span>
              </span>
              <p className="mt-1.5 text-muted">
                Analyst <span className="font-mono text-fg">{health.analyst}</span> · chambers{" "}
                {health.drivers.length ? (
                  health.drivers.map((d, i) => (
                    <span key={d.name}>
                      {i > 0 && " · "}
                      <span className={d.available ? "text-fg" : "text-faint line-through"}>
                        {d.name}
                      </span>
                    </span>
                  ))
                ) : (
                  <span className="text-faint">none reported</span>
                )}
              </p>
            </>
          ) : (
            <>
              <span className="inline-flex items-center gap-2 font-medium">
                <XCircle size={14} aria-hidden="true" />
                Nothing answered at <span className="font-mono">{probedOrigin}</span>
              </span>
              <p className="mt-1.5 text-muted">
                The console will run the bundled replay instead of a live detonation.
              </p>
            </>
          )}
        </div>
      )}

      {mixed && (
        <p className="flex items-start gap-2 text-sm leading-relaxed text-muted">
          <AlertTriangle size={14} className="mt-1 shrink-0 text-faint" aria-hidden="true" />
          <span>
            This page is https and the engine is plain-http loopback. Chrome, Edge and Brave allow
            that; Safari and every iOS browser block it outright and can only show the replay.
          </span>
        </p>
      )}

      <div>
        <p className="text-sm leading-relaxed text-faint">Start it with:</p>
        <pre className="mt-2 overflow-x-auto rounded-lg bg-surface-2 px-3 py-2 font-mono text-xs text-muted">
          make build &amp;&amp; ./bin/reactor serve
        </pre>
      </div>
    </Section>
  );
}

// ---- local data -----------------------------------------------------------

/** What is on this device, named exactly, and the one button that removes it. */
function LocalDataSection({ onCleared }: { onCleared: () => void }) {
  const [confirming, setConfirming] = useState(false);

  const clearAll = () => {
    clearCredentials();
    clearPrefs();
    clearEngineOrigin();
    try {
      localStorage.removeItem(ONBOARDING_DONE_KEY);
      localStorage.removeItem(THEME_STORAGE_KEY);
    } catch {
      /* private mode */
    }
    applyTheme("dark");
    setConfirming(false);
    onCleared();
  };

  const replayOnboarding = () => {
    try {
      localStorage.removeItem(ONBOARDING_DONE_KEY);
    } catch {
      /* private mode */
    }
  };

  return (
    <Section
      title="Local data"
      desc="Reactor stores five keys in this browser and nothing anywhere else."
    >
      <ul className="flex flex-col gap-1.5 text-sm text-muted">
        {[
          [CREDENTIALS_STORAGE_KEY, "Daytona + Fireworks keys"],
          [ENGINE_STORAGE_KEY, "engine origin override"],
          [PREFS_STORAGE_KEY, "detonation defaults"],
          [THEME_STORAGE_KEY, "theme choice"],
          [ONBOARDING_DONE_KEY, "first-run flag"],
        ].map(([key, what]) => (
          <li key={key} className="flex flex-wrap items-baseline gap-x-2">
            <code className="code-chip">{key}</code>
            <span className="text-faint">{what}</span>
          </li>
        ))}
      </ul>

      <div className="flex flex-wrap items-center gap-2 pt-1">
        <button
          type="button"
          onClick={replayOnboarding}
          className="focus-ring inline-flex items-center gap-2 rounded-xl bg-surface-2 px-3.5 py-2 text-sm font-medium text-muted transition hover:bg-surface-3 hover:text-fg"
        >
          Show first-run setup again
        </button>
        {confirming ? (
          <>
            <button
              type="button"
              onClick={clearAll}
              className="focus-ring inline-flex items-center gap-2 rounded-xl bg-danger px-3.5 py-2 text-sm font-semibold text-danger-fg transition hover:opacity-90"
            >
              Erase everything
            </button>
            <button
              type="button"
              onClick={() => setConfirming(false)}
              className="focus-ring inline-flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium text-muted transition hover:bg-surface-2 hover:text-fg"
            >
              Cancel
            </button>
          </>
        ) : (
          <button
            type="button"
            onClick={() => setConfirming(true)}
            className="focus-ring inline-flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium text-muted transition hover:bg-danger/10 hover:text-danger"
          >
            Clear all local data
          </button>
        )}
      </div>
    </Section>
  );
}

// ---- primitives -----------------------------------------------------------

function Section({
  title,
  desc,
  children,
}: {
  title: string;
  desc?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="panel overflow-hidden">
      <div className="flex items-center gap-3 border-b border-line px-5 py-3">
        <span className="strip-label whitespace-nowrap">{title}</span>
        <span className="rule" aria-hidden="true" />
      </div>
      <div className="flex flex-col gap-4 px-5 py-4">
        {desc && <p className="max-w-2xl text-sm leading-relaxed text-muted">{desc}</p>}
        {children}
      </div>
    </section>
  );
}

/** A labelled setting whose control sits on the right at wide widths. */
function Row({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2">
      <div className="min-w-0 max-w-md">
        <p className="text-sm font-medium text-fg">{label}</p>
        {hint && <p className="mt-0.5 text-sm leading-relaxed text-faint">{hint}</p>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

function Field({
  label,
  hint,
  htmlFor,
  docs,
  stored,
  children,
}: {
  label: string;
  hint?: string;
  htmlFor: string;
  docs?: string;
  /** Masked preview of what is already saved, when there is one. */
  stored?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-baseline gap-x-2">
        <label htmlFor={htmlFor} className="text-sm font-medium text-fg">
          {label}
        </label>
        {hint && <span className="text-xs text-faint">{hint}</span>}
        {stored ? (
          <span className="font-mono text-2xs text-success" title="Currently saved">
            {maskKey(stored)}
          </span>
        ) : null}
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

function Segmented<T extends string>({
  value,
  onChange,
  options,
}: {
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: string; icon: React.ComponentType<{ size?: number }> }[];
}) {
  return (
    <div role="radiogroup" className="flex items-center gap-0.5 rounded-xl bg-surface-2 p-1">
      {options.map((o) => {
        const active = o.value === value;
        const Icon = o.icon;
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(o.value)}
            className={cn(
              "focus-ring inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm transition-colors duration-200",
              active
                ? "bg-surface text-fg shadow-panel"
                : "text-muted hover:bg-surface/60 hover:text-fg",
            )}
          >
            <Icon size={14} aria-hidden="true" />
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

function Switch({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className={cn(
        "focus-ring relative h-6 w-11 shrink-0 rounded-full transition-colors duration-200",
        checked ? "bg-fg" : "bg-surface-3",
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          "absolute top-1 h-4 w-4 rounded-full transition-all duration-200 ease-instrument",
          checked ? "left-6 bg-bg" : "left-1 bg-faint",
        )}
      />
    </button>
  );
}

function Stepper({
  value,
  min,
  max,
  onChange,
}: {
  value: number;
  min: number;
  max: number;
  onChange: (n: number) => void;
}) {
  const step = (delta: number) => onChange(clampSessions(value + delta));
  return (
    <div className="flex items-center gap-1 rounded-xl bg-surface-2 p-1">
      <button
        type="button"
        onClick={() => step(-1)}
        disabled={value <= min}
        aria-label="One fewer session"
        className="focus-ring grid h-7 w-7 place-items-center rounded-lg text-muted transition-colors hover:bg-surface-3 hover:text-fg disabled:pointer-events-none disabled:opacity-30"
      >
        −
      </button>
      <input
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        value={value}
        onChange={(e) => onChange(clampSessions(Number(e.target.value)))}
        aria-label="Sessions per detonation"
        className="tnum w-10 bg-transparent text-center text-sm font-medium text-fg outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none"
      />
      <button
        type="button"
        onClick={() => step(1)}
        disabled={value >= max}
        aria-label="One more session"
        className="focus-ring grid h-7 w-7 place-items-center rounded-lg text-muted transition-colors hover:bg-surface-3 hover:text-fg disabled:pointer-events-none disabled:opacity-30"
      >
        +
      </button>
    </div>
  );
}

function Checkbox({
  checked,
  onChange,
  children,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  children: React.ReactNode;
}) {
  return (
    <label className="group flex w-fit cursor-pointer select-none items-center gap-2.5 text-sm text-muted transition-colors hover:text-fg">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="peer sr-only"
      />
      <span
        aria-hidden="true"
        className={cn(
          "grid h-[18px] w-[18px] shrink-0 place-items-center rounded-md border transition-colors",
          "peer-focus-visible:ring-2 peer-focus-visible:ring-fg/55 peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-surface",
          checked
            ? "border-fg bg-fg text-bg"
            : "border-line-strong bg-surface-2 text-transparent group-hover:border-faint",
        )}
      >
        <Check size={12} strokeWidth={3} />
      </span>
      {children}
    </label>
  );
}

function LinkRow({
  href,
  external,
  children,
}: {
  href: string;
  external?: boolean;
  children: React.ReactNode;
}) {
  const cls =
    "focus-ring inline-flex w-fit items-center gap-1.5 rounded text-muted underline-offset-4 transition-colors hover:text-fg hover:underline";
  if (external) {
    return (
      <a href={href} target="_blank" rel="noreferrer" className={cls}>
        {children}
        <ExternalLink size={12} aria-hidden="true" />
      </a>
    );
  }
  return (
    <Link href={href} className={cls}>
      {children}
    </Link>
  );
}
