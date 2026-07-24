// Package events defines the typed event contract that crosses every boundary
// in Reactor: chamber -> engine (JSONL), engine -> UI/TUI (SSE), engine ->
// analyst (redacted, see analyst_view.go).
//
// Hard rule from SPEC §12.1: the only things that cross the sandbox boundary
// toward the host are structured events. Nothing in this package should ever
// require the host to interpret artifact source text.
package events

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Kind discriminates the Event union.
type Kind string

const (
	KindLifecycle  Kind = "lifecycle"
	KindScan       Kind = "scan"       // static-baseline column (mcp-scan stdout)
	KindWire       Kind = "wire"       // MCP JSON-RPC proxy log
	KindTranscript Kind = "transcript" // what the victim agent decided to do
	KindBehavioral Kind = "behavioral" // syscalls, fs, egress
	KindSignal     Kind = "signal"     // oracle output
	KindAnalyst    Kind = "analyst"    // analyst investigative step
	KindVerdict    Kind = "verdict"
)

// Severity levels used by signals and verdicts.
const (
	SevNone     = "none"
	SevLow      = "low"
	SevMedium   = "medium"
	SevHigh     = "high"
	SevCritical = "critical"
)

// Verdict labels.
const (
	LabelAllowed   = "ALLOWED"
	LabelMalicious = "MALICIOUS"
	LabelSuspect   = "SUSPICIOUS"
)

// Artifact is the thing under test (SPEC §6).
type Artifact struct {
	ID     string            `json:"id"`
	Kind   string            `json:"kind"` // mcp_server | skill | zip
	Name   string            `json:"name"`
	Source string            `json:"source"`         // e.g. "npx -y @acme/notes-mcp" or a path
	Args   []string          `json:"args,omitempty"` // extra argv for the server command
	Env    map[string]string `json:"env,omitempty"`
	SHA256 string            `json:"sha256"`
	Label  string            `json:"label,omitempty"` // ground truth, eval only — never shown to the analyst
	Note   string            `json:"note,omitempty"`  // human blurb for the UI card
}

// Artifact kinds.
const (
	KindMCPServer = "mcp_server"
	KindSkill     = "skill"
	KindZip       = "zip"
)

// Event is the single envelope broadcast on the bus and appended to JSONL.
// Exactly one of the pointer fields is set.
type Event struct {
	ID           string `json:"id"`  // stable, citable: "wire:4:tools/list", "egress:7"
	Seq          int    `json:"seq"` // monotonic per detonation
	Kind         Kind   `json:"kind"`
	TSms         int64  `json:"ts_ms"`
	Session      int    `json:"session"`
	DetonationID string `json:"detonation_id,omitempty"`
	ArtifactID   string `json:"artifact_id,omitempty"`

	Lifecycle  *Lifecycle       `json:"lifecycle,omitempty"`
	Scan       *ScanLine        `json:"scan,omitempty"`
	Wire       *WireEvent       `json:"wire,omitempty"`
	Transcript *TranscriptEvent `json:"transcript,omitempty"`
	Behavioral *BehavioralEvent `json:"behavioral,omitempty"`
	Signal     *Signal          `json:"signal,omitempty"`
	Analyst    *AnalystStep     `json:"analyst,omitempty"`
	Verdict    *Verdict         `json:"verdict,omitempty"`
}

