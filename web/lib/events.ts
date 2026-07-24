// Typed mirror of internal/events/events.go — the frozen contract the UI renders
// against (docs/CONTRACT.md). Field names and shapes match the Go JSON tags
// exactly; a drift here is a rendering bug, so keep this file in lock-step with
// events.go. Everything the analyst must never see (raw description/params/argv
// text) is kept here because this is the *UI* view, which is allowed the raw
// bytes (it is the human, not the model).

export type Kind =
  | "lifecycle"
  | "scan"
  | "wire"
  | "transcript"
  | "behavioral"
  | "signal"
  | "analyst"
  | "verdict";

export type Severity = "none" | "low" | "medium" | "high" | "critical";

export type VerdictLabel = "ALLOWED" | "MALICIOUS" | "SUSPICIOUS";

export type ArtifactKind = "mcp_server" | "skill" | "zip";

export interface Artifact {
  id: string;
  kind: ArtifactKind | string;
  name: string;
  source: string;
  args?: string[];
  env?: Record<string, string>;
  sha256: string;
  label?: string; // ground truth, eval only — never shown to the analyst
  note?: string; // human blurb for the UI card
}

// ---- Lifecycle ------------------------------------------------------------

export type Phase =
  | "queued"
  | "provisioning"
  | "chamber_ready"
  | "bait_planted"
  | "sink_up"
  | "installing"
  | "session_start"
  | "session_end"
  | "victim_ready"
  | "analyzing"
  | "verdict"
  | "destroying"
  | "destroyed"
  | "error";

export interface ChamberInfo {
  driver: string; // "daytona" | "local"
  sandbox_id: string; // ws_7fa2
  gpu: string; // "RTX PRO 6000"
  model: string; // Qwen/Qwen3.6-27B-FP8
  revision: string;
  served: string; // sglang | sim
  tool_call_parser: string;
  temp: number;
  seed: number;
  simulated: boolean;
}

export interface Lifecycle {
  phase: Phase | string;
  message?: string;
  chamber?: ChamberInfo;
  meta?: Record<string, string>;
}

// ---- Scan (left column, static baseline) ----------------------------------

export interface ScanLine {
  tool: string; // "mcp-scan" | "snyk-agent-scan"
  stream: string; // stdout | stderr | result
  text: string;
  status?: string; // clean | issues | error
  issues: number;
  done: boolean;
}

export interface ScanResult {
  tool: string;
  available: boolean;
  status: string; // clean | issues | error | unavailable
  issues: number;
  findings?: string[];
  duration_ms: number;
}

// ---- Wire (MCP JSON-RPC proxy log) ----------------------------------------

export interface WireEvent {
  dir: string; // "agent→server" | "server→agent"
  method: string; // initialize | tools/list | tools/call | ...
  rpc_id?: string;

  tool?: string;
  description_sha256?: string;
  description_bytes?: number;
  schema_sha256?: string;

  tool_names?: string[];
  arg_keys?: string[];
  arg_paths?: string[];
  arg_hosts?: string[];
  arg_canaries?: string[];
  frames?: number;
  error_code?: number;

  // raw, attacker-controlled — UI only, stripped at the analyst boundary
  description?: string;
  params?: unknown;
  result_text?: string;
}

// ---- Transcript (what the victim chose to do) -----------------------------

export type TranscriptAction =
  | "assistant_message"
  | "tool_call"
  | "tool_result"
  | "refusal"
  | "final";

export interface TranscriptEvent {
  task: string;
  action: TranscriptAction | string;
  tool?: string;
  on_task: boolean;

  arg_keys?: string[];
  arg_paths?: string[];
  arg_hosts?: string[];
  arg_canaries?: string[];
  deviation?: string;

  tokens_in?: number;
  tokens_out?: number;

  // raw, attacker-influenced — UI only
  args?: Record<string, unknown>;
  text?: string;
  thought?: string;
}

// ---- Behavioral (syscalls / fs / egress) ----------------------------------

export type BehavioralOp =
  | "file_open"
  | "file_read"
  | "file_write"
  | "file_delete"
  | "process_spawn"
  | "connect"
  | "egress_dns"
  | "egress_http"
  | "install_start"
  | "install_done";

