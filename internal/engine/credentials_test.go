package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reactor-sec/reactor/internal/chamber/daytona"
)

func TestCredentialsFromRequestMerge(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/detonate", nil)
	r.Header.Set("X-Reactor-Daytona-Key", "hdr-dt")
	r.Header.Set("X-Reactor-Fireworks-Key", "hdr-fw")
	r.Header.Set("X-Reactor-Daytona-Url", "https://hdr.example/api")

	// Body wins over headers when both are set.
	got := credentialsFromRequest(r, RunCredentials{
		DaytonaAPIKey:   " body-dt \n",
		FireworksAPIKey: "",
	})
	if got.DaytonaAPIKey != "body-dt" {
		t.Fatalf("daytona key = %q, want body-dt (trimmed, body wins)", got.DaytonaAPIKey)
	}
	if got.FireworksAPIKey != "hdr-fw" {
		t.Fatalf("fireworks key = %q, want hdr-fw (header fills empty body)", got.FireworksAPIKey)
	}
	if got.DaytonaAPIURL != "https://hdr.example/api" {
		t.Fatalf("daytona url = %q", got.DaytonaAPIURL)
	}
}

func TestDriverForBYOK(t *testing.T) {
	e := newTestEngine(t)
	d := e.driverFor(RunCredentials{DaytonaAPIKey: "dtn_test"})
	if d.Name() != "daytona" {
		t.Fatalf("driver = %s, want daytona", d.Name())
	}
	if !d.Available() {
		t.Fatal("BYOK daytona driver must be Available without REACTOR_DRIVER")
	}
	// No key → falls back to local (test engine has no host Daytona).
	d2 := e.driverFor(RunCredentials{})
	if d2.Name() != "local" {
		t.Fatalf("empty creds driver = %s, want local", d2.Name())
	}
}

func TestDetonateAcceptsCredentialsWithoutEchoing(t *testing.T) {
	e := newTestEngine(t)
	// Detonate will try to provision; with sim victim + local chamber the run
	// may fail on missing real binaries behaviour, but the request path must
	// accept credentials and never put them on the wire in the response.
	body := `{"artifact_id":"art_notes_mcp","sessions":1,"credentials":{"daytona_api_key":"SECRET_DT","fireworks_api_key":"SECRET_FW"}}`
	rec := do(t, e, "POST", "/api/detonate", body)
	if rec.Code != http.StatusOK {
		// A 400 "unknown artifact" would mean the body failed to decode — bad.
		// Provision/runtime failures still return an id first; decode failures don't.
		if rec.Code == http.StatusBadRequest && strings.Contains(rec.Body.String(), "invalid") {
			t.Fatalf("credentials broke request decode: %s", rec.Body.String())
		}
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "SECRET_DT") || strings.Contains(raw, "SECRET_FW") {
		t.Fatalf("credentials leaked into HTTP response: %s", raw)
	}
	if rec.Code == http.StatusOK {
		var res struct {
			ID string `json:"detonation_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.ID == "" {
			t.Fatalf("expected detonation_id, got %s", rec.Body.String())
		}
		// Report snapshot must not carry the secrets either.
		rep, ok := e.Report(res.ID)
		if !ok {
			t.Fatal("report missing")
		}
		blob, _ := json.Marshal(rep)
		if strings.Contains(string(blob), "SECRET_DT") || strings.Contains(string(blob), "SECRET_FW") {
			t.Fatalf("credentials leaked into report: %s", blob)
		}
	}
}

func TestDaytonaNewWithKeySkipsDriverGate(t *testing.T) {
	t.Setenv("REACTOR_DRIVER", "")
	t.Setenv("DAYTONA_API_KEY", "")
	d := daytona.NewWithKey("k", "")
	if !d.Available() {
		t.Fatal("NewWithKey driver should be available without REACTOR_DRIVER")
	}
	env := daytona.New()
	if env.Available() {
		t.Fatal("env driver must stay gated when REACTOR_DRIVER is unset")
	}
}
