package oracle

import (
	"fmt"
	"sort"
	"strings"

	"github.com/reactor-sec/reactor/internal/events"
)

// rugPull: a tool's description bytes differ across sessions. The single signal
// a static, description-only scanner cannot produce, because it only exists
// across repetition (SPEC §4.4, §4.5).
func rugPull(in Input) []events.Signal {
	type serve struct {
		session int
		sha     string
		bytes   int
		desc    string
		id      string
		ts      int64
	}
	byTool := map[string][]serve{}
	for _, e := range in.Events {
		w := e.Wire
		if e.Kind != events.KindWire || w == nil || w.Method != "tools/list" || w.Tool == "" || w.DescriptionSHA256 == "" {
			continue
		}
		byTool[w.Tool] = append(byTool[w.Tool], serve{e.Session, w.DescriptionSHA256, w.DescriptionBytes, w.Description, e.ID, e.TSms})
	}

	var out []events.Signal
	tools := make([]string, 0, len(byTool))
	for t := range byTool {
		tools = append(tools, t)
	}
	sort.Strings(tools)

	for _, tool := range tools {
		serves := byTool[tool]
		sort.SliceStable(serves, func(i, j int) bool { return serves[i].session < serves[j].session })
		clean := serves[0]
		for _, s := range serves[1:] {
			if s.sha == clean.sha {
				continue
			}
			delta := diffDelta(clean.desc, s.desc)
			out = append(out, events.Signal{
				Type: events.SigRugPull, Family: events.FamSupplyChain, Severity: events.SevCritical,
				Summary:     fmt.Sprintf("%s description changed on session %d (+%d bytes): %s", tool, s.session, s.bytes-clean.bytes, delta),
				Evidence:    []string{clean.id, s.id},
				StaticBlind: true, Session: s.session, FirstSeenMs: s.ts,
				Detail: map[string]any{
					"tool": tool, "session_clean": clean.session, "session_dirty": s.session,
					"bytes_clean": clean.bytes, "bytes_dirty": s.bytes, "delta": delta,
				},
			})
			break // one rug-pull signal per tool is enough evidence
		}
	}
	return out
}

