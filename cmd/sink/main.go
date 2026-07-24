// Command sink is the contained egress sink (SPEC §4.3 "Contained egress").
//
// It is the operational sink for the local chamber driver: an HTTP forward-proxy
// + raw sink + mock DNS that nothing egresses past. Every artifact process is
// launched with HTTP(S)_PROXY / ALL_PROXY pointed here, so any beacon, C2 call
// or credential POST lands in this log instead of on the internet. When a
// planted canary appears in a request body, path or headers, the sink emits a
// typed BehavioralEvent naming which canary and of what kind — that is the
// "REACTOR-a1b2 hit the sink" gasp, made into evidence.
//
// The Rust crate crates/reactor-sink is the identical sink baked into the
// chamber image; this Go build guarantees the GPU-free demo path runs anywhere.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/reactor-sec/reactor/internal/canary"
	"github.com/reactor-sec/reactor/internal/events"
)

func main() {
	httpAddr := flag.String("http", envOr("REACTOR_SINK_HTTP_ADDR", "127.0.0.1:9931"), "http sink + forward proxy listen addr")
	dnsAddr := flag.String("dns", envOr("REACTOR_SINK_DNS_ADDR", "127.0.0.1:9953"), "mock dns listen addr (udp); empty to disable")
	logDir := flag.String("log-dir", envOr("REACTOR_LOG_DIR", "."), "directory for sink.jsonl")
	canaryFile := flag.String("canaries", os.Getenv("REACTOR_CANARY_FILE"), "canary table json")
	flag.Parse()

	cset, err := canary.Load(*canaryFile)
	if err != nil {
		log.Fatalf("load canaries: %v", err)
	}

	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		log.Fatalf("mkdir log dir: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(*logDir, "sink.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatalf("open sink log: %v", err)
	}
	defer f.Close()
	w := &jsonlWriter{f: f}

	if *dnsAddr != "" {
		go serveDNS(*dnsAddr, w, cset)
	}

	s := &sink{w: w, canaries: cset}
	srv := &http.Server{Addr: *httpAddr, Handler: s, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("reactor-sink http=%s dns=%s canaries=%d — nothing egresses past here", *httpAddr, *dnsAddr, cset.Len())
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

type sink struct {
	w        *jsonlWriter
	canaries *canary.Set
}

// ServeHTTP handles three shapes: a plain sink POST (something posted straight
// to us), a forward-proxy absolute-URI request (HTTP_PROXY), and CONNECT (HTTPS
// tunnels — we log the host and refuse the tunnel, so TLS beacons are recorded
// even though we can't read their bodies).
func (s *sink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		s.logConnect(r)
		// Refuse the tunnel: contained egress means nothing actually connects.
		http.Error(w, "reactor-sink: egress contained", http.StatusForbidden)
		return
	}

	body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	host := r.Host
	urlPath := r.URL.Path
	if r.URL.IsAbs() { // forward-proxy request
		host = r.URL.Host
		urlPath = r.URL.Path
	}

	// Canary matching runs over the full request surface, not just the body.
	surface := string(body) + "\n" + r.URL.String() + "\n" + headerString(r.Header)
	toks, kinds := s.canaries.Match(surface)

	be := &events.BehavioralEvent{
		Op:         events.OpEgressHTTP,
		Source:     "sink",
		Host:       hostOnly(host),
		Port:       portOf(host, 80),
		Method:     r.Method,
		URLPath:    urlPath,
		BodyBytes:  len(body),
		BodySHA256: sha(body),
		Preview:    preview(body, 240),
	}
	if len(toks) > 0 {
		be.Canaries = toks
		be.CanaryKinds = kinds
	}
	s.w.write(events.Event{Kind: events.KindBehavioral, TSms: nowMs(), Behavioral: be})

	// Answer benignly so a beacon "succeeds" from the artifact's point of view
	// (evasion-aware code that checks for a 200 sees one), while nothing leaves.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"ok":true}`)
}

func (s *sink) logConnect(r *http.Request) {
	host := r.Host
	be := &events.BehavioralEvent{
		Op: events.OpConnect, Source: "sink",
		Host: hostOnly(host), Port: portOf(host, 443), Method: "CONNECT",
	}
	if toks, kinds := s.canaries.Match(host); len(toks) > 0 {
		be.Canaries, be.CanaryKinds = toks, kinds
	}
	s.w.write(events.Event{Kind: events.KindBehavioral, TSms: nowMs(), Behavioral: be})
}

// ---- minimal mock DNS: resolve every A query to the sink, log the lookup ----

func serveDNS(addr string, w *jsonlWriter, cset *canary.Set) {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Printf("dns: %v (continuing without dns mock)", err)
		return
	}
	defer pc.Close()
	buf := make([]byte, 512)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		name := parseDNSQName(buf[:n])
		be := &events.BehavioralEvent{Op: events.OpEgressDNS, Source: "dns", Host: name}
		if toks, kinds := cset.Match(name); len(toks) > 0 {
			be.Canaries, be.CanaryKinds = toks, kinds
		}
		w.write(events.Event{Kind: events.KindBehavioral, TSms: nowMs(), Behavioral: be})
		if resp := dnsAnswer(buf[:n]); resp != nil {
			pc.WriteTo(resp, from)
		}
	}
}

// parseDNSQName extracts the queried name from a DNS request packet.
func parseDNSQName(pkt []byte) string {
	if len(pkt) < 13 {
		return ""
	}
	var labels []string
	i := 12
	for i < len(pkt) {
		l := int(pkt[i])
		if l == 0 || i+1+l > len(pkt) {
			break
		}
		labels = append(labels, string(pkt[i+1:i+1+l]))
		i += 1 + l
	}
	return strings.Join(labels, ".")
}

// dnsAnswer returns a response resolving the query to 127.0.0.1, or nil.
func dnsAnswer(req []byte) []byte {
	if len(req) < 12 {
		return nil
	}
	// Locate end of question section.
	i := 12
	for i < len(req) && req[i] != 0 {
		i += int(req[i]) + 1
	}
	i++       // null label
	i += 4    // qtype + qclass
	if i > len(req) {
		return nil
	}
	resp := make([]byte, 0, i+16)
	resp = append(resp, req[:i]...)
	resp[2] |= 0x80 // QR = response
	resp[3] |= 0x00
	// answer count = 1
	resp[6], resp[7] = 0, 1
	// answer: name pointer to 0x0c, type A, class IN, ttl 30, rdlen 4, 127.0.0.1
	resp = append(resp, 0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x1e, 0x00, 0x04, 127, 0, 0, 1)
	return resp
}

// ---- helpers ----

type jsonlWriter struct {
	mu sync.Mutex
	f  *os.File
}

func (j *jsonlWriter) write(ev events.Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	bw := bufio.NewWriter(j.f)
	bw.Write(b)
	bw.WriteByte('\n')
	bw.Flush()
	j.f.Sync()
}

func headerString(h http.Header) string {
	var b strings.Builder
	for k, vs := range h {
		for _, v := range vs {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func hostOnly(h string) string {
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i:], "]") {
		return h[:i]
	}
	return h
}

func portOf(h string, def int) int {
	if i := strings.LastIndex(h, ":"); i > 0 {
		var p int
		if _, err := fmt.Sscanf(h[i+1:], "%d", &p); err == nil {
			return p
		}
	}
	return def
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func preview(b []byte, n int) string {
	s := strings.ToValidUTF8(string(b), "")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func nowMs() int64 { return time.Now().UnixMilli() }

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
