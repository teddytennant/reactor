// The reducer folds the ordered event stream (live or replay) into the console's
// view state. It is the single place that: tracks per-tool descriptions to
// detect a rug pull and compute the byte-level diff; marks a session dirty the
// instant an oracle fires; and classifies egress as benign vs. exfiltration.

import type {
  AnalystStep,
  ChamberInfo,
  ReactorEvent,
  ScanLine,
  ScanResult,
  Signal,
  Verdict,
} from "./events";

export type SessionStatus = "running" | "clean" | "dirty";

export interface SessionView {
  n: number;
  status: SessionStatus;
  toolCount: number;
  baitCount: number;
  startedMs?: number;
  endedMs?: number;
}

export interface ByteDiff {
  id: string; // the wire event id that carries the mutation (evidence)
  tool: string;
  fromSession: number;
  toSession: number;
  fromBytes: number;
  toBytes: number;
  delta: number;
  prefix: string; // unchanged leading text
  added: string; // inserted run (highlighted)
  removed: string; // deleted run (rare)
  suffix: string; // unchanged trailing text
}

export interface WireLine {
  id: string;
  session: number;
  dir: string;
  method: string;
  tool?: string;
  descriptionBytes?: number;
  toolNames?: string[];
  argPaths?: string[];
  argCanaries?: string[];
  diff?: ByteDiff;
}

export interface TranscriptLine {
  id: string;
  session: number;
  action: string;
  tool?: string;
  onTask: boolean;
  task: string;
  argPaths?: string[];
  argCanaries?: string[];
  deviation?: string;
  text?: string;
}

export interface EgressLine {
  id: string;
  session: number;
  op: string;
  source?: string;
  host?: string;
  method?: string;
  urlPath?: string;
  port?: number;
  path?: string;
  bait?: boolean;
  baitLabel?: string;
  bodyBytes?: number;
  canaries?: string[];
  canaryKinds?: string[];
  malicious: boolean; // carried a canary or touched bait
}

export interface SignalView extends Signal {
  id: string;
}

export interface ProvisionStep {
  phase: string;
  message: string;
}

export interface ConsoleState {
  phase: string;
  chamber: ChamberInfo | null;
  detonationId: string | null;
  artifactName: string | null;
  provisioning: ProvisionStep[];
  scanLines: ScanLine[];
  scanResult: ScanResult | null;
  sessions: SessionView[];
  wire: WireLine[];
  transcript: TranscriptLine[];
  egress: EgressLine[];
  signals: SignalView[];
  diffs: ByteDiff[];
  analyst: AnalystStep[];
  verdict: Verdict | null;
  done: boolean;
  // internal: last-seen description bytes/text per tool, to diff a rug pull
  _descByTool: Record<string, { session: number; bytes: number; sha: string; text: string }>;
}

export function initialConsoleState(): ConsoleState {
  return {
    phase: "idle",
    chamber: null,
    detonationId: null,
    artifactName: null,
    provisioning: [],
    scanLines: [],
    scanResult: null,
    sessions: [],
    wire: [],
    transcript: [],
    egress: [],
    signals: [],
    diffs: [],
    analyst: [],
    verdict: null,
    done: false,
    _descByTool: {},
  };
}

// The lifecycle phases that make up the pre-session provisioning checklist.
const PROVISION_PHASES = new Set([
  "queued",
  "provisioning",
  "chamber_ready",
  "bait_planted",
  "sink_up",
  "installing",
]);

const PROVISION_LABEL: Record<string, string> = {
  queued: "Queued",
  provisioning: "Provisioning GPU sandbox",
  chamber_ready: "Chamber ready",
  bait_planted: "Bait credentials planted",
  sink_up: "Egress sink up",
  installing: "Installing artifact",
};

