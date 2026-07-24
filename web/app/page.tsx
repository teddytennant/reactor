"use client";

import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import {
  AlertTriangle,
  ChevronRight,
  FlaskConical,
  Loader2,
  RotateCcw,
  ShieldOff,
  Zap,
} from "lucide-react";
import type { Artifact, ReactorEvent } from "@/lib/events";
import { consoleReducer, initialConsoleState } from "@/lib/reducer";
import { detonate, detonateWithError, getArtifacts, getHealth, type DetonateBody } from "@/lib/api";
import { startLive, startReplay, type DetonationRunner } from "@/lib/runner";
import {
  FIXTURE_ARTIFACTS,
  REPLAYABLE_IDS,
  fixtureFor,
} from "@/lib/fixtures";
import {
  EMPTY_CREDENTIALS,
  isOnboardingDone,
  loadCredentials,
  type Credentials,
} from "@/lib/credentials";
import { cn } from "@/lib/cn";
import { unreachableReason } from "@/lib/engine";
import { TopBar } from "@/components/TopBar";
import { CredentialsModal } from "@/components/CredentialsModal";
import { ArtifactPicker } from "@/components/console/ArtifactPicker";
import { ArtifactIntake, isIngested, type IntakeTarget } from "@/components/console/ArtifactIntake";
import { ScanColumn } from "@/components/console/ScanColumn";
import { ReactorColumn } from "@/components/console/ReactorColumn";

const PLANNED_SESSIONS = 5;
type Mode = "probing" | "live" | "replay";

/** How each intake source names itself to POST /api/detonate (CONTRACT.md). */
function bodyFor(t: IntakeTarget): DetonateBody {
  if (t.source === "upload") return { upload_id: t.uploadId, sessions: PLANNED_SESSIONS };
  if (t.source === "repo") return { repo: t.repo, ref: t.ref, sessions: PLANNED_SESSIONS };
  return { artifact: t.artifact, sessions: PLANNED_SESSIONS };
}