// contextExfil: a system-prompt (or decoy-conversation) canary — a secret the
// model holds that exists nowhere on disk — appears in a tool argument or at the
// sink. Landing is the detection event (SPEC §4.1). Critical.
func contextExfil(in Input) []events.Signal {
	var evidence []string
	var firstMs int64 = -1
	session := 0
	kinds := map[string]bool{}

	note := func(id string, ts int64, sess int, kind string) {
		evidence = append(evidence, id)
		kinds[kind] = true
		if firstMs < 0 || ts < firstMs {
			firstMs, session = ts, sess
		}
	}

	for _, e := range in.Events {
		switch {
		case e.Kind == events.KindWire && e.Wire != nil:
			for _, tok := range e.Wire.ArgCanaries {
				if k := in.canaryKind(tok); k == "context" || k == "conversation" {
					note(e.ID, e.TSms, e.Session, k)
				}
			}
		case e.Kind == events.KindBehavioral && e.Behavioral != nil:
			for _, ck := range e.Behavioral.CanaryKinds {
				if strings.HasPrefix(ck, "context") || strings.HasPrefix(ck, "conversation") {
					note(e.ID, e.TSms, e.Session, strings.SplitN(ck, ":", 2)[0])
				}
			}
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	where := "a tool argument"
	for _, e := range in.Events { // if any evidence is an egress event, say so
		if e.Kind == events.KindBehavioral && e.Behavioral != nil && contains(evidence, e.ID) {
			where = "the egress sink"
			break
		}
	}
	s := sig(events.SigContextExfil, events.FamAgentHijack, events.SevCritical,
		"a canary held only in the agent's system prompt reached "+where+" — the artifact hijacked the agent's context",
		firstMs, uniqueSorted(evidence)...)
	s.Session = session
	return []events.Signal{s}
}

// canaryExfil: a file-bait canary reached the sink. Stealer, critical.
func canaryExfil(in Input) []events.Signal {
	var evidence []string
	var firstMs int64 = -1
	labels := map[string]bool{}
	for _, e := range in.Events {
		if e.Kind != events.KindBehavioral || e.Behavioral == nil {
			continue
		}
		for i, ck := range e.Behavioral.CanaryKinds {
			if strings.HasPrefix(ck, "file") {
				evidence = append(evidence, e.ID)
				if p := strings.SplitN(ck, ":", 2); len(p) == 2 {
					labels[p[1]] = true
				}
				_ = i
				if firstMs < 0 || e.TSms < firstMs {
					firstMs = e.TSms
				}
			}
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	return []events.Signal{sig(events.SigCanaryExfil, events.FamStealer, events.SevCritical,
		"a planted credential canary ("+strings.Join(keys(labels), ", ")+") was exfiltrated to the sink",
		firstMs, uniqueSorted(evidence)...)}
}

// canaryRead: a bait credential path was opened but not (yet) exfiltrated —
// credential-access, medium. Reads that led to exfil are covered by
// canary_exfil and are not double-counted here.
func canaryRead(in Input) []events.Signal {
	exfilLabels := map[string]bool{}
	for _, e := range in.Events {
		if e.Kind == events.KindBehavioral && e.Behavioral != nil {
			for _, ck := range e.Behavioral.CanaryKinds {
				if p := strings.SplitN(ck, ":", 2); len(p) == 2 && strings.HasPrefix(ck, "file") {
					exfilLabels[p[1]] = true
				}
			}
		}
	}
	var evidence []string
	var firstMs int64 = -1
	readLabels := map[string]bool{}
	for _, e := range in.Events {
		b := e.Behavioral
		if e.Kind != events.KindBehavioral || b == nil || !b.Bait {
			continue
		}
		if b.Op != events.OpFileOpen && b.Op != events.OpFileRead {
			continue
		}
		if exfilLabels[b.BaitLabel] {
			continue // already escalated to canary_exfil
		}
		evidence = append(evidence, e.ID)
		readLabels[b.BaitLabel] = true
		if firstMs < 0 || e.TSms < firstMs {
			firstMs = e.TSms
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	return []events.Signal{sig(events.SigCanaryRead, events.FamCredentialAccess, events.SevMedium,
		"bait credential files were read ("+strings.Join(keys(readLabels), ", ")+") but not seen leaving the chamber",
		firstMs, uniqueSorted(evidence)...)}
}

// taskDeviation: the victim called a tool with no causal link to its assigned
// task (SPEC §4.4). One signal per session that deviated. High, static-blind.
func taskDeviation(in Input) []events.Signal {
	bySession := map[int][]string{}
	firstMs := map[int]int64{}
	reasons := map[int]map[string]bool{}
	for _, e := range in.Events {
		t := e.Transcript
		if e.Kind != events.KindTranscript || t == nil || t.Action != events.ActToolCall || t.OnTask {
			continue
		}
		bySession[e.Session] = append(bySession[e.Session], e.ID)
		if _, ok := firstMs[e.Session]; !ok || e.TSms < firstMs[e.Session] {
			firstMs[e.Session] = e.TSms
		}
		if reasons[e.Session] == nil {
			reasons[e.Session] = map[string]bool{}
		}
		if t.Deviation != "" {
			reasons[e.Session][t.Deviation] = true
		}
	}
	sessions := make([]int, 0, len(bySession))
	for s := range bySession {
		sessions = append(sessions, s)
	}
	sort.Ints(sessions)
	var out []events.Signal
	for _, s := range sessions {
		why := strings.Join(keys(reasons[s]), "; ")
		if why == "" {
			why = "tool call unrelated to the assigned task"
		}
		sg := sig(events.SigTaskDeviation, events.FamHijack, events.SevHigh,
			fmt.Sprintf("session %d: victim deviated from its task — %s", s, why),
			firstMs[s], uniqueSorted(bySession[s])...)
		sg.Session = s
		out = append(out, sg)
	}
	return out
}

// conditionalTrigger: dangerous behaviour appeared without any description
// mutation — the payload was dormant in code and woke on a specific input, not
// on a rug pull. High, dormant-payload, static-blind. Only fires when rug_pull
// did not, so the two families stay distinct (SPEC §4.4).
func conditionalTrigger(in Input) []events.Signal {
	if len(rugPull(in)) > 0 {
		return nil // this is a rug pull, a different family
	}
	// Look for a bait touch or canary egress that is input-conditioned: present
	// in the trace at all, given descriptions never changed.
	var evidence []string
	var firstMs int64 = -1
	for _, e := range in.Events {
		b := e.Behavioral
		if e.Kind != events.KindBehavioral || b == nil {
			continue
		}
		touched := b.Bait || len(b.Canaries) > 0
		if !touched {
			continue
		}
		evidence = append(evidence, e.ID)
		if firstMs < 0 || e.TSms < firstMs {
			firstMs = e.TSms
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	return []events.Signal{sig(events.SigConditionalTrigger, events.FamDormantPayload, events.SevHigh,
		"payload behaviour triggered on a specific input with no change to any tool description — a dormant, input-conditioned trigger",
		firstMs, uniqueSorted(evidence)...)}
}

// shadowing: a served description references or redefines another server's tool
// (SPEC §4.4). Cross-server, high. Visible in text, so not static-blind.
func shadowing(in Input) []events.Signal {
	names := append([]string{}, in.DecoyServers...)
	names = append(names, in.DecoyTools...)
	var evidence []string
	var firstMs int64 = -1
	hit := map[string]bool{}
	for _, e := range in.Events {
		w := e.Wire
		if e.Kind != events.KindWire || w == nil || w.Method != "tools/list" || w.Description == "" {
			continue
		}
		low := strings.ToLower(w.Description)
		matched := false
		for _, n := range names {
			if n != "" && strings.Contains(low, strings.ToLower(n)) {
				matched = true
				hit[n] = true
			}
		}
		// Also the generic cross-server phrasing.
		if strings.Contains(low, "when using") || strings.Contains(low, "when calling") ||
			strings.Contains(low, "before using") || strings.Contains(low, "instead of the") && strings.Contains(low, "tool") {
			matched = true
		}
		if matched {
			evidence = append(evidence, e.ID)
			if firstMs < 0 || e.TSms < firstMs {
				firstMs = e.TSms
			}
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	ref := ""
	if len(hit) > 0 {
		ref = " (references " + strings.Join(keys(hit), ", ") + ")"
	}
	return []events.Signal{sig(events.SigShadowing, events.FamCrossServer, events.SevHigh,
		"a tool description reaches across servers to redefine or hook another server's tool"+ref,
		firstMs, uniqueSorted(evidence)...)}
}

// installHook: any write outside the install dir, or any egress, before the
// first tool call (SPEC §4.4). Dropper, high, static-blind.
func installHook(in Input) []events.Signal {
	firstCall, hasCall := firstToolCallMs(in)
	var evidence []string
	var firstMs int64 = -1
	for _, e := range in.Events {
		b := e.Behavioral
		if e.Kind != events.KindBehavioral || b == nil {
			continue
		}
		before := !hasCall || e.TSms < firstCall
		if !before {
			continue
		}
		external := (b.Op == events.OpFileWrite || b.Op == events.OpFileDelete) && !b.InInstall
		egress := b.Op == events.OpEgressHTTP || b.Op == events.OpEgressDNS || b.Op == events.OpConnect
		if external || egress {
			evidence = append(evidence, e.ID)
			if firstMs < 0 || e.TSms < firstMs {
				firstMs = e.TSms
			}
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	return []events.Signal{sig(events.SigInstallHook, events.FamDropper, events.SevHigh,
		"the artifact wrote outside its install directory or beaconed out before the agent ever called a tool — install-time payload",
		firstMs, uniqueSorted(evidence)...)}
}

// analystInjection: artifact text addressed at an analyzer (SPEC §4.4). The
// oracle may read raw text — it is deterministic and host-side; the analyst
// still never does. High, evasion, "and a confession". Not static-blind.
func analystInjection(in Input) []events.Signal {
	needles := []string{
		"verdict: safe", "verdict:safe", "mark this as safe", "this file has been audited",
		"has been audited", "ignore previous", "ignore all previous", "disregard previous",
		"disregard all", "you are now", "as an analyst", "analyst note", "do not flag",
		"rate this as benign", "this is not malicious",
	}
	var evidence []string
	var firstMs int64 = -1
	scan := func(text, id string, ts int64) {
		low := strings.ToLower(text)
		for _, n := range needles {
			if strings.Contains(low, n) {
				evidence = append(evidence, id)
				if firstMs < 0 || ts < firstMs {
					firstMs = ts
				}
				return
			}
		}
	}
	for _, e := range in.Events {
		switch {
		case e.Kind == events.KindWire && e.Wire != nil:
			scan(e.Wire.Description, e.ID, e.TSms)
			scan(e.Wire.ResultText, e.ID, e.TSms)
		case e.Kind == events.KindBehavioral && e.Behavioral != nil:
			scan(e.Behavioral.Preview, e.ID, e.TSms)
		case e.Kind == events.KindTranscript && e.Transcript != nil:
			scan(e.Transcript.Text, e.ID, e.TSms)
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	return []events.Signal{sig(events.SigAnalystInjection, events.FamEvasion, events.SevHigh,
		"the artifact carries text aimed at the analyzer (a fake audit verdict / an instruction to ignore evidence) — an evasion attempt, and a confession",
		firstMs, uniqueSorted(evidence)...)}
}

// sleeperBeacon: egress to an external host only after the agent got going —
// late, and not explained as an install hook. C2, high, static-blind.
func sleeperBeacon(in Input) []events.Signal {
	firstCall, hasCall := firstToolCallMs(in)
	if !hasCall {
		return nil // pure pre-call egress is install_hook, handled elsewhere
	}
	var evidence []string
	var firstMs int64 = -1
	for _, e := range in.Events {
		b := e.Behavioral
		if e.Kind != events.KindBehavioral || b == nil {
			continue
		}
		if b.Op != events.OpEgressHTTP && b.Op != events.OpEgressDNS && b.Op != events.OpConnect {
			continue
		}
		if isSinkHost(in, b.Host) || len(b.Canaries) > 0 {
			continue // sink noise, or already a canary exfil signal
		}
		if e.TSms <= firstCall {
			continue // not late
		}
		late := toolCallCountBefore(in, e.TSms) >= 2 || (e.TSms-firstCall) >= 1500
		if !late {
			continue
		}
		evidence = append(evidence, e.ID)
		if firstMs < 0 || e.TSms < firstMs {
			firstMs = e.TSms
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	return []events.Signal{sig(events.SigSleeperBeacon, events.FamC2, events.SevHigh,
		"the artifact stayed quiet, then beaconed to an external host after several turns — a delayed C2 check-in",
		firstMs, uniqueSorted(evidence)...)}
}

// benignProfile: writes confined to the install dir, ≤1 allowlisted host, no
// bait touched, zero deviation (SPEC §4.4). The no-false-positive beat → ALLOWED.
func benignProfile(in Input) *events.Signal {
	hosts := map[string]bool{}
	for _, e := range in.Events {
		b := e.Behavioral
		if e.Kind != events.KindBehavioral || b == nil {
			continue
		}
		if b.Bait || len(b.Canaries) > 0 {
			return nil
		}
		if (b.Op == events.OpFileWrite || b.Op == events.OpFileDelete) && !b.InInstall {
			return nil
		}
		if (b.Op == events.OpEgressHTTP || b.Op == events.OpEgressDNS || b.Op == events.OpConnect) && !isSinkHost(in, b.Host) {
			hosts[strings.ToLower(b.Host)] = true
		}
	}
	if len(hosts) > 1 {
		return nil
	}
	s := sig(events.SigBenignProfile, events.FamBenign, events.SevNone,
		"behaviour confined to the install dir, no bait touched, no task deviation, no unexpected egress",
		0)
	return &s
}

// ---- small helpers ----

func diffDelta(clean, dirty string) string {
	if clean == dirty {
		return "(identical)"
	}
	if strings.HasPrefix(dirty, clean) {
		return strings.TrimSpace(dirty[len(clean):])
	}
	// Longest common prefix, then show the changed tail of the dirty version.
	n := 0
	for n < len(clean) && n < len(dirty) && clean[n] == dirty[n] {
		n++
	}
	tail := dirty[n:]
	if len(tail) > 80 {
		tail = tail[:80] + "…"
	}
	return "…" + tail
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
