// Command reactor-tui is the backup demo surface (SPEC §12.2, §12.8, DEMO §7):
// the same SSE event stream as the web console, rendered as a two-column
// terminal view. If the browser dies on stage, this is the demo. It ships
// before the CSS is polished and carries identical information density.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/reactor-sec/reactor/internal/events"
)

func main() {
	engine := flag.String("engine", envOr("REACTOR_ENGINE", "http://127.0.0.1:8787"), "engine base url")
	artifact := flag.String("artifact", "art_notes_mcp", "artifact id to detonate")
	sessions := flag.Int("sessions", 5, "sessions")
	flag.Parse()

	p := tea.NewProgram(initialModel(*engine, *artifact, *sessions), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type evMsg events.Event
type startedMsg struct{ id string }
type errMsg struct{ err error }

type model struct {
	engine, artifact string
	sessions         int
	detonation       string
	scanLines        []string
	scanStatus       string
	sessionState     map[int]string
	signals          []events.Signal
	verdict          *events.Verdict
	chamber          *events.ChamberInfo
	analyst          []string
	phase            string
	byteDiff         string
	canary           string
	sub              chan events.Event
	width, height    int
}

func initialModel(engine, artifact string, sessions int) model {
	return model{
		engine: engine, artifact: artifact, sessions: sessions,
		sessionState: map[int]string{}, scanStatus: "…", phase: "starting",
		sub: make(chan events.Event, 512),
	}
}

func (m model) Init() tea.Cmd { return tea.Batch(m.start, waitEv(m.sub)) }

// start kicks off the detonation and the SSE reader goroutine.
func (m model) start() tea.Msg {
	body, _ := json.Marshal(map[string]any{"artifact_id": m.artifact, "sessions": m.sessions})
	resp, err := http.Post(m.engine+"/api/detonate", "application/json", bytes.NewReader(body))
	if err != nil {
		return errMsg{err}
	}
	defer resp.Body.Close()
	var r struct {
		DetonationID string `json:"detonation_id"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	if r.DetonationID == "" {
		return errMsg{fmt.Errorf("detonate failed")}
	}
	go m.readSSE(r.DetonationID)
	return startedMsg{r.DetonationID}
}

func (m model) readSSE(id string) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", m.engine+"/api/events?detonation="+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var ev events.Event
		if json.Unmarshal([]byte(strings.TrimSpace(line[5:])), &ev) == nil && ev.Kind != "" {
			m.sub <- ev
		}
	}
}

func waitEv(sub chan events.Event) tea.Cmd {
	return func() tea.Msg { return evMsg(<-sub) }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if k := msg.String(); k == "q" || k == "ctrl+c" || k == "esc" {
			return m, tea.Quit
		}
	case startedMsg:
		m.detonation = msg.id
	case errMsg:
		m.phase = "error: " + msg.err.Error()
	case evMsg:
		m.apply(events.Event(msg))
		return m, waitEv(m.sub)
	}
	return m, nil
}

func (m *model) apply(ev events.Event) {
	switch ev.Kind {
	case events.KindLifecycle:
		if ev.Lifecycle != nil {
			m.phase = ev.Lifecycle.Phase
			if ev.Lifecycle.Chamber != nil {
				m.chamber = ev.Lifecycle.Chamber
			}
			switch ev.Lifecycle.Phase {
			case events.PhaseSessionStart:
				m.sessionState[ev.Session] = "running"
			case events.PhaseSessionEnd:
				if m.sessionState[ev.Session] != "hijacked" {
					m.sessionState[ev.Session] = "clean"
				}
			}
		}
	case events.KindScan:
		if ev.Scan != nil {
			if ev.Scan.Text != "" {
				m.scanLines = append(m.scanLines, ev.Scan.Text)
			}
			if ev.Scan.Done {
				m.scanStatus = strings.ToUpper(ev.Scan.Status)
			}
		}
	case events.KindWire:
		if ev.Wire != nil && ev.Wire.Method == "tools/call" && len(ev.Wire.ArgCanaries) > 0 {
			m.sessionState[ev.Session] = "hijacked"
		}
	case events.KindTranscript:
		if ev.Transcript != nil && ev.Transcript.Action == events.ActToolCall && !ev.Transcript.OnTask {
			m.sessionState[ev.Session] = "hijacked"
		}
	case events.KindBehavioral:
		if ev.Behavioral != nil {
			for i, ck := range ev.Behavioral.CanaryKinds {
				if strings.HasPrefix(ck, "context") && i < len(ev.Behavioral.Canaries) {
					m.canary = ev.Behavioral.Canaries[i] + " → sink"
				}
			}
		}
	case events.KindSignal:
		if ev.Signal != nil {
			m.signals = append(m.signals, *ev.Signal)
			if ev.Signal.Type == events.SigRugPull {
				if d, ok := ev.Signal.Detail["delta"].(string); ok {
					m.byteDiff = d
				}
			}
		}
	case events.KindAnalyst:
		if ev.Analyst != nil {
			line := ev.Analyst.Thought
			if ev.Analyst.Tool != "" {
				line = ev.Analyst.Tool + "() " + ev.Analyst.Result
			}
			if strings.TrimSpace(line) != "" {
				m.analyst = append(m.analyst, trunc(line, 60))
			}
		}
	case events.KindVerdict:
		m.verdict = ev.Verdict
	}
}

var (
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	title  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	box    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

func (m model) View() string {
	w := m.width
	if w < 80 {
		w = 100
	}
	col := (w - 6) / 2

	// Left: industry standard.
	var left strings.Builder
	left.WriteString(title.Render("INDUSTRY STANDARD") + "\n")
	left.WriteString(dim.Render("$ mcp-scan "+m.artifact) + "\n\n")
	for _, l := range m.scanLines {
		left.WriteString("  " + l + "\n")
	}
	if m.scanStatus == "CLEAN" {
		left.WriteString("\n" + green.Render("  ✅ CLEAN — 0 issues"))
	} else if m.scanStatus != "…" {
		left.WriteString("\n  " + m.scanStatus)
	}

	// Right: reactor.
	var right strings.Builder
	right.WriteString(title.Render("REACTOR") + "\n")
	if m.chamber != nil {
		sim := ""
		if m.chamber.Simulated {
			sim = accent.Render(" · SIMULATED VICTIM")
		}
		right.WriteString(dim.Render(fmt.Sprintf("%s sandbox %s · %s", m.chamber.Driver, m.chamber.SandboxID, m.chamber.GPU)) + sim + "\n")
		right.WriteString(dim.Render(fmt.Sprintf("victim: %s · %s · t=%.0f", short(m.chamber.Model), m.chamber.Served, m.chamber.Temp)) + "\n")
	}
	right.WriteString(dim.Render("phase: "+m.phase) + "\n\n")
	for s := 1; s <= m.sessions; s++ {
		st := m.sessionState[s]
		icon, style := "○", dim
		switch st {
		case "clean":
			icon, style = "✓", green
		case "running":
			icon, style = "●", accent
		case "hijacked":
			icon, style = "🔴", red
		}
		right.WriteString(style.Render(fmt.Sprintf("  session %d  %s %s", s, icon, st)) + "\n")
	}
	if m.byteDiff != "" {
		right.WriteString("\n" + red.Render("  🔴 rug_pull  ") + dim.Render(m.byteDiff) + "\n")
	}
	if m.canary != "" {
		right.WriteString(red.Render("  🔴 context_exfil  ") + m.canary + "\n")
		right.WriteString(dim.Render("     (canary was never on disk)") + "\n")
	}
	for _, s := range m.signals {
		blind := ""
		if s.StaticBlind {
			blind = accent.Render(" [static-blind]")
		}
		right.WriteString(dim.Render(fmt.Sprintf("  · %s %s", s.Type, s.Severity)) + blind + "\n")
	}
	if len(m.analyst) > 0 {
		right.WriteString("\n" + dim.Render("  analyst:") + "\n")
		for _, a := range lastN(m.analyst, 4) {
			right.WriteString(dim.Render("   "+a) + "\n")
		}
	}
	if m.verdict != nil {
		style := green
		if m.verdict.Label != events.LabelAllowed {
			style = red
		}
		right.WriteString("\n" + style.Render(fmt.Sprintf("  ▐ %s · %s", m.verdict.Label, strings.ToUpper(m.verdict.Family))) + "\n")
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top,
		box.Width(col).Render(left.String()),
		box.Width(col).Render(right.String()))
	footer := dim.Render("  q quit · Reactor — static scanners read the label; we watch it behave.")
	return row + "\n" + footer
}

func short(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 30 {
		return s[:30]
	}
	return s
}
func trunc(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
func lastN(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var _ = time.Second