/** Insertion/deletion diff between two descriptions, anchored by shared ends. */
function textDiff(a: string, b: string) {
  const max = Math.min(a.length, b.length);
  let p = 0;
  while (p < max && a[p] === b[p]) p++;
  let s = 0;
  while (s < max - p && a[a.length - 1 - s] === b[b.length - 1 - s]) s++;
  return {
    prefix: b.slice(0, p),
    added: b.slice(p, b.length - s),
    removed: a.slice(p, a.length - s),
    suffix: s > 0 ? b.slice(b.length - s) : "",
  };
}

function egressOp(op: string): boolean {
  return op === "egress_http" || op === "egress_dns" || op === "connect";
}

export type ConsoleAction =
  | { type: "event"; ev: ReactorEvent }
  | { type: "meta"; detonationId?: string; artifactName?: string }
  | { type: "done" }
  | { type: "reset" };

export function consoleReducer(state: ConsoleState, action: ConsoleAction): ConsoleState {
  switch (action.type) {
    case "reset":
      return initialConsoleState();
    case "done":
      return { ...state, done: true };
    case "meta":
      return {
        ...state,
        detonationId: action.detonationId ?? state.detonationId,
        artifactName: action.artifactName ?? state.artifactName,
      };
    case "event":
      return applyEvent(state, action.ev);
    default:
      return state;
  }
}

function applyEvent(state: ConsoleState, ev: ReactorEvent): ConsoleState {
  switch (ev.kind) {
    case "lifecycle":
      return applyLifecycle(state, ev);
    case "scan":
      return applyScan(state, ev);
    case "wire":
      return applyWire(state, ev);
    case "transcript":
      return applyTranscript(state, ev);
    case "behavioral":
      return applyBehavioral(state, ev);
    case "signal":
      return applySignal(state, ev);
    case "analyst":
      return ev.analyst ? { ...state, analyst: [...state.analyst, ev.analyst] } : state;
    case "verdict":
      return ev.verdict
        ? { ...state, verdict: ev.verdict, phase: "verdict" }
        : state;
    default:
      return state;
  }
}

function applyLifecycle(state: ConsoleState, ev: ReactorEvent): ConsoleState {
  const lc = ev.lifecycle;
  if (!lc) return state;
  const next: ConsoleState = { ...state, phase: lc.phase };

  if (lc.chamber) next.chamber = lc.chamber;

  if (PROVISION_PHASES.has(lc.phase)) {
    next.provisioning = [
      ...state.provisioning,
      { phase: lc.phase, message: lc.message || PROVISION_LABEL[lc.phase] || lc.phase },
    ];
  }

  if (lc.phase === "session_start") {
    if (!state.sessions.some((s) => s.n === ev.session)) {
      next.sessions = [
        ...state.sessions,
        { n: ev.session, status: "running", toolCount: 0, baitCount: 0, startedMs: ev.ts_ms },
      ];
    }
  }

  if (lc.phase === "session_end") {
    // An explicit meta.status wins (e.g. a session that stays poisoned without
    // firing a *new* signal); otherwise a session is dirty iff an oracle fired.
    const forced = lc.meta?.status as SessionStatus | undefined;
    next.sessions = state.sessions.map((s) =>
      s.n === ev.session
        ? {
            ...s,
            status: forced ?? (s.status === "dirty" ? "dirty" : "clean"),
            endedMs: ev.ts_ms,
          }
        : s,
    );
  }

  return next;
}

function applyScan(state: ConsoleState, ev: ReactorEvent): ConsoleState {
  const sc = ev.scan;
  if (!sc) return state;
  const next: ConsoleState = { ...state, scanLines: [...state.scanLines, sc] };
  if (sc.done) {
    next.scanResult = {
      tool: sc.tool,
      available: true,
      status: sc.status || "clean",
      issues: sc.issues,
      findings: [],
      duration_ms: 0,
    };
  }
  return next;
}