export interface BehavioralEvent {
  op: BehavioralOp | string;
  source?: string; // strace | inotify | sink | dns

  path?: string;
  bait?: boolean;
  bait_label?: string;
  in_install_dir?: boolean;

  host?: string;
  port?: number;
  method?: string;
  url_path?: string;

  proc?: string;
  pid?: number;
  argv_len?: number;
  argv_hash?: string;
  argv_hosts?: string[];
  argv_paths?: string[];

  body_bytes?: number;
  body_sha256?: string;
  canaries?: string[];
  canary_kinds?: string[]; // context | file:<label>

  before_first_tool_call?: boolean;
  elapsed_ms?: number;

  // raw, attacker-controlled — UI only
  argv?: string[];
  preview?: string;
}

// ---- Signal (deterministic oracle output) ---------------------------------

export type SignalType =
  | "context_exfil"
  | "canary_exfil"
  | "canary_read"
  | "task_deviation"
  | "rug_pull"
  | "conditional_trigger"
  | "shadowing"
  | "install_hook"
  | "analyst_injection"
  | "sleeper_beacon"
  | "benign_profile";

export interface Signal {
  type: SignalType | string;
  family: string;
  severity: Severity;
  summary: string;
  evidence: string[]; // event IDs, never prose
  detail?: Record<string, unknown>;
  static_blind: boolean;
  session?: number;
  first_seen_ms?: number;
}

// ---- Analyst (investigative loop) -----------------------------------------

export interface AnalystStep {
  step: number;
  model: string;
  thought?: string;
  tool?: string;
  args?: Record<string, unknown>;
  result?: string;
  tokens_in?: number;
  tokens_out?: number;
  cost_usd?: number;
}

// ---- Verdict --------------------------------------------------------------

export interface Verdict {
  artifact_id: string;
  label: VerdictLabel | string;
  family: string;
  severity: Severity;
  explanation: string;
  evidence: string[];
  sessions: number;
  time_to_verdict_ms: number;
  cost_usd: number;
  analyst?: string;
  fallback?: boolean;
}

export interface VictimInfo {
  model: string;
  revision: string;
  served: string;
  tool_call_parser: string;
  temp: number;
  seed: number;
  simulated?: boolean;
}

export interface BaitReport {
  read: string[];
  exfiltrated: string[];
  context_canary_leaked: boolean;
}

// ---- Event envelope -------------------------------------------------------

export interface ReactorEvent {
  id: string;
  seq: number;
  kind: Kind;
  ts_ms: number;
  session: number;
  detonation_id?: string;
  artifact_id?: string;

  lifecycle?: Lifecycle;
  scan?: ScanLine;
  wire?: WireEvent;
  transcript?: TranscriptEvent;
  behavioral?: BehavioralEvent;
  signal?: Signal;
  analyst?: AnalystStep;
  verdict?: Verdict;
}

export interface DetonationReport {
  detonation_id: string;
  artifact_id: string;
  artifact?: Artifact;
  sandbox_id: string;
  driver: string;
  sessions: number;
  network: boolean;
  victim: VictimInfo;
  events: ReactorEvent[];
  signals: Signal[];
  bait: BaitReport;
  verdict?: Verdict;
  scan?: ScanResult;
  started_ms: number;
  ended_ms: number;
  error?: string;
}

// ---- Health / detonate --------------------------------------------------

export interface HealthDriver {
  name: string;
  available: boolean;
  why?: string;
}

export interface Health {
  ok: boolean;
  drivers: HealthDriver[];
  analyst: string; // "grok-4.5" | "deterministic"
}

export interface DetonateResponse {
  detonation_id: string;
}

// ---- tiny runtime guard ---------------------------------------------------

const KINDS: ReadonlySet<string> = new Set([
  "lifecycle",
  "scan",
  "wire",
  "transcript",
  "behavioral",
  "signal",
  "analyst",
  "verdict",
]);

/** isReactorEvent is a cheap structural guard for SSE frames and fixtures. */
export function isReactorEvent(x: unknown): x is ReactorEvent {
  if (!x || typeof x !== "object") return false;
  const e = x as Record<string, unknown>;
  return typeof e.kind === "string" && KINDS.has(e.kind);
}