export default function ConsolePage() {
  const [state, dispatch] = useReducer(consoleReducer, undefined, initialConsoleState);
  const [mode, setMode] = useState<Mode>("probing");
  const [artifacts, setArtifacts] = useState<Artifact[]>(FIXTURE_ARTIFACTS);
  const [selected, setSelected] = useState<Artifact | null>(
    FIXTURE_ARTIFACTS.find((a) => a.id === "art_notes") ?? FIXTURE_ARTIFACTS[0] ?? null,
  );
  const [running, setRunning] = useState(false);
  const [showPicker, setShowPicker] = useState(true);
  // What the intake armed, if anything. When set it beats the zoo selection:
  // an upload or a clone is a deliberate act, a zoo row is a default.
  const [intake, setIntake] = useState<IntakeTarget | null>(null);
  const [arming, setArming] = useState(false);
  const [runError, setRunError] = useState<string | null>(null);
  // Ingested artifacts get no host-side static baseline (CONTRACT.md), so the
  // left column has nothing to report and must say so rather than hang.
  const [runIngested, setRunIngested] = useState(false);

  // BYOK: Daytona + Fireworks keys in localStorage. First visit opens onboarding.
  const [credentials, setCredentials] = useState<Credentials>(EMPTY_CREDENTIALS);
  const [credModal, setCredModal] = useState<{ open: boolean; mode: "onboarding" | "settings" }>({
    open: false,
    mode: "onboarding",
  });

  const runnerRef = useRef<DetonationRunner | null>(null);
  const gotVerdictRef = useRef(false);
  const receivedRef = useRef(false);
  const selectedRef = useRef<Artifact | null>(selected);
  selectedRef.current = selected;
  const intakeRef = useRef<IntakeTarget | null>(intake);
  intakeRef.current = intake;

  // Probe the engine once; default to replay mode if unreachable (DEMO §7).
  // Also hydrate BYOK credentials and open first-run onboarding when needed.
  useEffect(() => {
    let alive = true;
    const creds = loadCredentials();
    setCredentials(creds);
    if (!isOnboardingDone()) {
      setCredModal({ open: true, mode: "onboarding" });
    }
    (async () => {
      const [health, arts] = await Promise.all([getHealth(), getArtifacts()]);
      if (!alive) return;
      if (arts && arts.length) {
        setArtifacts(arts);
        setSelected((prev) => arts.find((a) => a.id === prev?.id) ?? arts.find((a) => a.id === "art_notes") ?? arts[0]);
      }
      setMode(health?.ok ? "live" : "replay");
    })();
    return () => {
      alive = false;
    };
  }, []);

  const stopRunner = useCallback(() => {
    runnerRef.current?.stop();
    runnerRef.current = null;
  }, []);

  useEffect(() => () => stopRunner(), [stopRunner]);

  const beginReplay = useCallback((artifact: Artifact | null) => {
    const events = fixtureFor(artifact);
    runnerRef.current = startReplay(events as ReactorEvent[], {
      onEvent: (ev) => dispatch({ type: "event", ev }),
      onDone: () => setRunning(false),
    });
  }, []);

  const launch = useCallback(
    (id: string, artifact: Artifact | null, fixtureFallback: boolean) => {
      dispatch({ type: "meta", detonationId: id });
      runnerRef.current = startLive(id, {
        onEvent: (ev) => {
          receivedRef.current = true;
          if (ev.kind === "verdict") gotVerdictRef.current = true;
          dispatch({ type: "event", ev });
        },
        onDone: () => setRunning(false),
        onError: () => {
          // Live stream failed before completing — fall back to the fixture.
          // Never for an ingested artifact: replaying the bundled money shot
          // over someone's own upload would be a lie.
          if (fixtureFallback && !gotVerdictRef.current && !receivedRef.current) {
            beginReplay(artifact);
            return;
          }
          setRunning(false);
          if (!receivedRef.current) {
            setRunError("the event stream closed before the engine sent anything");
          }
        },
      });
    },
    [beginReplay],
  );

  const run = useCallback(
    async (opts: { forceReplay?: boolean } = {}) => {
      const target = intakeRef.current;
      const artifact = target ? target.artifact : selectedRef.current;
      const wantsLive = mode === "live" && !opts.forceReplay;

      const begin = () => {
        stopRunner();
        dispatch({ type: "reset" });
        gotVerdictRef.current = false;
        receivedRef.current = false;
        setRunError(null);
        setRunning(true);
        setShowPicker(false);
        setRunIngested(isIngested(target));
        dispatch({ type: "meta", artifactName: artifact?.name });
      };

      // Uploads, clones and inline specs have no fixture standing behind them,
      // and the POST itself may be cloning for up to 90s. So the intake stays
      // on screen until the engine has accepted it, and a refusal lands as the
      // engine's own sentence rather than as a silent fixture replay.
      if (target) {
        if (!wantsLive) {
          setRunError("Upload and repository intake need a live engine.");
          return;
        }
        setArming(true);
        const res = await detonateWithError(bodyFor(target));
        setArming(false);
        if (!res.id) {
          setRunError(res.error ?? "the engine refused the detonation");
          return;
        }
        begin();
        launch(res.id, artifact, false);
        return;
      }

      begin();
      if (wantsLive) {
        const id = await detonate({ artifact_id: artifact?.id, sessions: PLANNED_SESSIONS });
        if (id) {
          launch(id, artifact, true);
          return;
        }
        // POST failed — degrade to replay so the demo never dies.
      }
      beginReplay(artifact);
    },
    [mode, stopRunner, beginReplay, launch],
  );

  const reset = useCallback(() => {
    stopRunner();
    dispatch({ type: "reset" });
    setRunning(false);
    setRunError(null);
    setShowPicker(true);
  }, [stopRunner]);

  const isReplayable = useCallback((a: Artifact) => REPLAYABLE_IDS.has(a.id), []);
  const armed = intake?.artifact ?? selected;
  const artifactName = state.artifactName ?? armed?.name ?? "artifact";
  // An ingested artifact never produces scan lines, so the column must not sit
  // there claiming to be waiting for a scanner that was deliberately not run.
  const noBaseline = runIngested && state.scanLines.length === 0;
  const scanActive = (running || state.scanLines.length > 0) && !noBaseline;
  // The scanner emitted nothing and never will, so the transcript says so in
  // the one place a reader looks — under the prompt — rather than leaving the
  // column blank and looking broken.
  const scanLines = noBaseline
    ? [
        {
          tool: "mcp-scan",
          stream: "stdout",
          text: "(not run — no static baseline is produced for an uploaded or cloned artifact)",
          issues: 0,
          done: false,
        },
      ]
    : state.scanLines;

  /**
   * The core bloom (DESIGN §1 Texture / §2) — the state-reactive radial ambient
   * behind the Reactor column, and the most identity-defining element on the
   * page. It reads the same two facts the header bar does: are we detonating,
   * and what did the verdict say.
   */
  const coreState: "idle" | "running" | "blocked" | "allowed" = running
    ? "running"
    : state.verdict?.label === "MALICIOUS"
      ? "blocked"
      : state.verdict?.label === "ALLOWED"
        ? "allowed"
        : "idle";

  return (
    <div className="flex min-h-dvh flex-col bg-bg lg:h-dvh lg:min-h-0 lg:overflow-hidden">
      <TopBar
        credentials={credentials}
        onOpenSettings={() => setCredModal({ open: true, mode: "settings" })}
      />
      <CredentialsModal
        open={credModal.open}
        mode={credModal.mode}
        onClose={() => setCredModal((m) => ({ ...m, open: false }))}
        onChange={setCredentials}
      />

      {/* The only thing that ever reaches past this box is `.core-bloom::before`,
          whose ambient layer is inset `-5%` horizontally by design. `overflow-x-clip`
          contains that decorative overhang without turning main into a scroll
          container; every real child is width-constrained and `min-w-0`. */}
      <main className="console-veil mx-auto flex w-full max-w-[1400px] flex-1 flex-col gap-4 overflow-x-clip px-4 pb-4 pt-4 sm:px-6 lg:min-h-0">
        {showPicker ? (
          <PickerPanel
            mode={mode}
            artifacts={artifacts}
            selected={selected}
            onSelect={(a) => {
              setIntake(null);
              setRunError(null);
              setSelected(a);
            }}
            intake={intake}
            onIntake={(t) => {
              setIntake(t);
              setRunError(null);
            }}
            isReplayable={isReplayable}
            onDetonate={() => run()}
            disabled={running}
            arming={arming}
            error={runError}
          />
        ) : (
          <RunHeaderBar
            mode={mode}
            artifact={armed}
            running={running}
            arming={arming}
            error={runError}
            verdictLabel={state.verdict?.label}
            onDetonate={() => run()}
            onReset={reset}
          />
        )}

        {/* At idle the columns size to their content — two 700px hollow boxes
            read as broken, not as an instrument at rest. Once a detonation is
            armed the console goes back to filling the viewport (DESIGN §3). */}
        <div
          className={cn(
            "grid min-h-0 grid-cols-1 gap-4 lg:grid-cols-2",
            showPicker ? "lg:auto-rows-min" : "lg:flex-1 lg:auto-rows-fr",
          )}
        >
          {/* The scan column, with the one thing it cannot say for itself: an
              ingested artifact gets no host-side baseline, so `unavailable` is
              a policy, not a failure (CONTRACT.md § Artifact ingest). */}
          <div
            className={cn(
              "grid min-h-0",
              noBaseline ? "grid-rows-[auto_minmax(0,1fr)] gap-2.5" : "grid-rows-[minmax(0,1fr)]",
            )}
          >
            {noBaseline && (
              <p className="flex items-start gap-2 px-1 text-sm leading-relaxed text-muted">
                <ShieldOff size={14} className="mt-1 shrink-0 text-faint" aria-hidden="true" />
                <span>
                  Static baseline <span className="font-mono">unavailable</span> for an uploaded or
                  cloned artifact — mcp-scan reads tool descriptions by installing and launching the
                  server on this host, and Reactor will not do that with a stranger&rsquo;s code.
                  The right column is the only column here.
                </span>
              </p>
            )}
            <ScanColumn
              artifactName={artifactName}
              scanLines={scanLines}
              result={state.scanResult}
              active={scanActive}
            />
          </div>
          {/* The bloom lives on the column wrapper so it reads around and
              between the Reactor panels, never inside a single one. */}
          <div
            className="core-bloom grid min-h-0 grid-rows-[minmax(0,1fr)]"
            data-core={coreState}
          >
            <ReactorColumn state={state} plannedSessions={PLANNED_SESSIONS} active={running} />
          </div>
        </div>
      </main>
    </div>
  );
}

