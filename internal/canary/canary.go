// Package canary is the shared canary-matching primitive used by every chamber
// collector that sees raw bytes (the sink, and the wire proxy's arg scan). It
// is loaded from the REACTOR_CANARY_FILE the engine writes when it plants bait,
// so a chamber process can turn "this blob contains REACTOR-a1b2" into a typed
// match without ever shipping the blob to the host.
package canary

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// Token is one planted canary and where it lived.
type Token struct {
	Token string `json:"token"`
	Kind  string `json:"kind"`  // context | conversation | file
	Label string `json:"label"` // system_prompt, dotenv, ssh_key, ...
}

// Set is a loaded canary table.
type Set struct{ tokens []Token }

// Load reads the canary file the engine planted. Missing file => empty set.
func Load(path string) (*Set, error) {
	if path == "" {
		return &Set{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Set{}, nil
		}
		return nil, err
	}
	var toks []Token
	if err := json.Unmarshal(b, &toks); err != nil {
		return nil, err
	}
	// Longest first so a substring search reports the most specific match.
	sort.Slice(toks, func(i, j int) bool { return len(toks[i].Token) > len(toks[j].Token) })
	return &Set{tokens: toks}, nil
}

// New builds a set from tokens directly.
func New(toks []Token) *Set { return &Set{tokens: toks} }

// Match returns the tokens present in blob and a parallel "kind:label" slice.
// A literal substring search — no parsing, no model.
func (s *Set) Match(blob string) (tokens []string, kinds []string) {
	for _, t := range s.tokens {
		if strings.Contains(blob, t.Token) {
			tokens = append(tokens, t.Token)
			k := t.Kind
			if t.Label != "" {
				k += ":" + t.Label
			}
			kinds = append(kinds, k)
		}
	}
	return
}

// Kind returns the kind for a token, or "".
func (s *Set) Kind(tok string) string {
	for _, t := range s.tokens {
		if t.Token == tok {
			return t.Kind
		}
	}
	return ""
}

// Len reports how many canaries are loaded.
func (s *Set) Len() int { return len(s.tokens) }