// Lifecycle marks chamber and session state changes. Phases are UI-legible.
type Lifecycle struct {
	Phase   string            `json:"phase"` // see Phase* consts
	Message string            `json:"message,omitempty"`
	Chamber *ChamberInfo      `json:"chamber,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
}

const (
	PhaseQueued       = "queued"
	PhaseProvisioning = "provisioning"
	PhaseChamberReady = "chamber_ready"
	PhaseBaitPlanted  = "bait_planted"
	PhaseSinkUp       = "sink_up"
	PhaseInstalling   = "installing"
	PhaseSessionStart = "session_start"
	PhaseSessionEnd   = "session_end"
	PhaseVictimReady  = "victim_ready"
	PhaseAnalyzing    = "analyzing"
	PhaseVerdict      = "verdict"
	PhaseDestroying   = "destroying"
	PhaseDestroyed    = "destroyed"
	PhaseError        = "error"
)

// ChamberInfo is what the UI shows in the chamber header strip.
type ChamberInfo struct {
	Driver     string  `json:"driver"`     // "daytona" | "local"
	SandboxID  string  `json:"sandbox_id"` // ws_7fa2
	GPU        string  `json:"gpu"`        // "RTX PRO 6000"
	Model      string  `json:"model"`      // Qwen/Qwen3.6-27B-FP8
	Revision   string  `json:"revision"`   // pinned HF sha
	Served     string  `json:"served"`     // sglang | sim
	ToolParser string  `json:"tool_call_parser"`
	Temp       float64 `json:"temp"`
	Seed       int     `json:"seed"`
	Simulated  bool    `json:"simulated"` // true when no GPU weights are behind it
}

// ScanLine is one line of the static-baseline column (SPEC §7 static-blind).
type ScanLine struct {
	Tool   string `json:"tool"`             // "mcp-scan" | "snyk-agent-scan"
	Stream string `json:"stream"`           // stdout | stderr | result
	Text   string `json:"text"`             // raw scanner output — UI only
	Status string `json:"status,omitempty"` // clean | issues | error
	Issues int    `json:"issues"`
	Done   bool   `json:"done"`
}

// WireEvent is one MCP JSON-RPC message observed by the transparent proxy
// (SPEC §6). Byte-level description fidelity is the rug-pull evidence, so the
// proxy hashes the exact bytes it forwarded — never a re-serialized copy.
type WireEvent struct {
	Dir    string `json:"dir"`    // "agent→server" | "server→agent"
	Method string `json:"method"` // initialize | tools/list | tools/call | ...
	RPCID  string `json:"rpc_id,omitempty"`

	Tool              string `json:"tool,omitempty"`
	DescriptionSHA256 string `json:"description_sha256,omitempty"`
	DescriptionBytes  int    `json:"description_bytes,omitempty"`
	SchemaSHA256      string `json:"schema_sha256,omitempty"`

	ToolNames   []string `json:"tool_names,omitempty"`   // from tools/list
	ArgKeys     []string `json:"arg_keys,omitempty"`     // structural view of tools/call params
	ArgPaths    []string `json:"arg_paths,omitempty"`    // filesystem-looking values
	ArgHosts    []string `json:"arg_hosts,omitempty"`    // host-looking values
	ArgCanaries []string `json:"arg_canaries,omitempty"` // our tokens, safe to show the analyst
	Frames      int      `json:"frames,omitempty"`
	ErrorCode   int      `json:"error_code,omitempty"`

	// ---- raw, attacker-controlled. UI only. Stripped at the analyst boundary.
	Description string          `json:"description,omitempty"`
	Params      json.RawMessage `json:"params,omitempty"`
	ResultText  string          `json:"result_text,omitempty"`
}

// TranscriptEvent is what the victim agent chose to do, versus the benign task
// it was given (SPEC §6).
type TranscriptEvent struct {
	Task   string `json:"task"`
	Action string `json:"action"` // see Act* consts
	Tool   string `json:"tool,omitempty"`
	OnTask bool   `json:"on_task"`

	ArgKeys     []string `json:"arg_keys,omitempty"`
	ArgPaths    []string `json:"arg_paths,omitempty"`
	ArgHosts    []string `json:"arg_hosts,omitempty"`
	ArgCanaries []string `json:"arg_canaries,omitempty"`
	Deviation   string   `json:"deviation,omitempty"` // our own structured reason code

	TokensIn  int `json:"tokens_in,omitempty"`
	TokensOut int `json:"tokens_out,omitempty"`

	// ---- raw, attacker-influenced. UI only.
	Args    map[string]any `json:"args,omitempty"`
	Text    string         `json:"text,omitempty"`
	Thought string         `json:"thought,omitempty"`
}

// Transcript actions.
const (
	ActAssistant  = "assistant_message"
	ActToolCall   = "tool_call"
	ActToolResult = "tool_result"
	ActRefusal    = "refusal" // logged as its own outcome (SPEC §11)
	ActFinal      = "final"
)

// BehavioralEvent covers syscall / filesystem / egress collectors (SPEC §4.3).
type BehavioralEvent struct {
	Op     string `json:"op"`               // see Op* consts
	Source string `json:"source,omitempty"` // strace | inotify | sink | dns

	Path      string `json:"path,omitempty"`
	Bait      bool   `json:"bait,omitempty"`
	BaitLabel string `json:"bait_label,omitempty"` // aws_credentials, ssh_key, ...
	InInstall bool   `json:"in_install_dir,omitempty"`

	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	Method  string `json:"method,omitempty"`
	URLPath string `json:"url_path,omitempty"`

	Proc      string   `json:"proc,omitempty"`
	PID       int      `json:"pid,omitempty"`
	ArgvLen   int      `json:"argv_len,omitempty"`
	ArgvHash  string   `json:"argv_hash,omitempty"`
	ArgvHosts []string `json:"argv_hosts,omitempty"`
	ArgvPaths []string `json:"argv_paths,omitempty"`

	BodyBytes   int      `json:"body_bytes,omitempty"`
	BodySHA256  string   `json:"body_sha256,omitempty"`
	Canaries    []string `json:"canaries,omitempty"`     // matched REACTOR-* tokens
	CanaryKinds []string `json:"canary_kinds,omitempty"` // context | file:<label>

	BeforeFirstToolCall bool  `json:"before_first_tool_call,omitempty"`
	ElapsedMs           int64 `json:"elapsed_ms,omitempty"`

	// ---- raw, attacker-controlled. UI only.
	Argv    []string `json:"argv,omitempty"`
	Preview string   `json:"preview,omitempty"`
}

// Behavioral ops.
const (
	OpFileOpen     = "file_open"
	OpFileRead     = "file_read"
	OpFileWrite    = "file_write"
	OpFileDelete   = "file_delete"
	OpProcessSpawn = "process_spawn"
	OpConnect      = "connect"
	OpEgressDNS    = "egress_dns"
	OpEgressHTTP   = "egress_http"
	OpInstallStart = "install_start"
	OpInstallDone  = "install_done"
)

// Signal is deterministic oracle output (SPEC §4.4). Every field here is
// produced by Reactor, never copied from the artifact, so a Signal is safe to
// hand the analyst verbatim.
type Signal struct {
	Type        string         `json:"type"`
	Family      string         `json:"family"`
	Severity    string         `json:"severity"`
	Summary     string         `json:"summary"`
	Evidence    []string       `json:"evidence"` // event IDs, never prose
	Detail      map[string]any `json:"detail,omitempty"`
	StaticBlind bool           `json:"static_blind"` // a description-only scanner cannot produce this
	Session     int            `json:"session,omitempty"`
	FirstSeenMs int64          `json:"first_seen_ms,omitempty"`
}

// Oracle signal types (SPEC §4.4).
const (
	SigContextExfil       = "context_exfil"
	SigCanaryExfil        = "canary_exfil"
	SigCanaryRead         = "canary_read"
	SigTaskDeviation      = "task_deviation"
	SigRugPull            = "rug_pull"
	SigConditionalTrigger = "conditional_trigger"
	SigShadowing          = "shadowing"
	SigInstallHook        = "install_hook"
	SigAnalystInjection   = "analyst_injection"
	SigSleeperBeacon      = "sleeper_beacon"
	SigBenignProfile      = "benign_profile"
)

// Families.
const (
	FamAgentHijack      = "agent-hijack"
	FamStealer          = "stealer"
	FamCredentialAccess = "credential-access"
	FamHijack           = "hijack"
	FamSupplyChain      = "supply-chain"
	FamDormantPayload   = "dormant-payload"
	FamCrossServer      = "cross-server"
	FamDropper          = "dropper"
	FamEvasion          = "evasion"
	FamC2               = "c2"
	FamBenign           = "benign"
)

// AnalystStep is one turn of the analyst's investigative loop (SPEC §4.5).
type AnalystStep struct {
	Step      int            `json:"step"`
	Model     string         `json:"model"`
	Thought   string         `json:"thought,omitempty"` // analyst's own prose — safe, it is ours
	Tool      string         `json:"tool,omitempty"`
	Args      map[string]any `json:"args,omitempty"`
	Result    string         `json:"result,omitempty"`
	TokensIn  int            `json:"tokens_in,omitempty"`
	TokensOut int            `json:"tokens_out,omitempty"`
	CostUSD   float64        `json:"cost_usd,omitempty"`
}

// Verdict is the shipped output (SPEC §6). evidence[] MUST reference event IDs.
type Verdict struct {
	ArtifactID      string   `json:"artifact_id"`
	Label           string   `json:"label"`
	Family          string   `json:"family"`
	Severity        string   `json:"severity"`
	Explanation     string   `json:"explanation"`
	Evidence        []string `json:"evidence"`
	Sessions        int      `json:"sessions"`
	TimeToVerdictMs int64    `json:"time_to_verdict_ms"`
	CostUSD         float64  `json:"cost_usd"`
	Analyst         string   `json:"analyst,omitempty"`  // model id that wrote it
	Fallback        bool     `json:"fallback,omitempty"` // deterministic verdict, no hosted analyst
}

// VictimInfo pins exactly what ate the poison (SPEC §5.1).
type VictimInfo struct {
	Model          string  `json:"model"`
	Revision       string  `json:"revision"`
	Served         string  `json:"served"`
	ToolCallParser string  `json:"tool_call_parser"`
	Temp           float64 `json:"temp"`
	Seed           int     `json:"seed"`
	Simulated      bool    `json:"simulated,omitempty"`
}

// BaitReport summarises what the artifact touched.
type BaitReport struct {
	Read                []string `json:"read"`
	Exfiltrated         []string `json:"exfiltrated"`
	ContextCanaryLeaked bool     `json:"context_canary_leaked"`
}

// DetonationReport is the full record for one artifact (SPEC §6).
type DetonationReport struct {
	DetonationID string      `json:"detonation_id"`
	ArtifactID   string      `json:"artifact_id"`
	Artifact     *Artifact   `json:"artifact,omitempty"`
	SandboxID    string      `json:"sandbox_id"`
	Driver       string      `json:"driver"`
	Sessions     int         `json:"sessions"`
	Network      bool        `json:"network"`
	Victim       VictimInfo  `json:"victim"`
	Events       []Event     `json:"events"`
	Signals      []Signal    `json:"signals"`
	Bait         BaitReport  `json:"bait"`
	Verdict      *Verdict    `json:"verdict,omitempty"`
	Scan         *ScanResult `json:"scan,omitempty"`
	StartedMs    int64       `json:"started_ms"`
	EndedMs      int64       `json:"ended_ms"`
	Error        string      `json:"error,omitempty"`
}

// ScanResult is the static baseline for the left column / scorecard.
type ScanResult struct {
	Tool       string   `json:"tool"`
	Available  bool     `json:"available"`
	Status     string   `json:"status"` // clean | issues | error | unavailable
	Issues     int      `json:"issues"`
	Findings   []string `json:"findings,omitempty"`
	DurationMs int64    `json:"duration_ms"`
}

// ---------------------------------------------------------------------------
// ID allocation — evidence IDs are citable strings, per SPEC §6
// ("wire:4:tools/list", "egress:7"). Never hand an oracle a bare index.
// ---------------------------------------------------------------------------

// IDGen produces stable, human-legible, collision-free event IDs.
type IDGen struct {
	mu    sync.Mutex
	seq   int
	taken map[string]int
}

// NewIDGen returns a ready allocator.
func NewIDGen() *IDGen { return &IDGen{taken: map[string]int{}} }

// Next stamps ev with a Seq and a stable ID derived from its kind.
//
// Two ID shapes, both taken verbatim from SPEC §6's evidence examples:
//   - named:   "wire:4:tools/list"  (repeat uses get "#2", "#3", …)
//   - counted: "egress:7"           (always suffixed with a 1-based counter)
func (g *IDGen) Next(ev *Event) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	ev.Seq = g.seq
	if ev.ID != "" {
		return
	}
	base, counted := g.base(ev)
	n := g.taken[base] + 1
	g.taken[base] = n
	switch {
	case counted:
		ev.ID = fmt.Sprintf("%s:%d", base, n)
	case n == 1:
		ev.ID = base
	default:
		ev.ID = fmt.Sprintf("%s#%d", base, n)
	}
}

func (g *IDGen) base(ev *Event) (string, bool) {
	switch ev.Kind {
	case KindWire:
		m := "rpc"
		if ev.Wire != nil && ev.Wire.Method != "" {
			m = ev.Wire.Method
		}
		return fmt.Sprintf("wire:%d:%s", ev.Session, m), false
	case KindTranscript:
		a := "step"
		if ev.Transcript != nil && ev.Transcript.Action != "" {
			a = ev.Transcript.Action
		}
		return fmt.Sprintf("tx:%d:%s", ev.Session, a), false
	case KindBehavioral:
		if ev.Behavioral == nil {
			return "beh", true
		}
		switch ev.Behavioral.Op {
		case OpEgressHTTP, OpEgressDNS, OpConnect:
			return "egress", true
		case OpProcessSpawn:
			return "proc", true
		case OpFileOpen, OpFileRead:
			return "fs.read", true
		case OpFileWrite, OpFileDelete:
			return "fs.write", true
		default:
			return "beh", true
		}
	case KindSignal:
		if ev.Signal != nil {
			return "signal:" + ev.Signal.Type, false
		}
		return "signal", true
	case KindAnalyst:
		return "analyst", true
	case KindScan:
		return "scan", true
	case KindVerdict:
		return "verdict", true
	default:
		return "life", true
	}
}