// ---- mode readout ---------------------------------------------------------

/**
 * Engine-vs-fixture provenance. Real information, but secondary: it is plain
 * sentence-case sans in `--muted`, never a chip and never parked in a card's
 * top-right corner where it reads as a status a user has to decode.
 */
function ModeNote({ mode, className }: { mode: Mode; className?: string }) {
  if (mode === "probing") {
    return <span className={cn("text-sm text-faint", className)}>Probing engine…</span>;
  }
  if (mode === "live") {
    return <span className={cn("text-sm text-muted", className)}>Live engine</span>;
  }
  return (
    <span
      title="No engine reached — playing a bundled recording of a real detonation through the same render path."
      className={cn("text-sm text-muted", className)}
    >
      Bundled replay
    </span>
  );
}

/**
 * Shown when the probe found no engine. The cause is diagnosed after the fact
 * from what actually failed (lib/engine.ts), never predicted from a browser
 * allowlist — WebKit is the one engine we can name with certainty.
 */
function ReplayNotice({ className }: { className?: string }) {
  const [reason, setReason] = useState<"webkit" | "mixed-content" | "no-engine">("no-engine");
  useEffect(() => setReason(unreachableReason()), []);

  return (
    <div className={cn("rounded-xl border border-line bg-surface-2/60 px-4 py-3", className)}>
      <p className="text-sm font-medium text-fg">
        {reason === "webkit"
          ? "Safari can only show the replay"
          : "Running the bundled replay"}
      </p>
      <p className="mt-1.5 text-sm leading-relaxed text-muted">
        {reason === "webkit" ? (
          <>
            Safari blocks this page from reaching an engine on your own machine, so live
            detonation is not possible here. Everything below is a recording of a real run.{" "}
            <span className="text-fg">
              To detonate your own files with your own keys, open this in Chrome, Edge, or Brave.
            </span>
          </>
        ) : (
          <>
            No engine answered, so this is a recording of a real run. To detonate your own files
            with your own keys, clone the repo, run{" "}
            <code className="rounded bg-surface-3 px-1 py-0.5 font-mono text-xs">
              make build &amp;&amp; ./bin/reactor serve
            </code>
            , then reload. Chrome, Edge and Brave can reach a local engine from this page; Safari
            cannot, and other browsers vary.
          </>
        )}
      </p>
    </div>
  );
}

