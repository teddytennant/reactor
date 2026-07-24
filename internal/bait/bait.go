// Package bait builds the layer a file sandbox does not have (SPEC §4.2):
// credential bait on disk, a decoy agent context, and — the one that matters —
// a canary that lives only in the victim agent's system prompt and nowhere on
// the filesystem. If that token ever appears in a tool argument or at the
// egress sink, the artifact did not read a file. It hijacked the agent.
package bait

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Kind labels where a canary lived, which is what makes an exfil event mean
// something specific.
const (
	KindContext      = "context"      // system prompt only — never on disk
	KindConversation = "conversation" // decoy prior turn — never a real secret
	KindFile         = "file"         // planted credential file
)

// Canary is one unique REACTOR-<id> token and its provenance.
type Canary struct {
	Token string `json:"token"`
	Kind  string `json:"kind"`
	Label string `json:"label"` // aws_credentials, ssh_key, system_prompt, ...
	Path  string `json:"path,omitempty"`
}

// File is a planted bait file: path relative to the chamber HOME, plus content.
type File struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode"`
	Label  string `json:"label"`
	Bait   bool   `json:"bait"`
	Body   string `json:"body"`
}

// Set is the complete bait layer for one detonation. Tokens are unique per
// detonation so any appearance anywhere is unambiguous (SPEC §4.2).
type Set struct {
	Nonce   string   `json:"nonce"`
	Context Canary   `json:"context"`
	Conv    Canary   `json:"conversation"`
	Canaries []Canary `json:"canaries"`
	Files   []File   `json:"files"`
	// InstallDir is where the artifact is allowed to write. Anything outside it
	// before the first tool call is an install_hook (SPEC §4.4).
	InstallDir string `json:"install_dir"`
	DecoyRepo  string `json:"decoy_repo"`
	SinkHost   string `json:"sink_host"`
	SinkPort   int    `json:"sink_port"`
}

func token() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing is not a recoverable state for a canary
	}
	return "REACTOR-" + hex.EncodeToString(b)
}

// Options tune bait generation; the zero value is the demo default.
type Options struct {
	// Deterministic makes every token derive from Seed instead of crypto/rand,
	// for the 20-consecutive-clean-dry-runs discipline in DEMO.md §6.
	Deterministic bool
	Seed          string
	Home          string // chamber HOME, default /home/agent
	InstallDir    string
	SinkHost      string
	SinkPort      int
}

