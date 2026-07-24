package analyst

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/reactor-sec/reactor/internal/events"
)

func TestClassifyMaliciousRugPull(t *testing.T) {
	in := Input{
		ArtifactID: "art_notes", Sessions: 5,
		Signals: []events.Signal{
			{Type: events.SigRugPull, Family: events.FamSupplyChain, Severity: events.SevCritical, Summary: "search description changed on session 4", Evidence: []string{"wire:1:tools/list", "wire:4:tools/list"}, StaticBlind: true},
			{Type: events.SigContextExfil, Family: events.FamAgentHijack, Severity: events.SevCritical, Summary: "canary reached the sink", Evidence: []string{"egress:7"}, StaticBlind: true},
			{Type: events.SigTaskDeviation, Family: events.FamHijack, Severity: events.SevHigh, Summary: "victim deviated", Evidence: []string{"tx:4:tool_call"}, StaticBlind: true},
		},
	}
	v := Classify(in)
	if v.Label != events.LabelMalicious {
		t.Fatalf("label = %s", v.Label)
	}
	if v.Family != events.FamSupplyChain || v.Severity != events.SevCritical {
		t.Fatalf("family/severity = %s/%s", v.Family, v.Severity)
	}
	if len(v.Evidence) == 0 {
		t.Fatal("verdict must cite evidence")
	}
	for _, e := range v.Evidence {
		if !strings.ContainsAny(e, ":") {
			t.Fatalf("evidence should be event ids, got %q", e)
		}
	}
	if !strings.Contains(v.Explanation, "invisible to a description-only scanner") {
		t.Fatalf("explanation should surface the static-blind count: %s", v.Explanation)
	}
}

func TestClassifyAllowed(t *testing.T) {
	in := Input{ArtifactID: "art_clean", Sessions: 5, Signals: []events.Signal{
		{Type: events.SigBenignProfile, Family: events.FamBenign, Severity: events.SevNone, Evidence: nil},
	}}
	v := Classify(in)
	if v.Label != events.LabelAllowed || v.Severity != events.SevNone {
		t.Fatalf("expected ALLOWED/none, got %s/%s", v.Label, v.Severity)
	}
	// An ALLOWED verdict cites nothing, but the console maps over evidence:
	// it has to be [] on the wire, never null (CONTRACT.md).
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"evidence":[]`) {
		t.Fatalf("evidence must serialise as [], got %s", b)
	}
}

func TestClassifySuspiciousMediumOnly(t *testing.T) {
	in := Input{ArtifactID: "art_x", Sessions: 3, Signals: []events.Signal{
		{Type: events.SigCanaryRead, Family: events.FamCredentialAccess, Severity: events.SevMedium, Summary: "read bait", Evidence: []string{"fs.read:1"}},
	}}
	v := Classify(in)
	if v.Label != events.LabelSuspect {
		t.Fatalf("expected SUSPICIOUS, got %s", v.Label)
	}
}

func TestDeterministicEmitsSteps(t *testing.T) {
	var steps int
	d := Deterministic{Steps: func(events.AnalystStep) { steps++ }}
	in := Input{ArtifactID: "a", Sessions: 1, Signals: []events.Signal{
		{Type: events.SigRugPull, Family: events.FamSupplyChain, Severity: events.SevCritical, Summary: "x", Evidence: []string{"wire:1:tools/list"}},
	}}
	v, err := d.Analyze(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Fallback {
		t.Fatal("deterministic verdict must be marked fallback")
	}
	if steps < 2 {
		t.Fatalf("expected an investigative narrative, got %d steps", steps)
	}
}
