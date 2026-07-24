"use client";

import { TriangleAlert } from "lucide-react";
import type { ChamberInfo } from "@/lib/events";
import { cn } from "@/lib/cn";
import { Led, type UiTone } from "@/components/ui";

/**
 * The chamber's identity bar — the header of the Reactor panel.
 *
 * Row 1 says what this column *is*, mirroring the scan column so the two read
 * as a genuine side-by-side: a name, then one sentence-case line of prose. Its
 * right-hand slot is deliberately empty at rest — a status word and a
 * detonation counter are only information once something is actually
 * happening, so both appear when the run starts and not before.
 *
 * Row 2 is the nameplate: the artifact under test, then the machine's own
 * facts — sans keys, mono values (detonation id, sandbox, GPU, victim model,
 * sampling parameters).
 *
 * Sans carries the chrome; mono is reserved for the machine data itself
 * (DESIGN §1 Type). Figures are `--fg`, their labels `--muted`; no accent
 * appears here at all — blue is spent on the primary action and selection
 * state, and a readout is neither. Uppercase is spent only on a genuine
 * verdict stamp (BLOCKED / ALLOWED) — never on DETONATING.
 */
export type ChamberStatus = "idle" | "running" | "blocked" | "allowed" | "suspicious";

const STATUS: Record<
  ChamberStatus,
  { label: string; led: UiTone; text: string; pulse?: boolean; stamp?: boolean }
> = {
  // Idle is never painted — it only names the resting state for the live region.
  idle: { label: "Standby", led: "neutral", text: "text-faint" },
  running: { label: "Detonating", led: "live", text: "text-live", pulse: true },
  blocked: { label: "Blocked", led: "danger", text: "text-danger", stamp: true },
  allowed: { label: "Allowed", led: "success", text: "text-success", stamp: true },
  suspicious: { label: "Suspicious", led: "warning", text: "text-warning" },
};

export function ChamberHeader({
  chamber,
  artifactName,
  detonationId,
  sessionCount = 0,
  plannedSessions = 0,
  status = "idle",
}: {
  chamber: ChamberInfo | null;
  artifactName?: string | null;
  detonationId?: string | null;
  sessionCount?: number;
  plannedSessions?: number;
  status?: ChamberStatus;
}) {
  const s = STATUS[status];
  const idle = status === "idle";

  return (
    <div className="shrink-0 border-b border-line">
      {/* --- identity: what this column is, and what it is doing ------------ */}
      <div className="flex items-start justify-between gap-4 px-4 py-4">
        <div className="min-w-0">
          <div className="text-sm font-medium text-fg">Reactor</div>
          <p className="mt-1 text-sm leading-relaxed text-muted">
            A runtime detonation chamber. It runs the artifact and watches how it behaves.
          </p>
        </div>

        <div className="flex shrink-0 items-center gap-3">
          {/* The live region stays mounted so a state change is announced; at
              rest it renders nothing — "Standby" only repeated what the body
              already says, and an idle readout is not information. */}
          <span
            className="inline-flex items-center gap-2"
            role="status"
            aria-label={`Chamber ${s.label}`}
          >
            {!idle && (
              <>
                <Led tone={s.led} pulse={s.pulse} />
                <span
                  className={cn(
                    s.stamp
                      ? "text-2xs font-semibold uppercase tracking-label-wide"
                      : "text-sm font-medium",
                    s.text,
                  )}
                >
                  {s.label}
                </span>
              </>
            )}
          </span>

          {/* Useful during a run (Det 3/5), noise before one. */}
          {!idle && plannedSessions > 0 && (
            <span
              className="inline-flex shrink-0 items-baseline gap-1.5"
              title={`${sessionCount} of ${plannedSessions} detonations`}
            >
              <span className="text-sm text-muted">Det</span>
              <span className="tnum font-mono text-2xs">
                <span className="font-medium text-fg">{sessionCount}</span>
                <span className="text-muted">/{plannedSessions}</span>
              </span>
            </span>
          )}
        </div>
      </div>

      {/* --- nameplate: the subject, then sans keys and mono values --------- */}
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1.5 border-t border-line px-4 py-2.5">
        <span
          className="inline-flex min-w-0 items-baseline gap-1.5"
          title={artifactName ?? undefined}
        >
          <span className="whitespace-nowrap text-2xs text-faint">Artifact</span>
          <span
            className={cn(
              "min-w-0 truncate text-2xs",
              artifactName ? "font-mono font-medium text-fg" : "text-muted",
            )}
          >
            {artifactName ?? "No artifact loaded"}
          </span>
        </span>

        {detonationId && <Meta k="Det" v={detonationId} />}
        {chamber ? (
          <>
            <Meta k="Sandbox" v={chamber.sandbox_id} />
            <Meta k="GPU" v={chamber.gpu} />
            <Meta k="Victim" v={shortModel(chamber.model)} strong />
            <Meta k="Served" v={chamber.served} />
            <Meta k="Temp" v={String(chamber.temp)} />
            <Meta k="Seed" v={String(chamber.seed)} />
            <Meta
              k="Rev"
              v={chamber.revision.slice(0, 7)}
              title={`revision ${chamber.revision}`}
            />
            {chamber.simulated && (
              <span className="inline-flex items-center gap-1.5 rounded-lg bg-warning/10 px-2 py-0.5 text-2xs font-medium text-warning">
                <TriangleAlert size={11} aria-hidden="true" />
                Simulated victim
              </span>
            )}
          </>
        ) : (
          <span className="text-2xs text-faint">Chamber not provisioned</span>
        )}
      </div>
    </div>
  );
}

/** One nameplate field: a sentence-case sans key, then its tabular mono value. */
function Meta({
  k,
  v,
  strong,
  title,
}: {
  k: string;
  v: string;
  strong?: boolean;
  title?: string;
}) {
  return (
    <span className="inline-flex min-w-0 items-baseline gap-1.5" title={title}>
      <span className="whitespace-nowrap text-2xs text-faint">{k}</span>
      <span
        className={cn(
          "tnum truncate font-mono text-2xs",
          strong ? "text-fg" : "text-muted",
        )}
      >
        {v}
      </span>
    </span>
  );
}

function shortModel(model: string): string {
  const i = model.lastIndexOf("/");
  return i >= 0 ? model.slice(i + 1) : model;
}