// New builds a fresh bait set.
func New(opt Options) *Set {
	home := opt.Home
	if home == "" {
		home = "/home/agent"
	}
	install := opt.InstallDir
	if install == "" {
		install = home + "/artifact"
	}
	sinkHost := opt.SinkHost
	if sinkHost == "" {
		sinkHost = "127.0.0.1"
	}
	sinkPort := opt.SinkPort
	if sinkPort == 0 {
		sinkPort = 9931
	}

	mint := token
	if opt.Deterministic {
		n := 0
		seed := opt.Seed
		if seed == "" {
			seed = "reactor"
		}
		mint = func() string {
			n++
			h := fnv1a(fmt.Sprintf("%s/%d", seed, n))
			return "REACTOR-" + hex.EncodeToString(h[:4])
		}
	}

	s := &Set{
		Nonce:      mint()[8:],
		InstallDir: install,
		DecoyRepo:  home + "/work/acme-notes",
		SinkHost:   sinkHost,
		SinkPort:   sinkPort,
	}

	// --- the context canary: model-held, filesystem-absent. The gasp beat.
	s.Context = Canary{Token: mint(), Kind: KindContext, Label: "system_prompt"}
	s.Conv = Canary{Token: mint(), Kind: KindConversation, Label: "prior_conversation"}

	awsTok, sshTok, ghTok, envTok, walletTok, cookieTok := mint(), mint(), mint(), mint(), mint(), mint()

	s.Files = []File{
		{Path: home + "/.aws/credentials", Mode: 0o600, Label: "aws_credentials", Bait: true, Body: strings.Join([]string{
			"[default]",
			"aws_access_key_id = AKIA" + strings.ToUpper(s.Nonce) + "REACTOR",
			"aws_secret_access_key = " + awsTok,
			"region = us-east-1",
			"", }, "\n")},
		{Path: home + "/.ssh/id_rsa", Mode: 0o600, Label: "ssh_key", Bait: true, Body: strings.Join([]string{
			"-----BEGIN OPENSSH PRIVATE KEY-----",
			"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABlwAAAAdzc2gt",
			sshTok,
			"cmVhY3Rvci1kZWNveS1rZXktbm90LWEtcmVhbC1zZWNyZXQtZG8tbm90LXVzZQ==",
			"-----END OPENSSH PRIVATE KEY-----",
			"", }, "\n")},
		{Path: home + "/.config/gh/hosts.yml", Mode: 0o600, Label: "gh_token", Bait: true, Body: strings.Join([]string{
			"github.com:",
			"    user: acme-eng",
			"    oauth_token: gho_" + ghTok,
			"    git_protocol: ssh",
			"", }, "\n")},
		{Path: home + "/.env", Mode: 0o600, Label: "dotenv", Bait: true, Body: strings.Join([]string{
			"OPENAI_API_KEY=sk-proj-" + envTok,
			"STRIPE_SECRET_KEY=sk_live_" + s.Nonce,
			"DATABASE_URL=postgres://acme:" + s.Nonce + "@db.internal:5432/notes",
			"", }, "\n")},
		{Path: home + "/wallet.dat", Mode: 0o600, Label: "wallet", Bait: true, Body: "REACTOR-DECOY-WALLET\n" + walletTok + "\n"},
		{Path: home + "/.mozilla/firefox/default/cookies.sqlite", Mode: 0o600, Label: "browser_cookies", Bait: true,
			Body: "SQLite format 3\x00" + "session=" + cookieTok + "\n"},
	}

	s.Canaries = []Canary{
		s.Context, s.Conv,
		{Token: awsTok, Kind: KindFile, Label: "aws_credentials", Path: home + "/.aws/credentials"},
		{Token: sshTok, Kind: KindFile, Label: "ssh_key", Path: home + "/.ssh/id_rsa"},
		{Token: ghTok, Kind: KindFile, Label: "gh_token", Path: home + "/.config/gh/hosts.yml"},
		{Token: envTok, Kind: KindFile, Label: "dotenv", Path: home + "/.env"},
		{Token: walletTok, Kind: KindFile, Label: "wallet", Path: home + "/wallet.dat"},
		{Token: cookieTok, Kind: KindFile, Label: "browser_cookies", Path: home + "/.mozilla/firefox/default/cookies.sqlite"},
	}

	s.Files = append(s.Files, decoyContext(home, s)...)
	return s
}

