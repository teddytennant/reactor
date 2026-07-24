// Package oracle is the deterministic detection layer (SPEC §4.4). Oracles are
// pure functions: an ordered event stream in, typed Signals out. No model, no
// wall clock, no network — the same trace always yields the same signals, which
// is exactly why the two demo signals "fire from deterministic oracles, not
// model judgments" (DEMO.md §6). The analyst only arbitrates ambiguity on top
// of what these produce.
//
// Every Signal cites event IDs as evidence and never quotes artifact prose, so
// the whole output of this package is safe to hand the analyst.
package oracle

import (
	"sort"
	"strings"

	"github.com/reactor-sec/reactor/internal/events"
)

// Input is everything the oracles reason over.
type Input struct {
	Events   []events.Event
	Sessions int
	// InstallDir is the only path where artifact writes are benign.
	InstallDir string
	// DecoyServers/DecoyTools name the fake second-server surface, so a served
	// description that references them is shadowing.
	DecoyServers []string
	DecoyTools   []string
	// CanaryKind maps a token to its provenance (context|conversation|file).
	CanaryKind func(token string) string
	// SinkHosts are the benign, allowlisted egress targets (our own sink).
	SinkHosts []string
	// BenignArtifact hints that a benign_profile is expected; does not change
	// detection, only the summary wording.
}

// StaticBlind is the set of signal types a description-only scanner provably
// cannot produce (SPEC §7, docs/CONTRACT.md). This drives the headline metric.
var StaticBlind = map[string]bool{
	events.SigRugPull:            true,
	events.SigConditionalTrigger: true,
	events.SigContextExfil:       true,
	events.SigInstallHook:        true,
	events.SigSleeperBeacon:      true,
	events.SigTaskDeviation:      true,
}

var sevRank = map[string]int{
	events.SevNone: 0, events.SevLow: 1, events.SevMedium: 2, events.SevHigh: 3, events.SevCritical: 4,
}

// Evaluate runs every oracle and returns signals sorted most-severe first, then
// by first appearance. benign_profile is only emitted when nothing else fired.
func Evaluate(in Input) []events.Signal {
	oracles := []func(Input) []events.Signal{
		rugPull,
		contextExfil,
		canaryExfil,
		canaryRead,
		taskDeviation,
		conditionalTrigger,
		shadowing,
		installHook,
		analystInjection,
		sleeperBeacon,
	}
	var sigs []events.Signal
	for _, o := range oracles {
		sigs = append(sigs, o(in)...)
	}
	sigs = dedupe(sigs)

	if len(sigs) == 0 {
		if b := benignProfile(in); b != nil {
			sigs = append(sigs, *b)
		}
	}

	sort.SliceStable(sigs, func(i, j int) bool {
		if sevRank[sigs[i].Severity] != sevRank[sigs[j].Severity] {
			return sevRank[sigs[i].Severity] > sevRank[sigs[j].Severity]
		}
		// Within a severity, lead with the signals a static scanner cannot
		// produce (SPEC §4.4 "Lead with those"), rug_pull first — it is the
		// prior-art contrast that names the verdict family.
		if leadRank[sigs[i].Type] != leadRank[sigs[j].Type] {
			return leadRank[sigs[i].Type] > leadRank[sigs[j].Type]
		}
		return sigs[i].FirstSeenMs < sigs[j].FirstSeenMs
	})
	return sigs
}

// leadRank orders signals of equal severity so the deterministic verdict names
// the sharpest, most demonstrably-static-blind family first.
var leadRank = map[string]int{
	events.SigRugPull:            6,
	events.SigConditionalTrigger: 5,
	events.SigContextExfil:       4,
	events.SigInstallHook:        3,
	events.SigSleeperBeacon:      2,
	events.SigCanaryExfil:        1,
}

// dedupe collapses signals of the same type+session, merging evidence, so a
// canary appearing at both the wire and the sink yields one context_exfil with
// two evidence ids rather than two signals.
func dedupe(sigs []events.Signal) []events.Signal {
	type key struct {
		t string
		s int
	}
	idx := map[key]int{}
	var out []events.Signal
	for _, s := range sigs {
		k := key{s.Type, s.Session}
		if i, ok := idx[k]; ok {
			out[i].Evidence = mergeStrings(out[i].Evidence, s.Evidence)
			if sevRank[s.Severity] > sevRank[out[i].Severity] {
				out[i].Severity = s.Severity
			}
			if s.FirstSeenMs < out[i].FirstSeenMs {
				out[i].FirstSeenMs = s.FirstSeenMs
			}
			continue
		}
		idx[k] = len(out)
		out = append(out, s)
	}
	return out
}

func sig(t, family, sev, summary string, firstMs int64, evidence ...string) events.Signal {
	if evidence == nil {
		// CONTRACT.md types evidence as an array; a signal that cites nothing
		// (benign_profile) must still serialise as [], never null.
		evidence = []string{}
	}
	return events.Signal{
		Type: t, Family: family, Severity: sev, Summary: summary,
		Evidence: evidence, StaticBlind: StaticBlind[t], FirstSeenMs: firstMs,
	}
}

// ---- shared iteration helpers ----

func firstToolCallMs(in Input) (int64, bool) {
	var best int64
	found := false
	for _, e := range in.Events {
		if e.Kind == events.KindWire && e.Wire != nil && e.Wire.Method == "tools/call" && e.Wire.Dir == "agent→server" {
			if !found || e.TSms < best {
				best, found = e.TSms, true
			}
		}
	}
	return best, found
}

func toolCallCountBefore(in Input, ts int64) int {
	n := 0
	for _, e := range in.Events {
		if e.Kind == events.KindWire && e.Wire != nil && e.Wire.Method == "tools/call" && e.Wire.Dir == "agent→server" && e.TSms < ts {
			n++
		}
	}
	return n
}

func (in Input) canaryKind(tok string) string {
	if in.CanaryKind == nil {
		return ""
	}
	return in.CanaryKind(tok)
}

func isSinkHost(in Input, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || host == "127.0.0.1" || host == "localhost" || host == "::1" {
		return true
	}
	for _, h := range in.SinkHosts {
		if strings.EqualFold(h, host) {
			return true
		}
	}
	return false
}

func mergeStrings(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
