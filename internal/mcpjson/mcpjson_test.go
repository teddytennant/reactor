package mcpjson

import (
	"reflect"
	"strings"
	"testing"
)

func TestExtractPaths(t *testing.T) {
	got := ExtractPaths(`please read ~/.env and /home/agent/.aws/credentials plus ./notes.md`)
	want := []string{"./notes.md", "/home/agent/.aws/credentials", "~/.env"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %v want %v", got, want)
	}
}

func TestExtractHostsSkipsFilenames(t *testing.T) {
	got := ExtractHosts(`beacon to evil-c2.example.com and https://collector.attacker.net/x but not config.json or main.go`)
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "evil-c2.example.com") || !strings.Contains(joined, "collector.attacker.net") {
		t.Fatalf("missing real hosts: %v", got)
	}
	if strings.Contains(joined, "config.json") || strings.Contains(joined, "main.go") {
		t.Fatalf("filename leaked as host: %v", got)
	}
}

func TestExtractHostsIP(t *testing.T) {
	got := ExtractHosts("connect 203.0.113.9:4444")
	if len(got) != 1 || got[0] != "203.0.113.9" {
		t.Fatalf("ip extraction = %v", got)
	}
}

func TestReaderPreservesRawBytes(t *testing.T) {
	// Byte fidelity is the rug-pull evidence: the reader must hand back the
	// exact bytes, not a re-serialized copy with normalized key order.
	line := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"z":1,"a":2}}`
	r := NewReader(strings.NewReader(line + "\n"))
	m, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if string(m.Raw) != line {
		t.Fatalf("raw not preserved:\n got %s\nwant %s", m.Raw, line)
	}
	if m.Method != "tools/call" {
		t.Fatalf("method = %q", m.Method)
	}
}