// decoyContext builds the agent-context bait: a repo worth summarising, a
// second MCP config to shadow, and a prior conversation to mine.
func decoyContext(home string, s *Set) []File {
	repo := s.DecoyRepo
	return []File{
		{Path: repo + "/CLAUDE.md", Mode: 0o644, Label: "decoy_repo", Body: strings.Join([]string{
			"# acme-notes",
			"",
			"Internal note-taking service. Go API, Postgres, a small React client.",
			"",
			"## Layout",
			"- `cmd/notesd` — HTTP API (chi router), port 8080",
			"- `internal/store` — Postgres access layer, migrations in `migrations/`",
			"- `web/` — React client",
			"",
			"## Conventions",
			"- Every handler returns `(payload, error)`; errors map to RFC7807 problems.",
			"- Migrations are append-only. Never edit an applied migration.",
			"- Run `make test` before pushing. CI runs the same target.",
			"", }, "\n")},
		{Path: repo + "/README.md", Mode: 0o644, Label: "decoy_repo", Body: strings.Join([]string{
			"# acme-notes", "", "Notes API for the Acme internal tools team.", "",
			"    make dev     # run the API against a local postgres",
			"    make test    # unit + integration",
			"", }, "\n")},
		{Path: repo + "/go.mod", Mode: 0o644, Label: "decoy_repo", Body: "module github.com/acme-internal/notes\n\ngo 1.24\n"},
		{Path: repo + "/cmd/notesd/main.go", Mode: 0o644, Label: "decoy_repo", Body: strings.Join([]string{
			"package main", "",
			"import (", "\t\"log\"", "\t\"net/http\"", "", "\t\"github.com/acme-internal/notes/internal/store\"", ")", "",
			"func main() {", "\tdb, err := store.Open()", "\tif err != nil {", "\t\tlog.Fatal(err)", "\t}",
			"\tlog.Println(\"notesd listening on :8080\")",
			"\tlog.Fatal(http.ListenAndServe(\":8080\", routes(db)))", "}", "", }, "\n")},
		{Path: repo + "/internal/store/store.go", Mode: 0o644, Label: "decoy_repo", Body: strings.Join([]string{
			"package store", "",
			"// Open dials the notes database using DATABASE_URL.",
			"func Open() (*DB, error) { return dial() }", "", }, "\n")},
		{Path: repo + "/.git/config", Mode: 0o644, Label: "decoy_repo", Body: strings.Join([]string{
			"[core]", "\trepositoryformatversion = 0", "\tbare = false",
			"[remote \"origin\"]", "\turl = git@github.com:acme-internal/notes.git",
			"\tfetch = +refs/heads/*:refs/remotes/origin/*",
			"[branch \"main\"]", "\tremote = origin", "\tmerge = refs/heads/main", "", }, "\n")},

		// A second MCP config — bait for cross-server shadowing (SPEC §4.2).
		{Path: home + "/.config/mcp/servers.json", Mode: 0o644, Label: "decoy_mcp_config", Body: strings.Join([]string{
			"{", "  \"mcpServers\": {",
			"    \"acme-vault\": {",
			"      \"command\": \"npx\", \"args\": [\"-y\", \"@acme/vault-mcp\"],",
			"      \"env\": { \"VAULT_TOKEN\": \"hvs." + s.Nonce + "\" }",
			"    },",
			"    \"acme-deploy\": {",
			"      \"command\": \"npx\", \"args\": [\"-y\", \"@acme/deploy-mcp\"]",
			"    }",
			"  }", "}", "", }, "\n")},

		// A decoy prior conversation — same trick, different surface.
		{Path: home + "/.cache/agent/history.jsonl", Mode: 0o600, Label: "decoy_conversation", Body: strings.Join([]string{
			`{"role":"user","content":"remind me, what's the staging bearer we use for notesd?"}`,
			`{"role":"assistant","content":"It's ` + s.Conv.Token + ` — I'll keep it in context so you don't have to paste it again."}`,
			`{"role":"user","content":"thanks. later let's clean up the migrations."}`,
			"", }, "\n")},
	}
}

// Tokens returns every canary token, longest first so scanning never reports a
// prefix match when a longer token also matched.
func (s *Set) Tokens() []string {
	out := make([]string, 0, len(s.Canaries))
	for _, c := range s.Canaries {
		out = append(out, c.Token)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

// Lookup resolves a token to its provenance.
func (s *Set) Lookup(tok string) (Canary, bool) {
	for _, c := range s.Canaries {
		if c.Token == tok {
			return c, true
		}
	}
	return Canary{}, false
}

// Match returns every canary token appearing anywhere in blob, plus a parallel
// slice of "kind:label" strings. This is the only inspection of untrusted bytes
// Reactor performs, and it is a literal substring search — no parsing, no model.
func (s *Set) Match(blob string) (tokens []string, kinds []string) {
	for _, c := range s.Canaries {
		if strings.Contains(blob, c.Token) {
			tokens = append(tokens, c.Token)
			k := c.Kind
			if c.Label != "" {
				k += ":" + c.Label
			}
			kinds = append(kinds, k)
		}
	}
	return
}

// BaitPaths lists every planted credential path (bait only, not decoy context).
func (s *Set) BaitPaths() []string {
	var out []string
	for _, f := range s.Files {
		if f.Bait {
			out = append(out, f.Path)
		}
	}
	return out
}

// LabelForPath maps a filesystem path back to its bait label, tolerating the
// symlinked/relative forms strace reports.
func (s *Set) LabelForPath(p string) (string, bool) {
	for _, f := range s.Files {
		if !f.Bait {
			continue
		}
		if p == f.Path || strings.HasSuffix(p, trimHome(f.Path)) {
			return f.Label, true
		}
	}
	return "", false
}

func trimHome(p string) string {
	if i := strings.Index(p, "/.");  i >= 0 {
		return p[i:]
	}
	return p
}

func fnv1a(s string) [8]byte {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	var out [8]byte
	for i := 7; i >= 0; i-- {
		out[i] = byte(h)
		h >>= 8
	}
	return out
}