function applyWire(state: ConsoleState, ev: ReactorEvent): ConsoleState {
  const w = ev.wire;
  if (!w) return state;

  const line: WireLine = {
    id: ev.id,
    session: ev.session,
    dir: w.dir,
    method: w.method,
    tool: w.tool,
    descriptionBytes: w.description_bytes,
    toolNames: w.tool_names,
    argPaths: w.arg_paths,
    argCanaries: w.arg_canaries,
  };

  const next: ConsoleState = { ...state };

  // Track tool count on the session from tools/list.
  if (w.tool_names && w.tool_names.length) {
    next.sessions = state.sessions.map((s) =>
      s.n === ev.session ? { ...s, toolCount: w.tool_names!.length } : s,
    );
  }

  // Rug-pull detection: a tool's description bytes changed across detonations.
  // The baseline (first clean description) is held stable so the diff reports
  // "session 1 → session 4", not "3 → 4".
  if (w.method === "tools/list" && w.tool && w.description) {
    const prev = state._descByTool[w.tool];
    const curr = {
      session: ev.session,
      bytes: w.description_bytes ?? w.description.length,
      sha: w.description_sha256 ?? "",
      text: w.description,
    };
    const changed = prev && (prev.sha !== curr.sha || prev.text !== curr.text);
    // Only (re)write the baseline on first sight or on an actual change.
    if (!prev || changed) {
      next._descByTool = { ...state._descByTool, [w.tool]: curr };
    }
    if (changed) {
      const d = textDiff(prev.text, curr.text);
      const diff: ByteDiff = {
        id: ev.id,
        tool: w.tool,
        fromSession: prev.session,
        toSession: ev.session,
        fromBytes: prev.bytes,
        toBytes: curr.bytes,
        delta: curr.bytes - prev.bytes,
        prefix: d.prefix,
        added: d.added,
        removed: d.removed,
        suffix: d.suffix,
      };
      line.diff = diff;
      next.diffs = [...state.diffs, diff];
    }
  }

  next.wire = [...state.wire, line];
  return next;
}

function applyTranscript(state: ConsoleState, ev: ReactorEvent): ConsoleState {
  const t = ev.transcript;
  if (!t) return state;
  const line: TranscriptLine = {
    id: ev.id,
    session: ev.session,
    action: t.action,
    tool: t.tool,
    onTask: t.on_task,
    task: t.task,
    argPaths: t.arg_paths,
    argCanaries: t.arg_canaries,
    deviation: t.deviation,
    text: t.text,
  };
  return { ...state, transcript: [...state.transcript, line] };
}

function applyBehavioral(state: ConsoleState, ev: ReactorEvent): ConsoleState {
  const b = ev.behavioral;
  if (!b) return state;
  const next: ConsoleState = { ...state };

  // Bait reads bump the session's bait counter.
  if (b.bait && (b.op === "file_open" || b.op === "file_read")) {
    next.sessions = state.sessions.map((s) =>
      s.n === ev.session ? { ...s, baitCount: s.baitCount + 1 } : s,
    );
  }

  if (egressOp(b.op) || b.bait) {
    const malicious = !!(b.canaries && b.canaries.length) || !!b.bait;
    const line: EgressLine = {
      id: ev.id,
      session: ev.session,
      op: b.op,
      source: b.source,
      host: b.host,
      method: b.method,
      urlPath: b.url_path,
      port: b.port,
      path: b.path,
      bait: b.bait,
      baitLabel: b.bait_label,
      bodyBytes: b.body_bytes,
      canaries: b.canaries,
      canaryKinds: b.canary_kinds,
      malicious,
    };
    next.egress = [...state.egress, line];
  }

  return next;
}

function applySignal(state: ConsoleState, ev: ReactorEvent): ConsoleState {
  const sig = ev.signal;
  if (!sig) return state;
  const view: SignalView = { ...sig, id: ev.id };
  const next: ConsoleState = { ...state, signals: [...state.signals, view] };

  // A fired malicious oracle marks its session dirty immediately.
  if (sig.type !== "benign_profile") {
    const sess = sig.session || ev.session;
    next.sessions = state.sessions.map((s) =>
      s.n === sess ? { ...s, status: "dirty" } : s,
    );
  }
  return next;
}
