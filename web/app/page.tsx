"use client";

import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { Play, RotateCcw, Sparkles, Zap } from "lucide-react";
import type { Artifact, ReactorEvent } from "@/lib/events";
import { consoleReducer, initialConsoleState } from "@/lib/reducer";
import { detonate, getArtifacts, getHealth } from "@/lib/api";
import { startLive, startReplay, type DetonationRunner } from "@/lib/runner";
import {
  FIXTURE_ARTIFACTS,
  REPLAYABLE_IDS,
  fixtureFor,
} from "@/lib/fixtures";
import { cn } from "@/lib/cn";
import { TopBar } from "@/components/TopBar";
import { ArtifactPicker } from "@/components/console/ArtifactPicker";
import { ScanColumn } from "@/components/console/ScanColumn";
import { ReactorColumn } from "@/components/console/ReactorColumn";

const PLANNED_SESSIONS = 5;
type Mode = "probing" | "live" | "replay";

export default function ConsolePage() {
  const [state, dispatch] = useReducer(consoleReducer, undefined, initialConsoleState);
  const [mode, setMode] = useState<Mode>("probing");
  const [artifacts, setArtifacts] = useState<Artifact[]>(FIXTURE_ARTIFACTS);
  const [selected, setSelected] = useState<Artifact | null>(
    FIXTURE_ARTIFACTS.find((a) => a.id === "art_notes") ?? FIXTURE_ARTIFACTS[0] ?? null,
  );
  const [running, setRunning] = useState(false);
  const [showPicker, setShowPicker] = useState(true);

  const runnerRef = useRef<DetonationRunner | null>(null);
  const gotVerdictRef = useRef(false);
  const receivedRef = useRef(false);
  const selectedRef = useRef<Artifact | null>(selected);
  selectedRef.current = selected;

  // Probe the engine once; default to replay mode if unreachable (DEMO §7).
  useEffect(() => {
    let alive = true;
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

  const run = useCallback(
    async (opts: { forceReplay?: boolean } = {}) => {
      stopRunner();
      dispatch({ type: "reset" });
      gotVerdictRef.current = false;
      receivedRef.current = false;
      setRunning(true);
      setShowPicker(false);
      const artifact = selectedRef.current;
      dispatch({ type: "meta", artifactName: artifact?.name });

      const wantsLive = mode === "live" && !opts.forceReplay;
      if (wantsLive) {
        const id = await detonate({ artifact_id: artifact?.id, sessions: PLANNED_SESSIONS });
        if (id) {
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
              if (!gotVerdictRef.current && !receivedRef.current) {
                beginReplay(artifact);
              } else {
                setRunning(false);
              }
            },
          });
          return;
        }
        // POST failed — degrade to replay so the demo never dies.
      }
      beginReplay(artifact);
    },
    [mode, stopRunner, beginReplay],
  );

  const reset = useCallback(() => {
    stopRunner();
    dispatch({ type: "reset" });
    setRunning(false);
    setShowPicker(true);
  }, [stopRunner]);

  const isReplayable = useCallback((a: Artifact) => REPLAYABLE_IDS.has(a.id), []);
  const artifactName = state.artifactName ?? selected?.name ?? "artifact";
  const scanActive = running || state.scanLines.length > 0;

  return (
    <div className="flex min-h-screen flex-col bg-bg">
      <TopBar />

      <main className="console-veil mx-auto flex w-full max-w-[1400px] flex-1 flex-col gap-4 px-4 py-4 sm:px-6 lg:min-h-0">
        {showPicker ? (
          <PickerPanel
            mode={mode}
            artifacts={artifacts}
            selected={selected}
            onSelect={setSelected}
            isReplayable={isReplayable}
            onDetonate={() => run()}
            onReplay={() => run({ forceReplay: true })}
            disabled={running}
          />
        ) : (
          <RunHeaderBar
            mode={mode}
            artifact={selected}
            running={running}
            verdictLabel={state.verdict?.label}
            onReplay={() => run({ forceReplay: true })}
            onDetonate={() => run()}
            onReset={reset}
          />
        )}

        <div className="grid min-h-0 grid-cols-1 gap-4 lg:flex-1 lg:auto-rows-fr lg:grid-cols-2">
          <ScanColumn
            artifactName={artifactName}
            scanLines={state.scanLines}
            result={state.scanResult}
            active={scanActive}
          />
          <ReactorColumn state={state} plannedSessions={PLANNED_SESSIONS} active={running} />
        </div>
      </main>
    </div>
  );
}

// ---- mode chip ------------------------------------------------------------

function ModeChip({ mode }: { mode: Mode }) {
  if (mode === "probing") {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-md border border-line px-2 py-1 text-2xs font-medium text-faint">
        <span className="h-1.5 w-1.5 animate-pulse-dot rounded-full bg-faint" />
        probing engine…
      </span>
    );
  }
  if (mode === "live") {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-md border border-success/35 bg-success/10 px-2 py-1 text-2xs font-semibold uppercase tracking-wide text-success">
        <span className="h-1.5 w-1.5 rounded-full bg-success" />
        Live engine
      </span>
    );
  }
  return (
    <span
      title="Engine unreachable — playing the bundled money-shot fixture through the same render path."
      className="inline-flex items-center gap-1.5 rounded-md border border-accent/35 bg-accent/10 px-2 py-1 text-2xs font-semibold uppercase tracking-wide text-accent"
    >
      <span className="h-1.5 w-1.5 rounded-full bg-accent" />
      Replay
    </span>
  );
}

