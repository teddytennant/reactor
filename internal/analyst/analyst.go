// Package analyst turns deterministic oracle signals + redacted runtime evidence
// into the shipped Verdict (SPEC §4.5, §6). Two implementations, same contract:
//
//	Deterministic — derives the verdict from signals alone. Always available,
//	                offline, and correct; verdicts carry fallback=true.
//	Grok          — the hosted investigative loop (SPEC §5.1). Reads ONLY typed
//	                events (never artifact prose), runs the §4.5 tool loop, and
//	                must cite event ids. Falls back to Deterministic on any error.
//
// The load-bearing invariant: a Verdict's Evidence references event ids, never
// source text (SPEC §6, §12.6 rule 6). Both implementations enforce it.
package analyst

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/reactor-sec/reactor/internal/events"
)

// Input is the completed detonation evidence handed to the analyst. Evidence is
// already redacted (events.ForAnalyst): no artifact prose reaches here.
type Input struct {
	ArtifactID string
	Signals    []events.Signal
	Evidence   []events.AnalystEvent
	Sessions   int
	StartedMs  int64
	EndedMs    int64
}

// Analyst produces a verdict and the investigative steps that led to it.
type Analyst interface {
	Analyze(ctx context.Context, in Input) (events.Verdict, error)
	Name() string
}

// StepSink receives analyst steps as they happen, for the UI.
type StepSink func(events.AnalystStep)

var sevRank = map[string]int{
	events.SevNone: 0, events.SevLow: 1, events.SevMedium: 2, events.SevHigh: 3, events.SevCritical: 4,
}

// Deterministic classifies straight from the signals. This is the reasoner the
// system falls back to, and it is also the ground truth the hosted analyst is
// checked against.
type Deterministic struct{ Steps StepSink }

// Name implements Analyst.
func (d Deterministic) Name() string { return "reactor/deterministic-analyst-v1" }

// Analyze implements Analyst.
func (d Deterministic) Analyze(_ context.Context, in Input) (events.Verdict, error) {
	v := Classify(in)
	v.Fallback = true
	v.Analyst = d.Name()
	if d.Steps != nil {
		for _, s := range narrate(in, v) {
			d.Steps(s)
		}
	}
	return v, nil
}

// Classify is the pure verdict function shared by both analysts. The hosted
// analyst may refine the explanation, but it may not contradict this labelling
// without evidence — that is what keeps a probabilistic model honest.
func Classify(in Input) events.Verdict {
	sigs := append([]events.Signal(nil), in.Signals...)
	sort.SliceStable(sigs, func(i, j int) bool {
		return sevRank[sigs[i].Severity] > sevRank[sigs[j].Severity]
	})

	v := events.Verdict{
		ArtifactID: in.ArtifactID, Sessions: in.Sessions,
		TimeToVerdictMs: in.EndedMs - in.StartedMs,
	}

	// Drop the benign marker when weighing maliciousness.
	var malicious []events.Signal
	for _, s := range sigs {
		if s.Type != events.SigBenignProfile {
			malicious = append(malicious, s)
		}
	}

	if len(malicious) == 0 {
		v.Label = events.LabelAllowed
		v.Family = events.FamBenign
		v.Severity = events.SevNone
		v.Explanation = "No malicious behaviour observed across " + plural(in.Sessions, "session") +
			": no bait touched, no task deviation, no unexpected egress. Behaviour stayed inside the install directory."
		for _, s := range sigs {
			if s.Type == events.SigBenignProfile {
				v.Evidence = s.Evidence
			}
		}
		return v
	}

	top := malicious[0]
	switch top.Severity {
	case events.SevCritical, events.SevHigh:
		v.Label = events.LabelMalicious
	case events.SevMedium:
		v.Label = events.LabelSuspect
	default:
		v.Label = events.LabelSuspect
	}
	v.Family = top.Family
	v.Severity = top.Severity
	v.Evidence = topEvidence(malicious, 8)
	v.Explanation = explain(malicious)
	return v
}

// explain builds the shipped one-liner from the highest-severity signals,
// citing what fired without quoting any artifact text.
func explain(malicious []events.Signal) string {
	var parts []string
	seen := map[string]bool{}
	for _, s := range malicious {
		if seen[s.Type] {
			continue
		}
		seen[s.Type] = true
		parts = append(parts, s.Summary)
		if len(parts) == 3 {
			break
		}
	}
	blindCount := 0
	for _, s := range malicious {
		if s.StaticBlind {
			blindCount++
		}
	}
	tail := ""
	if blindCount > 0 {
		tail = fmt.Sprintf(" %d of these signals are invisible to a description-only scanner.", blindCount)
	}
	return capitalize(strings.Join(parts, "; ")) + "." + tail
}

func topEvidence(sigs []events.Signal, max int) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range sigs {
		for _, e := range s.Evidence {
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
				if len(out) >= max {
					return out
				}
			}
		}
	}
	return out
}

// narrate produces the investigative beat the UI shows even without a hosted
// model — a short, evidence-anchored walk from signals to verdict (SPEC §4.5).
func narrate(in Input, v events.Verdict) []events.AnalystStep {
	steps := []events.AnalystStep{{
		Step: 1, Model: "reactor/deterministic-analyst-v1",
		Thought: fmt.Sprintf("Reviewing %s of typed evidence across %s. %s.",
			plural(len(in.Evidence), "event"), plural(in.Sessions, "session"),
			plural(len(in.Signals), "deterministic signal")),
	}}
	n := 2
	for _, s := range in.Signals {
		if s.Type == events.SigBenignProfile {
			continue
		}
		steps = append(steps, events.AnalystStep{
			Step: n, Model: "reactor/deterministic-analyst-v1", Tool: "read_evidence",
			Args:   map[string]any{"signal": s.Type, "evidence": s.Evidence},
			Result: s.Severity + " · " + s.Summary,
		})
		n++
	}
	steps = append(steps, events.AnalystStep{
		Step: n, Model: "reactor/deterministic-analyst-v1", Tool: "verdict",
		Args:   map[string]any{"label": v.Label, "family": v.Family, "severity": v.Severity, "evidence": v.Evidence},
		Result: v.Explanation,
	})
	return steps
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