// ---- idle picker panel ----------------------------------------------------

function PickerPanel({
  mode,
  artifacts,
  selected,
  onSelect,
  intake,
  onIntake,
  isReplayable,
  onDetonate,
  disabled,
  arming,
  error,
}: {
  mode: Mode;
  artifacts: Artifact[];
  selected: Artifact | null;
  onSelect: (a: Artifact) => void;
  intake: IntakeTarget | null;
  onIntake: (t: IntakeTarget | null) => void;
  isReplayable: (a: Artifact) => boolean;
  onDetonate: () => void;
  disabled: boolean;
  arming: boolean;
  error: string | null;
}) {
  const [zooOpen, setZooOpen] = useState(false);
  const zooId = "artifact-zoo";

  // With no engine there is nothing to upload to, so the sample rack is the
  // only way through and opens itself.
  useEffect(() => {
    if (mode === "replay") setZooOpen(true);
  }, [mode]);

  const sampleArmed = !intake && selected;

  return (
    <section className="panel shrink-0 overflow-hidden">
      {/* intake strip — a label and nothing else; provenance lives in the
          footnote below, not as a chip floating in the corner */}
      <div className="flex items-center gap-3 border-b border-line px-5 py-3">
        <span className="strip-label whitespace-nowrap">Artifact intake</span>
        <span className="rule" aria-hidden="true" />
      </div>

      <div className="flex flex-col gap-4 px-5 py-4">
        <div className="max-w-2xl">
          <h1 className="text-xl font-semibold tracking-tight text-fg">
            Drop an untrusted agent artifact
          </h1>
          <p className="mt-2 text-base leading-relaxed text-muted">
            Static scanners read the label. Reactor points a sacrificial victim agent at it and
            watches it behave.
          </p>
        </div>

        {mode === "replay" && <ReplayNotice className="max-w-3xl" />}

        <ArtifactIntake
          mode={mode}
          target={intake}
          onTarget={onIntake}
          disabled={disabled || arming}
        />

        {/* The zoo is the sample rack now: folded away by default, and still a
            bounded, self-scrolling well when opened so it can never push the
            primary action or the console columns below the fold. */}
        <div className="flex max-w-3xl flex-col gap-2.5">
          <button
            type="button"
            onClick={() => setZooOpen((o) => !o)}
            aria-expanded={zooOpen}
            aria-controls={zooId}
            className="focus-ring group -mx-1.5 flex items-center gap-2 rounded-lg px-1.5 py-1 text-left"
          >
            <ChevronRight
              size={14}
              aria-hidden="true"
              className={cn(
                "shrink-0 text-faint transition-transform duration-200 ease-instrument",
                zooOpen && "rotate-90",
              )}
            />
            <span className="strip-label whitespace-nowrap group-hover:text-fg">
              or try a sample artifact
            </span>
            <span className="rule" aria-hidden="true" />
            <span className="inline-flex min-w-0 items-baseline gap-2 whitespace-nowrap text-sm text-muted">
              {sampleArmed && !zooOpen && (
                <>
                  <span className="min-w-0 max-w-[14rem] truncate font-mono text-2xs text-fg">
                    {selected.name}
                  </span>
                  <span className="text-faint" aria-hidden="true">
                    ·
                  </span>
                </>
              )}
              <span>
                <span className="tnum">{artifacts.length}</span> in zoo
              </span>
            </span>
          </button>
          <div id={zooId} hidden={!zooOpen}>
            <ArtifactPicker
              artifacts={artifacts}
              selectedId={intake ? null : (selected?.id ?? null)}
              onSelect={onSelect}
              isReplayable={isReplayable}
              showOfflineTag={mode === "replay"}
              disabled={disabled}
            />
          </div>
        </div>
      </div>

      {/* arming strip — the one primary action, carried by maximum ink contrast
          rather than hue: `bg-fg text-bg` is near-white on near-black in dark and
          inverts to near-black on white in light. No chroma anywhere on the page. */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-3 border-t border-line bg-surface-2/40 px-5 py-4">
        <button
          type="button"
          onClick={onDetonate}
          disabled={disabled || arming || (!selected && !intake)}
          className="focus-ring inline-flex items-center gap-2 rounded-xl bg-fg px-4 py-2.5 text-sm font-semibold text-bg transition duration-200 ease-instrument hover:opacity-90 active:scale-[0.99] disabled:pointer-events-none disabled:opacity-40"
        >
          {arming ? (
            <>
              <Loader2 size={15} className="animate-spin-slow" aria-hidden="true" />
              {intake?.source === "repo" ? "Cloning…" : "Arming…"}
            </>
          ) : (
            <>
              <Zap size={15} className="fill-current" aria-hidden="true" />
              Detonate
            </>
          )}
        </button>
        <span className="ml-auto inline-flex min-w-0 flex-wrap items-center justify-end gap-x-2 gap-y-1 text-sm text-muted">
          <FlaskConical size={14} className="shrink-0 text-faint" aria-hidden="true" />
          <span>
            <span className="tnum">{PLANNED_SESSIONS}</span> detonations · fresh sandbox each ·
            victim holds only bait
          </span>
          <span className="text-faint" aria-hidden="true">
            ·
          </span>
          <ModeNote mode={mode} />
        </span>
        <RunError error={error} />
      </div>
    </section>
  );
}

/**
 * A refused detonation, in the engine's own words. The engine writes these for
 * a person and they carry no host paths (CONTRACT.md), so they are rendered
 * verbatim rather than translated into something vaguer.
 */
function RunError({ error }: { error: string | null }) {
  if (!error) return null;
  return (
    <p
      role="alert"
      className="flex w-full items-start gap-2 text-sm leading-relaxed text-danger"
    >
      <AlertTriangle size={14} className="mt-1 shrink-0" aria-hidden="true" />
      <span className="min-w-0">{error}</span>
    </p>
  );
}

// ---- running / done header bar --------------------------------------------

function RunHeaderBar({
  mode,
  artifact,
  running,
  arming,
  error,
  verdictLabel,
  onDetonate,
  onReset,
}: {
  mode: Mode;
  artifact: Artifact | null;
  running: boolean;
  arming: boolean;
  error: string | null;
  verdictLabel?: string;
  onDetonate: () => void;
  onReset: () => void;
}) {
  const status: { text: string; cls: string } = arming
    ? { text: "arming…", cls: "text-live" }
    : running
      ? { text: "detonating…", cls: "text-live" }
      : verdictLabel === "MALICIOUS"
        ? { text: "blocked", cls: "text-danger" }
        : verdictLabel === "ALLOWED"
          ? { text: "allowed", cls: "text-success" }
          : verdictLabel
            ? { text: "complete", cls: "text-muted" }
            : { text: "ready", cls: "text-muted" };

  return (
    <div className="panel flex shrink-0 flex-wrap items-center gap-x-4 gap-y-3 px-4 py-3 sm:px-5">
      {/* subject — the package name is machine data, so it stays mono. No lamp:
          the status word beside it already says what the run is doing. */}
      <span className="min-w-0 truncate font-mono text-sm font-medium text-fg">
        {artifact?.name}
      </span>

      <span className="hidden h-5 w-px bg-line sm:block" aria-hidden="true" />

      {/* status readout — sentence-case sans, paired with its label so state is
          never carried by colour alone */}
      <div className="flex items-baseline gap-2">
        <span className="strip-label">Status</span>
        <span className={cn("text-sm font-medium capitalize", status.cls)}>{status.text}</span>
      </div>

      <div className="ml-auto flex flex-wrap items-center gap-2">
        <ModeNote mode={mode} />
        {!running && (
          <button
            type="button"
            onClick={onDetonate}
            disabled={arming}
            className="focus-ring inline-flex items-center gap-2 rounded-xl bg-surface-2 px-3.5 py-2 text-sm font-medium text-fg transition-colors duration-200 ease-instrument hover:bg-surface-3 disabled:pointer-events-none disabled:opacity-40"
          >
            {arming ? (
              <Loader2 size={14} className="animate-spin-slow text-faint" aria-hidden="true" />
            ) : (
              <Zap size={14} className="text-faint" aria-hidden="true" />
            )}
            Re-detonate
          </button>
        )}
        <button
          type="button"
          onClick={onReset}
          className="focus-ring inline-flex items-center gap-2 rounded-xl bg-surface-2 px-3.5 py-2 text-sm font-medium text-muted transition-colors duration-200 ease-instrument hover:bg-surface-3 hover:text-fg"
        >
          <RotateCcw size={14} aria-hidden="true" />
          New
        </button>
      </div>
      <RunError error={error} />
    </div>
  );
}