// ---- idle picker panel ----------------------------------------------------

function PickerPanel({
  mode,
  artifacts,
  selected,
  onSelect,
  isReplayable,
  onDetonate,
  onReplay,
  disabled,
}: {
  mode: Mode;
  artifacts: Artifact[];
  selected: Artifact | null;
  onSelect: (a: Artifact) => void;
  isReplayable: (a: Artifact) => boolean;
  onDetonate: () => void;
  onReplay: () => void;
  disabled: boolean;
}) {
  return (
    <div className="rounded-2xl border border-line bg-surface p-4 sm:p-5">
      <div className="mb-3 flex items-center justify-between gap-2">
        <div>
          <h1 className="text-[15px] font-semibold tracking-tight text-fg">
            Drop an untrusted agent artifact
          </h1>
          <p className="mt-0.5 text-xs text-muted">
            Static scanners read the label. Reactor points a sacrificial victim agent at it and
            watches it behave.
          </p>
        </div>
        <ModeChip mode={mode} />
      </div>

      <ArtifactPicker
        artifacts={artifacts}
        selectedId={selected?.id ?? null}
        onSelect={onSelect}
        isReplayable={isReplayable}
        showOfflineTag={mode === "replay"}
        disabled={disabled}
      />

      <div className="mt-4 flex flex-wrap items-center gap-3 border-t border-line pt-4">
        <button
          type="button"
          onClick={onDetonate}
          disabled={disabled || !selected}
          className="inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2 text-sm font-semibold text-accent-fg shadow-glow-accent transition-transform hover:brightness-110 active:scale-[0.98] disabled:opacity-50"
        >
          <Zap size={15} />
          Detonate
        </button>
        <button
          type="button"
          onClick={onReplay}
          disabled={disabled}
          className="inline-flex items-center gap-2 rounded-lg border border-line bg-surface px-3.5 py-2 text-sm font-medium text-muted transition-colors hover:bg-surface-2 hover:text-fg disabled:opacity-50"
        >
          <Play size={14} />
          Replay demo
        </button>
        <span className="ml-auto inline-flex items-center gap-1.5 text-2xs text-faint">
          <Sparkles size={12} />
          {PLANNED_SESSIONS} detonations · fresh sandbox each · victim holds only bait
        </span>
      </div>
    </div>
  );
}

// ---- running / done header bar --------------------------------------------

function RunHeaderBar({
  mode,
  artifact,
  running,
  verdictLabel,
  onReplay,
  onDetonate,
  onReset,
}: {
  mode: Mode;
  artifact: Artifact | null;
  running: boolean;
  verdictLabel?: string;
  onReplay: () => void;
  onDetonate: () => void;
  onReset: () => void;
}) {
  const status = running
    ? { text: "detonating…", cls: "text-accent" }
    : verdictLabel === "MALICIOUS"
      ? { text: "blocked", cls: "text-danger" }
      : verdictLabel === "ALLOWED"
        ? { text: "allowed", cls: "text-success" }
        : verdictLabel
          ? { text: "complete", cls: "text-muted" }
          : { text: "ready", cls: "text-muted" };

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-line bg-surface px-4 py-3">
      <div className="flex min-w-0 items-center gap-3">
        <span
          className={cn(
            "h-2 w-2 shrink-0 rounded-full",
            running ? "animate-pulse-dot bg-accent" : verdictLabel === "MALICIOUS" ? "bg-danger" : verdictLabel === "ALLOWED" ? "bg-success" : "bg-faint",
          )}
        />
        <div className="min-w-0">
          <div className="truncate font-mono text-[13px] font-medium text-fg">{artifact?.name}</div>
          <div className={cn("text-2xs font-medium uppercase tracking-wide", status.cls)}>
            {status.text}
          </div>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <ModeChip mode={mode} />
        {!running && (
          <button
            type="button"
            onClick={onDetonate}
            className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-3 py-1.5 text-[13px] font-medium text-muted transition-colors hover:bg-surface-2 hover:text-fg"
          >
            <Zap size={13} />
            Re-detonate
          </button>
        )}
        <button
          type="button"
          onClick={onReplay}
          disabled={running}
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-3 py-1.5 text-[13px] font-medium text-muted transition-colors hover:bg-surface-2 hover:text-fg disabled:opacity-50"
        >
          <Play size={13} />
          Replay
        </button>
        <button
          type="button"
          onClick={onReset}
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-3 py-1.5 text-[13px] font-medium text-muted transition-colors hover:bg-surface-2 hover:text-fg"
        >
          <RotateCcw size={13} />
          New
        </button>
      </div>
    </div>
  );
}
