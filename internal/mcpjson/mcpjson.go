// Package mcpjson is a deliberately thin MCP/JSON-RPC layer.
//
// Why not an MCP SDK: the wire proxy's whole job is byte-level fidelity of tool
// descriptions across sessions — that is the rug_pull evidence. A typed SDK
// unmarshals into structs and re-serialises, which normalises key order,
// whitespace and unicode escapes and quietly destroys the exact thing we are
// measuring. So the proxy forwards the original bytes untouched and inspects a
// *copy*. 200 lines of stdlib beats a dependency that launders the evidence.
package mcpjson

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Message is a JSON-RPC 2.0 frame with the raw bytes retained.
type Message struct {
	Raw     []byte
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is the JSON-RPC error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// IDString renders the request id for logging without asserting its type.
func (m *Message) IDString() string {
	if len(m.ID) == 0 {
		return ""
	}
	return strings.Trim(string(m.ID), `"`)
}

// Tool is one entry of a tools/list result.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Title       string          `json:"title,omitempty"`
}

// ToolsListResult is the shape of a tools/list response.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// CallParams is the shape of tools/call params.
type CallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// CallResult is the shape of a tools/call response.
type CallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content is one content block.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Reader reads newline-delimited JSON-RPC frames from an MCP stdio transport.
type Reader struct{ sc *bufio.Scanner }

// NewReader wraps r. MCP stdio frames are newline-delimited JSON; the buffer is
// sized for pathological tool descriptions because the whole point is that a
// server may serve an enormous one.
func NewReader(r io.Reader) *Reader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return &Reader{sc: sc}
}

// Next returns the next frame, or io.EOF. Blank lines are skipped. Lines that
// are not JSON are returned as a Message with only Raw set, so a proxy can
// forward server chatter verbatim without dropping it.
func (r *Reader) Next() (*Message, error) {
	for r.sc.Scan() {
		line := r.sc.Bytes()
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			continue
		}
		raw := append([]byte(nil), line...)
		m := &Message{Raw: raw}
		if err := json.Unmarshal(raw, m); err != nil {
			m.Raw = raw
			return m, nil
		}
		m.Raw = raw
		return m, nil
	}
	if err := r.sc.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// WriteFrame writes raw bytes plus the newline terminator.
func WriteFrame(w io.Writer, raw []byte) error {
	if _, err := w.Write(raw); err != nil {
		return err
	}
	_, err := w.Write([]byte("\n"))
	return err
}

// Encode marshals v into a frame.
func Encode(v any) ([]byte, error) { return json.Marshal(v) }

// Request builds a client request frame.
func Request(id int, method string, params any) ([]byte, error) {
	obj := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		obj["params"] = params
	}
	return json.Marshal(obj)
}

// Notification builds a client notification frame.
func Notification(method string, params any) ([]byte, error) {
	obj := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		obj["params"] = params
	}
	return json.Marshal(obj)
}

// ---------------------------------------------------------------------------
// Structural extraction — how attacker-controlled values reach the analyst.
//
// The analyst never sees a tool argument's text. It sees which keys were
// present, which values were path-shaped, and which were host-shaped. That is
// enough to reason about "the agent attached a filesystem path to a search
// call" without ever reading the artifact's prose.
// ---------------------------------------------------------------------------

var (
	pathRe = regexp.MustCompile(`(?:^|[\s"'=:,\(\[])((?:~|\.{0,2})/[A-Za-z0-9_.\-/]{2,})`)
	hostRe = regexp.MustCompile(`(?i)\b((?:[a-z0-9](?:[a-z0-9\-]{0,61}[a-z0-9])?\.)+(?:[a-z]{2,24}))\b`)
	ipRe   = regexp.MustCompile(`\b((?:\d{1,3}\.){3}\d{1,3})\b`)
)

// ArgKeys returns sorted top-level argument names.
func ArgKeys(args map[string]any) []string {
	out := make([]string, 0, len(args))
	for k := range args {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Flatten renders nested argument values into a single scannable string,
// used only for structural extraction and canary matching.
func Flatten(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// ExtractPaths returns unique filesystem-shaped substrings.
func ExtractPaths(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range pathRe.FindAllStringSubmatch(s, -1) {
		p := strings.TrimRight(m[1], `.,;:"')`)
		if len(p) < 3 || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ExtractHosts returns unique host-shaped substrings, including bare IPs and
// the host component of any URL.
func ExtractHosts(s string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		h = strings.ToLower(strings.Trim(h, "./"))
		if h == "" || seen[h] {
			return
		}
		seen[h] = true
		out = append(out, h)
	}
	for _, f := range strings.Fields(strings.NewReplacer(`"`, " ", `'`, " ", ",", " ").Replace(s)) {
		if strings.Contains(f, "://") {
			if u, err := url.Parse(f); err == nil && u.Host != "" {
				add(u.Hostname())
			}
		}
	}
	for _, m := range hostRe.FindAllStringSubmatch(s, -1) {
		// Skip things that are really filenames (config.json, main.go).
		if isFileish(m[1]) {
			continue
		}
		add(m[1])
	}
	for _, m := range ipRe.FindAllStringSubmatch(s, -1) {
		add(m[1])
	}
	sort.Strings(out)
	return out
}

var fileExts = map[string]bool{
	"json": true, "js": true, "ts": true, "go": true, "py": true, "md": true, "txt": true,
	"yml": true, "yaml": true, "toml": true, "lock": true, "sh": true, "sqlite": true,
	"dat": true, "env": true, "log": true, "sum": true, "mod": true, "tsx": true, "jsx": true,
	"png": true, "svg": true, "css": true, "html": true, "rs": true, "cfg": true, "ini": true,
}

func isFileish(h string) bool {
	i := strings.LastIndex(h, ".")
	if i < 0 {
		return false
	}
	return fileExts[strings.ToLower(h[i+1:])]
}
