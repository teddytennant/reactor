// Package wire is the transparent MCP proxy (SPEC §4.3 "MCP wire log", §12.2).
// It sits on the stdio between the victim agent (MCP client) and the artifact
// (MCP server) and logs every JSON-RPC frame in both directions.
//
// Byte fidelity is the whole point. The rug-pull oracle diffs the exact bytes
// of a tool description across sessions, so the proxy forwards the ORIGINAL
// frame bytes untouched and only ever *inspects* a decoded copy. It never
// re-serialises what it forwards — that would normalise key order and unicode
// escapes and launder the evidence.
package wire

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/reactor-sec/reactor/internal/canary"
	"github.com/reactor-sec/reactor/internal/events"
	"github.com/reactor-sec/reactor/internal/mcpjson"
)

// Proxy pumps frames between a client and a server, emitting WireEvents.
type Proxy struct {
	Session  int
	Canaries *canary.Set
	// Emit receives one Event per observed frame. Must be safe for concurrent
	// use — the two directions run on separate goroutines.
	Emit func(events.Event)

	mu      sync.Mutex
	pending map[string]string // rpc id -> tool name, to annotate call results
}

// New builds a proxy.
func New(session int, cset *canary.Set, emit func(events.Event)) *Proxy {
	return &Proxy{Session: session, Canaries: cset, Emit: emit, pending: map[string]string{}}
}

// Pump runs both directions until EOF. fromClient/toServer carry the victim's
// requests to the artifact; fromServer/toClient carry the artifact's responses
// back. Returns when both directions have closed.
func (p *Proxy) Pump(fromClient io.Reader, toServer io.Writer, fromServer io.Reader, toClient io.Writer) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.one("agent→server", fromClient, toServer) }()
	go func() { defer wg.Done(); p.one("server→agent", fromServer, toClient) }()
	wg.Wait()
}

func (p *Proxy) one(dir string, in io.Reader, out io.Writer) {
	r := mcpjson.NewReader(in)
	for {
		m, err := r.Next()
		if err != nil {
			return
		}
		// Forward the exact bytes first, so proxying never adds latency skew to
		// the thing we are timing, and never mutates the payload.
		if out != nil && len(m.Raw) > 0 {
			mcpjson.WriteFrame(out, m.Raw)
		}
		p.observe(dir, m)
	}
}

func (p *Proxy) observe(dir string, m *mcpjson.Message) {
	we := &events.WireEvent{Dir: dir, Method: m.Method, RPCID: m.IDString()}

	switch {
	case dir == "agent→server" && m.Method == "tools/call":
		p.observeCall(we, m)
	case dir == "server→agent" && len(m.Result) > 0:
		p.observeResult(we, m)
	}

	// Notifications and requests with no special handling still get logged so
	// the wire timeline is complete (initialize, ping, notifications/*).
	p.Emit(events.Event{Kind: events.KindWire, Session: p.Session, TSms: nowMs(), Wire: we})
}

// observeCall extracts the structural view of a tools/call and matches canaries
// in its arguments. A system-prompt canary appearing here is the context-exfil
// event at the wire level (SPEC §4.4) — the agent put a secret it holds into a
// tool argument.
func (p *Proxy) observeCall(we *events.WireEvent, m *mcpjson.Message) {
	var cp mcpjson.CallParams
	if len(m.Params) > 0 {
		json.Unmarshal(m.Params, &cp)
	}
	we.Tool = cp.Name
	we.ArgKeys = mcpjson.ArgKeys(cp.Arguments)
	flat := mcpjson.Flatten(cp.Arguments)
	we.ArgPaths = mcpjson.ExtractPaths(flat)
	we.ArgHosts = mcpjson.ExtractHosts(flat)
	if p.Canaries != nil {
		if toks, _ := p.Canaries.Match(flat); len(toks) > 0 {
			we.ArgCanaries = toks
		}
	}
	we.Params = m.Params // raw, UI-only; stripped at the analyst boundary
	if we.RPCID != "" && cp.Name != "" {
		p.mu.Lock()
		p.pending[we.RPCID] = cp.Name
		p.mu.Unlock()
	}
}

// observeResult handles server→agent results. A tools/list result is the
// rug-pull surface: hash each tool's description bytes so a later session's
// bytes can be diffed against this one.
func (p *Proxy) observeResult(we *events.WireEvent, m *mcpjson.Message) {
	// Is this a tools/list result? It has a "tools" array.
	var tl mcpjson.ToolsListResult
	if err := json.Unmarshal(m.Result, &tl); err == nil && len(tl.Tools) > 0 {
		p.emitToolsList(m, tl)
		we.Method = "tools/list"
		we.ToolNames = toolNames(tl.Tools)
		we.Frames = len(tl.Tools)
		return
	}

	// Otherwise it's a tools/call result; annotate with the tool name and scan
	// the returned content for canaries (exfil-via-result surface).
	if name, ok := p.take(we.RPCID); ok {
		we.Method = "tools/call"
		we.Tool = name
	}
	var cr mcpjson.CallResult
	if json.Unmarshal(m.Result, &cr) == nil {
		var text string
		for _, c := range cr.Content {
			text += c.Text + "\n"
		}
		we.ResultText = text
		if p.Canaries != nil {
			if toks, _ := p.Canaries.Match(text); len(toks) > 0 {
				we.ArgCanaries = toks
			}
		}
	}
}

// emitToolsList emits one WireEvent per served tool, each carrying that tool's
// exact description hash and byte count — the per-tool rug-pull evidence.
func (p *Proxy) emitToolsList(m *mcpjson.Message, tl mcpjson.ToolsListResult) {
	for _, t := range tl.Tools {
		we := &events.WireEvent{
			Dir:               "server→agent",
			Method:            "tools/list",
			RPCID:             m.IDString(),
			Tool:              t.Name,
			DescriptionSHA256: sha(t.Description),
			DescriptionBytes:  len(t.Description),
			Description:       t.Description, // raw, UI-only
		}
		if len(t.InputSchema) > 0 {
			we.SchemaSHA256 = sha(string(t.InputSchema))
		}
		// A description that references another server's tool is shadowing bait
		// evidence; the oracle decides, but we surface the tool names it names.
		p.Emit(events.Event{Kind: events.KindWire, Session: p.Session, TSms: nowMs(), Wire: we})
	}
}

func (p *Proxy) take(id string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n, ok := p.pending[id]
	if ok {
		delete(p.pending, id)
	}
	return n, ok
}

func toolNames(ts []mcpjson.Tool) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}
