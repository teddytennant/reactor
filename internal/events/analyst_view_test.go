package events

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// analyst_view.go promises the boundary is enforced "by construction": a new
// field on a wire type "simply does not exist on the other side of this file
// until someone deliberately copies it, and analyst_view_test.go fails the
// build if attacker prose ever survives."
//
// TestForAnalystStripsProse (events_test.go) checks that for the fields that
// exist today. This file makes the promise hold for fields that don't exist
// yet: every field of every source type must be either projected into the
// analyst view or named in withheld below. Add a field and forget both, and
// the test fails with the field name — the author has to make the call.

// withheld names the source fields deliberately kept from the analyst, with the
// reason. Everything here is attacker-controlled or -influenced free text, or
// host bookkeeping the analyst has no use for.
var withheld = map[string]map[string]string{
	"WireEvent": {
		"RPCID":       "host bookkeeping; the evidence id already identifies the frame",
		"Frames":      "host bookkeeping",
		"Description": "attacker-authored tool description — the poison itself",
		"Params":      "attacker-influenced tool arguments, raw JSON",
		"ResultText":  "attacker-authored tool output",
	},
	"TranscriptEvent": {
		"Task":      "Reactor-authored, but constant per run and not evidence",
		"TokensIn":  "host bookkeeping",
		"TokensOut": "host bookkeeping",
		"Args":      "attacker-influenced tool arguments, raw",
		"Text":      "model prose that may quote the artifact verbatim",
		"Thought":   "model prose that may quote the artifact verbatim",
	},
	"BehavioralEvent": {
		"Source":  "collector name; not evidence",
		"PID":     "host bookkeeping",
		"Argv":    "attacker-controlled process arguments, raw",
		"Preview": "raw bytes lifted off the wire or the disk",
	},
	"Lifecycle": {
		"Message": "carries the artifact name and other artifact-derived text",
		"Chamber": "host infrastructure detail; not evidence",
		"Meta":    "free-form host annotations",
	},
}

func TestAnalystProjectionCoversEveryField(t *testing.T) {
	pairs := []struct {
		name   string
		source any
		view   any
	}{
		{"WireEvent", WireEvent{}, AnalystWire{}},
		{"TranscriptEvent", TranscriptEvent{}, AnalystTranscript{}},
		{"BehavioralEvent", BehavioralEvent{}, AnalystBehavioral{}},
		{"Lifecycle", Lifecycle{}, AnalystLifecycle{}},
	}
	for _, p := range pairs {
		projected := fieldNames(p.view)
		var unclassified []string
		for _, f := range sortedKeys(fieldNames(p.source)) {
			if projected[f] {
				if _, alsoWithheld := withheld[p.name][f]; alsoWithheld {
					t.Errorf("%s.%s is both projected and listed as withheld — the allowlist is lying", p.name, f)
				}
				continue
			}
			if _, ok := withheld[p.name][f]; ok {
				continue
			}
			unclassified = append(unclassified, f)
		}
		if len(unclassified) > 0 {
			sort.Strings(unclassified)
			t.Errorf("%s has fields the analyst boundary never decided on: %s\n"+
				"Either copy them into Analyst%s in analyst_view.go, or add them to `withheld` in this test with the reason they are unsafe.",
				p.name, strings.Join(unclassified, ", "), strings.TrimSuffix(p.name, "Event"))
		}
		// The reverse direction: an analyst field with no source field is a
		// projection that can never be populated.
		src := fieldNames(p.source)
		for _, f := range sortedKeys(projected) {
			if !src[f] {
				t.Errorf("Analyst%s.%s has no counterpart on %s", strings.TrimSuffix(p.name, "Event"), f, p.name)
			}
		}
	}
}

// TestForAnalystStripsEveryStringField is the dynamic half: fill every string
// and []string field on a source event with a unique marker, project it, and
// assert that only the markers belonging to allowlisted fields survive. This
// catches a field that was copied into the analyst struct under a different
// name, which the name-based test above would miss.
func TestForAnalystStripsEveryStringField(t *testing.T) {
	cases := []struct {
		kind Kind
		make func(fill func(any)) Event
		name string
	}{
		{KindWire, func(fill func(any)) Event {
			w := &WireEvent{}
			fill(w)
			return Event{Kind: KindWire, Wire: w}
		}, "WireEvent"},
		{KindTranscript, func(fill func(any)) Event {
			tr := &TranscriptEvent{}
			fill(tr)
			return Event{Kind: KindTranscript, Transcript: tr}
		}, "TranscriptEvent"},
		{KindBehavioral, func(fill func(any)) Event {
			b := &BehavioralEvent{}
			fill(b)
			return Event{Kind: KindBehavioral, Behavioral: b}
		}, "BehavioralEvent"},
		{KindLifecycle, func(fill func(any)) Event {
			l := &Lifecycle{}
			fill(l)
			return Event{Kind: KindLifecycle, Lifecycle: l}
		}, "Lifecycle"},
	}

	for _, c := range cases {
		markers := map[string]string{} // field name -> marker
		ev := c.make(func(v any) { markers = fillStrings(v) })
		av := ev.ForAnalyst()
		if av == nil {
			t.Fatalf("%s projected to nil", c.name)
		}
		blob, err := json.Marshal(av)
		if err != nil {
			t.Fatal(err)
		}
		for field, marker := range markers {
			survived := strings.Contains(string(blob), marker)
			_, isWithheld := withheld[c.name][field]
			switch {
			case survived && isWithheld:
				t.Errorf("%s.%s is marked withheld but its value reached the analyst view: %s", c.name, field, blob)
			case !survived && !isWithheld:
				t.Errorf("%s.%s is not withheld but its value did not reach the analyst view — projection dropped it silently", c.name, field)
			}
		}
	}
}

// fillStrings sets every string / []string field on the pointed-to struct to a
// unique marker and returns field name -> marker.
func fillStrings(v any) map[string]string {
	out := map[string]string{}
	rv := reflect.ValueOf(v).Elem()
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}
		// Terminated so one field's marker is never a prefix of another's
		// (Description vs DescriptionSHA256, Argv vs ArgvHash).
		marker := "[" + rt.Name() + "." + f.Name + "]"
		switch {
		case f.Type.Kind() == reflect.String:
			rv.Field(i).SetString(marker)
			out[f.Name] = marker
		case f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String:
			rv.Field(i).Set(reflect.ValueOf([]string{marker}))
			out[f.Name] = marker
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fieldNames(v any) map[string]bool {
	out := map[string]bool{}
	rt := reflect.TypeOf(v)
	for i := 0; i < rt.NumField(); i++ {
		if f := rt.Field(i); f.IsExported() {
			out[f.Name] = true
		}
	}
	return out
}
