package engine

import (
	"net/http"
	"os"
	"strings"

	"github.com/reactor-sec/reactor/internal/chamber"
	"github.com/reactor-sec/reactor/internal/chamber/daytona"
	"github.com/reactor-sec/reactor/internal/chamber/local"
	"github.com/reactor-sec/reactor/internal/oai"
)

// RunCredentials are optional visitor BYOK keys attached to a single
// detonation. They never land in DetonationReport, SSE events, or logs —
// only in the in-memory detonation handle for the duration of the run.
//
// Empty fields mean "use the engine host env" (DAYTONA_*/FIREWORKS_*).
type RunCredentials struct {
	DaytonaAPIKey   string `json:"daytona_api_key,omitempty"`
	DaytonaAPIURL   string `json:"daytona_api_url,omitempty"`
	FireworksAPIKey string `json:"fireworks_api_key,omitempty"`
}

// Empty reports whether the visitor sent nothing useful.
func (c RunCredentials) Empty() bool {
	return strings.TrimSpace(c.DaytonaAPIKey) == "" &&
		strings.TrimSpace(c.FireworksAPIKey) == ""
}

// normalize trims whitespace so a paste with trailing newline still works.
func (c RunCredentials) normalize() RunCredentials {
	return RunCredentials{
		DaytonaAPIKey:   strings.TrimSpace(c.DaytonaAPIKey),
		DaytonaAPIURL:   strings.TrimSpace(c.DaytonaAPIURL),
		FireworksAPIKey: strings.TrimSpace(c.FireworksAPIKey),
	}
}

// credentialsFromRequest merges the JSON body field with the X-Reactor-*
// header fallbacks (upload uses headers because multipart has no JSON body).
// Body wins when both are set.
func credentialsFromRequest(r *http.Request, body RunCredentials) RunCredentials {
	out := body.normalize()
	if out.DaytonaAPIKey == "" {
		out.DaytonaAPIKey = strings.TrimSpace(r.Header.Get("X-Reactor-Daytona-Key"))
	}
	if out.DaytonaAPIURL == "" {
		out.DaytonaAPIURL = strings.TrimSpace(r.Header.Get("X-Reactor-Daytona-Url"))
	}
	if out.FireworksAPIKey == "" {
		out.FireworksAPIKey = strings.TrimSpace(r.Header.Get("X-Reactor-Fireworks-Key"))
	}
	return out.normalize()
}

// driverFor picks the chamber driver for one detonation. A visitor Daytona key
// always provisions against their account (BYOK public demo). Otherwise the
// engine's configured preference order runs (Daytona-when-env, else local).
func (e *Engine) driverFor(creds RunCredentials) chamber.Driver {
	if creds.DaytonaAPIKey != "" {
		return daytona.NewWithKey(creds.DaytonaAPIKey, creds.DaytonaAPIURL)
	}
	for _, d := range e.drivers {
		if d.Available() {
			return d
		}
	}
	return local.New()
}

// fireworksModel picks the Fireworks model id for BYOK runs: host ANALYST_MODEL
// / VICTIM_MODEL / FIREWORKS_MODEL when set, else the project default.
func fireworksModel() string {
	for _, k := range []string{"ANALYST_MODEL", "VICTIM_MODEL", "FIREWORKS_MODEL"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return "accounts/fireworks/models/gpt-oss-120b"
}

// analystClient builds the hosted analyst client for this detonation. Visitor
// Fireworks key beats host env; REACTOR_ANALYST=deterministic still forces
// offline.
func analystClient(creds RunCredentials) (*oai.Client, bool) {
	if analystForced() {
		return nil, false
	}
	if creds.FireworksAPIKey != "" {
		return oai.NewFireworks(creds.FireworksAPIKey, fireworksModel()), true
	}
	return oai.FromEnv()
}
