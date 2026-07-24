// Package daytona implements the chamber.Driver against a real Daytona sandbox
// — the sponsor story (SPEC §8): one disposable sandbox per detonation holding
// the artifact, the wire proxy, the victim, and the egress sink. On a GPU
// snapshot it also holds SGLang + the Qwen victim weights; on a CPU sandbox it
// runs the same topology with a hosted or simulated victim.
//
// The driver talks to the Daytona REST API directly (bearer token, no SDK
// dependency). It is gated behind an explicit opt-in (REACTOR_DRIVER=daytona)
// so the default demo path stays on the always-available local driver; flip the
// env to run detonations inside a live sandbox.
package daytona

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/reactor-sec/reactor/internal/chamber"
	"github.com/reactor-sec/reactor/internal/events"
)

const defaultBase = "https://app.daytona.io/api"

// defaultToolboxProxy is the region proxy that fronts every sandbox's toolbox
// daemon. The sandbox object normally carries it as toolboxProxyUrl; this is
// the fallback for the rare response that omits it.
const defaultToolboxProxy = "https://proxy.app.daytona.io/toolbox"

// Driver provisions Daytona sandboxes.
type Driver struct {
	base   string
	key    string
	client *http.Client
	// byok is true when the key came from a visitor request rather than host
	// env. BYOK drivers skip the REACTOR_DRIVER gate so a public UI can route
	// into the visitor's account without flipping the host default.
	byok bool

	upOnce   sync.Once
	upClient *http.Client
}

// uploadHTTP is the client used for binary uploads. It deliberately has no
// Timeout: the engine's binaries run to ~10 MB and a modest uplink puts that
// well past the 120 s that suits a control-plane call, so the request context is
// the only clock. Transport-level limits still catch a dead peer.
func (d *Driver) uploadHTTP() *http.Client {
	d.upOnce.Do(func() {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.ResponseHeaderTimeout = 5 * time.Minute
		d.upClient = &http.Client{Transport: tr}
	})
	return d.upClient
}

// New builds a Daytona driver from DAYTONA_API_KEY / DAYTONA_API_URL.
func New() *Driver {
	base := os.Getenv("DAYTONA_API_URL")
	if base == "" {
		base = defaultBase
	}
	return &Driver{
		base:   strings.TrimRight(base, "/"),
		key:    firstEnv("DAYTONA_API_KEY", "DAYTONA_KEY"),
		client: &http.Client{Timeout: 120 * time.Second},
		byok:   false,
	}
}

// NewWithKey builds a driver from an explicit API key (visitor BYOK). A key
// supplied this way is always Available — the REACTOR_DRIVER gate only applies
// to host-env credentials so a local demo does not accidentally bill Daytona.
func NewWithKey(apiKey, apiURL string) *Driver {
	base := strings.TrimSpace(apiURL)
	if base == "" {
		base = os.Getenv("DAYTONA_API_URL")
	}
	if base == "" {
		base = defaultBase
	}
	return &Driver{
		base:   strings.TrimRight(base, "/"),
		key:    strings.TrimSpace(apiKey),
		client: &http.Client{Timeout: 120 * time.Second},
		byok:   true,
	}
}

// Name implements chamber.Driver.
func (*Driver) Name() string { return "daytona" }

// Available reports whether a Daytona sandbox can be provisioned. Host-env
// drivers are gated behind REACTOR_DRIVER=daytona so the local driver stays
// the default; BYOK drivers (NewWithKey) are available whenever a key is set.
func (d *Driver) Available() bool {
	if d.key == "" {
		return false
	}
	if d.byok {
		return true
	}
	return strings.EqualFold(os.Getenv("REACTOR_DRIVER"), "daytona")
}

// Why implements chamber.Driver.
func (d *Driver) Why() string {
	switch {
	case d.key == "":
		return "set DAYTONA_API_KEY to run detonations inside a disposable Daytona sandbox"
	case d.byok:
		return "visitor BYOK — one disposable sandbox per detonation on their Daytona account"
	case !strings.EqualFold(os.Getenv("REACTOR_DRIVER"), "daytona"):
		return "configured; set REACTOR_DRIVER=daytona to route detonations into a live sandbox"
	default:
		return "one disposable sandbox per detonation (artifact + wire + victim + sink inside it)"
	}
}

// Provision creates a sandbox and returns a chamber bound to it.
func (d *Driver) Provision(ctx context.Context, spec chamber.Spec) (chamber.Chamber, error) {
	home := spec.Home
	if home == "" {
		home = "/home/daytona"
	}
	snapshot := spec.Snapshot
	if snapshot == "" {
		snapshot = os.Getenv("REACTOR_DAYTONA_SNAPSHOT")
	}
	body := map[string]any{
		"labels":             map[string]string{"reactor.detonation": spec.DetonationID, "reactor": "chamber"},
		"autoStopInterval":   15,
		"autoDeleteInterval": 30,
	}
	if snapshot != "" {
		body["snapshot"] = snapshot
	}
	var created struct {
		ID              string `json:"id"`
		State           string `json:"state"`
		ToolboxProxyURL string `json:"toolboxProxyUrl"`
		RunnerID        string `json:"runnerId"`
	}
	if err := d.do(ctx, http.MethodPost, "/sandbox", body, &created); err != nil {
		return nil, fmt.Errorf("create sandbox: %w", err)
	}
	if err := d.waitStarted(ctx, created.ID); err != nil {
		return nil, err
	}
	// The toolbox (exec + fs) is reached through the region proxy, not the
	// control-plane API: the sandbox object carries the proxy base as
	// toolboxProxyUrl and the sandbox id goes in the path, so a call looks like
	//
	//	POST {toolboxProxyUrl}/{sandboxId}/process/execute
	//
	// authenticated with the ordinary Daytona API key — the same credential as
	// the control plane, which matters for BYOK where no key is in host env.
	// Validated live end to end (create / exec / fs / destroy). The old
	// control-plane route {base}/toolbox/{id}/toolbox/... no longer exists.
	proxy := created.ToolboxProxyURL
	if proxy == "" {
		proxy = os.Getenv("REACTOR_DAYTONA_TOOLBOX_URL")
	}
	if proxy == "" {
		proxy = defaultToolboxProxy
	}
	c := &Chamber{d: d, id: created.ID, home: home, proxyURL: proxy,
		proxyToken: os.Getenv("REACTOR_DAYTONA_PROXY_TOKEN")}
	// Best-effort: discover the sandbox user's real home.
	if out, err := c.exec(ctx, "echo $HOME", "", nil, 10*time.Second); err == nil {
		if h := strings.TrimSpace(out.Result); h != "" {
			c.home = h
		}
	}
	return c, nil
}

func (d *Driver) waitStarted(ctx context.Context, id string) error {
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		var s struct {
			State string `json:"state"`
		}
		if err := d.do(ctx, http.MethodGet, "/sandbox/"+id, nil, &s); err == nil {
			switch strings.ToLower(s.State) {
			case "started", "running":
				return nil
			case "error", "build_failed", "destroyed":
				return fmt.Errorf("sandbox entered state %q", s.State)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("sandbox %s did not start in time", id)
}

// do performs a JSON REST call.
func (d *Driver) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.key)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, truncate(string(raw), 300))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

var _ = events.KindWire // keep events imported for signature symmetry with local
