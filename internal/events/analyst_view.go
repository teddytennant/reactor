package events

// The analyst boundary.
//
// SPEC §4.1: the analyst "sees raw untrusted text? Never. Only typed events."
// SPEC §10: "the analyst is protected by architecture, not by prompt."
//
// That promise is only real if it is enforced by construction, so this file
// builds the analyst's view as *separate types with an explicit allowlist*
// rather than by blanking fields on the wire types. A new field added to
// WireEvent cannot leak to the analyst by forgetting a tag: it simply does not
// exist on the other side of this file until someone deliberately copies it,
// and analyst_view_test.go fails the build if attacker prose ever survives.
//
// Everything below is either produced by Reactor (hashes, counts, canary ids,
// oracle summaries) or is a structural projection of attacker data (key names,
// path-shaped values, host-shaped values) — never free text.

// AnalystEvent is the redacted projection of Event handed to the analyst model.
type AnalystEvent struct {
	ID      string `json:"id"`
	Kind    Kind   `json:"kind"`
	TSms    int64  `json:"ts_ms"`
	Session int    `json:"session"`

	Wire       *AnalystWire       `json:"wire,omitempty"`
	Transcript *AnalystTranscript `json:"transcript,omitempty"`
	Behavioral *AnalystBehavioral `json:"behavioral,omitempty"`
	Signal     *Signal            `json:"signal,omitempty"` // Reactor-authored, safe verbatim
	Lifecycle  *AnalystLifecycle  `json:"lifecycle,omitempty"`
}

// AnalystWire drops Description, Params and ResultText.
type AnalystWire struct {
	Dir               string   `json:"dir"`
	Method            string   `json:"method"`
	Tool              string   `json:"tool,omitempty"`
	DescriptionSHA256 string   `json:"description_sha256,omitempty"`
	DescriptionBytes  int      `json:"description_bytes,omitempty"`
	SchemaSHA256      string   `json:"schema_sha256,omitempty"`
	ToolNames         []string `json:"tool_names,omitempty"`
	ArgKeys           []string `json:"arg_keys,omitempty"`
	ArgPaths          []string `json:"arg_paths,omitempty"`
	ArgHosts          []string `json:"arg_hosts,omitempty"`
	ArgCanaries       []string `json:"arg_canaries,omitempty"`
	ErrorCode         int      `json:"error_code,omitempty"`
}

// AnalystTranscript drops Args, Text and Thought.
type AnalystTranscript struct {
	Action      string   `json:"action"`
	Tool        string   `json:"tool,omitempty"`
	OnTask      bool     `json:"on_task"`
	ArgKeys     []string `json:"arg_keys,omitempty"`
	ArgPaths    []string `json:"arg_paths,omitempty"`
	ArgHosts    []string `json:"arg_hosts,omitempty"`
	ArgCanaries []string `json:"arg_canaries,omitempty"`
	Deviation   string   `json:"deviation,omitempty"`
}

// AnalystBehavioral drops Argv and Preview.
type AnalystBehavioral struct {
	Op                  string   `json:"op"`
	Path                string   `json:"path,omitempty"`
	Bait                bool     `json:"bait,omitempty"`
	BaitLabel           string   `json:"bait_label,omitempty"`
	InInstall           bool     `json:"in_install_dir,omitempty"`
	Host                string   `json:"host,omitempty"`
	Port                int      `json:"port,omitempty"`
	Method              string   `json:"method,omitempty"`
	URLPath             string   `json:"url_path,omitempty"`
	Proc                string   `json:"proc,omitempty"`
	ArgvLen             int      `json:"argv_len,omitempty"`
	ArgvHash            string   `json:"argv_hash,omitempty"`
	ArgvHosts           []string `json:"argv_hosts,omitempty"`
	ArgvPaths           []string `json:"argv_paths,omitempty"`
	BodyBytes           int      `json:"body_bytes,omitempty"`
	BodySHA256          string   `json:"body_sha256,omitempty"`
	Canaries            []string `json:"canaries,omitempty"`
	CanaryKinds         []string `json:"canary_kinds,omitempty"`
	BeforeFirstToolCall bool     `json:"before_first_tool_call,omitempty"`
	ElapsedMs           int64    `json:"elapsed_ms,omitempty"`
}

// AnalystLifecycle keeps phase only — no artifact-authored Message.
type AnalystLifecycle struct {
	Phase string `json:"phase"`
}

// ForAnalyst projects an Event into its redacted form. It returns nil for
// kinds the analyst has no business reading at all (scan output is the other
// vendor's prose; analyst steps are its own transcript; verdicts are output).
func (e Event) ForAnalyst() *AnalystEvent {
	out := &AnalystEvent{ID: e.ID, Kind: e.Kind, TSms: e.TSms, Session: e.Session}
	switch e.Kind {
	case KindWire:
		if e.Wire == nil {
			return nil
		}
		w := e.Wire
		out.Wire = &AnalystWire{
			Dir: w.Dir, Method: w.Method, Tool: w.Tool,
			DescriptionSHA256: w.DescriptionSHA256, DescriptionBytes: w.DescriptionBytes,
			SchemaSHA256: w.SchemaSHA256, ToolNames: w.ToolNames,
			ArgKeys: w.ArgKeys, ArgPaths: w.ArgPaths, ArgHosts: w.ArgHosts,
			ArgCanaries: w.ArgCanaries, ErrorCode: w.ErrorCode,
		}
	case KindTranscript:
		if e.Transcript == nil {
			return nil
		}
		t := e.Transcript
		out.Transcript = &AnalystTranscript{
			Action: t.Action, Tool: t.Tool, OnTask: t.OnTask,
			ArgKeys: t.ArgKeys, ArgPaths: t.ArgPaths, ArgHosts: t.ArgHosts,
			ArgCanaries: t.ArgCanaries, Deviation: t.Deviation,
		}
	case KindBehavioral:
		if e.Behavioral == nil {
			return nil
		}
		b := e.Behavioral
		out.Behavioral = &AnalystBehavioral{
			Op: b.Op, Path: b.Path, Bait: b.Bait, BaitLabel: b.BaitLabel, InInstall: b.InInstall,
			Host: b.Host, Port: b.Port, Method: b.Method, URLPath: b.URLPath,
			Proc: b.Proc, ArgvLen: b.ArgvLen, ArgvHash: b.ArgvHash,
			ArgvHosts: b.ArgvHosts, ArgvPaths: b.ArgvPaths,
			BodyBytes: b.BodyBytes, BodySHA256: b.BodySHA256,
			Canaries: b.Canaries, CanaryKinds: b.CanaryKinds,
			BeforeFirstToolCall: b.BeforeFirstToolCall, ElapsedMs: b.ElapsedMs,
		}
	case KindSignal:
		if e.Signal == nil {
			return nil
		}
		s := *e.Signal
		out.Signal = &s
	case KindLifecycle:
		if e.Lifecycle == nil {
			return nil
		}
		out.Lifecycle = &AnalystLifecycle{Phase: e.Lifecycle.Phase}
	default:
		return nil
	}
	return out
}

// ForAnalystSlice projects a stream, dropping events with no analyst view.
func ForAnalystSlice(evs []Event) []AnalystEvent {
	out := make([]AnalystEvent, 0, len(evs))
	for _, e := range evs {
		if a := e.ForAnalyst(); a != nil {
			out = append(out, *a)
		}
	}
	return out
}
