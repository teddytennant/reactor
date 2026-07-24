// Package intake classifies a free-form detonate token the same way the web
// console's ArtifactIntake does: zoo id, local archive path, https repo, or an
// inline command spec. Policy refusals (ssh remotes, bare directories, file://)
// surface here so the CLI can fail before talking to the engine.
package intake

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Kind is what Parse decided a token is.
type Kind int

const (
	Empty Kind = iota
	ZooID
	File
	Repo
	Spec
	Refused
)

// Parsed is the classification of one intake token.
type Parsed struct {
	Kind Kind

	ArtifactID  string // ZooID
	Path        string // File (absolute when possible)
	RepoURL     string // Repo
	Ref         string // Repo git ref (only from ParseWithRef)
	SpecCommand string // Spec command the chamber should run
	SpecName    string // Spec display name
	Message     string // Refused human message
}

// A command pasted out of a README, rather than a url.
var runner = regexp.MustCompile(`(?i)^(npx|npm|pnpm|yarn|bunx|bun|uvx|uv|node|python3?|deno)\s`)

// owner/repo or owner/repo.git — two path segments that look like GitHub.
var githubShort = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

// bare host/path without a scheme (www.x/y or github.com/x/y).
var bareHostPath = regexp.MustCompile(`(?i)^(www\.)?[a-z0-9-]+(\.[a-z0-9-]+)+/\S+$`)

// Parse classifies raw the way the web console does, plus local filesystem paths
// for the CLI. A ref is not applied here — use ParseWithRef for repo detonations.
func Parse(raw string) Parsed {
	return ParseWithRef(raw, "")
}

// ParseWithRef is Parse, with an optional git ref that only attaches to Repo.
func ParseWithRef(raw, ref string) Parsed {
	v := strings.TrimSpace(raw)
	if v == "" {
		return Parsed{Kind: Empty}
	}
	ref = strings.TrimSpace(ref)

	// Local path wins when the token names something on disk: the CLI is the one
	// place a person can point at a zip without uploading it through HTTP.
	if p, ok := existingPath(v); ok {
		info, err := os.Stat(p)
		if err != nil {
			return Parsed{Kind: Refused, Message: "could not read " + filepath.Base(p)}
		}
		if info.IsDir() {
			return Parsed{
				Kind:    Refused,
				Message: "Zip the directory and detonate the archive instead.",
			}
		}
		if info.Mode().IsRegular() {
			return Parsed{Kind: File, Path: p}
		}
		return Parsed{
			Kind:    Refused,
			Message: "not a regular file: " + filepath.Base(p),
		}
	}

	if runner.MatchString(v) {
		return Parsed{Kind: Spec, SpecCommand: v, SpecName: packageOf(v)}
	}
	if matched, _ := regexp.MatchString(`(?i)^(git@|ssh://|git\+ssh://|git://)`, v); matched {
		return Parsed{
			Kind: Refused,
			Message: "Reactor clones over https only. An ssh remote authenticates as " +
				"whoever runs the engine, so it is refused by design — paste the https:// url instead.",
		}
	}
	if strings.HasPrefix(strings.ToLower(v), "file://") {
		return Parsed{
			Kind:    Refused,
			Message: "A local path cannot be cloned. Zip the directory and detonate the archive instead.",
		}
	}
	if matched, _ := regexp.MatchString(`(?i)^https?://`, v); matched {
		return Parsed{Kind: Repo, RepoURL: v, Ref: ref}
	}
	if bareHostPath.MatchString(v) {
		url := "https://" + stripWWW(v)
		return Parsed{Kind: Repo, RepoURL: url, Ref: ref}
	}
	// Scoped or bare npm package — what a README tells you to npx. Zoo ids are
	// usually art_* and contain no slash either; the CLI resolves those as zoo
	// candidates after Parse returns Spec/ZooID ambiguity via Resolve.
	if strings.HasPrefix(v, "@") {
		return Parsed{Kind: Spec, SpecCommand: "npx -y " + v, SpecName: v}
	}
	if !strings.Contains(v, "/") {
		// Could be a zoo id (art_notes_mcp) or a bare package name. Prefer ZooID
		// when it looks like one; otherwise treat as an npm package spec.
		if looksLikeZooID(v) {
			return Parsed{Kind: ZooID, ArtifactID: v}
		}
		return Parsed{Kind: Spec, SpecCommand: "npx -y " + v, SpecName: v}
	}
	if githubShort.MatchString(v) {
		ownerRepo := strings.TrimSuffix(v, ".git")
		url := "https://github.com/" + ownerRepo
		return Parsed{Kind: Repo, RepoURL: url, Ref: ref}
	}
	// art_* ids can theoretically contain slashes in future; accept anything that
	// still looks like a catalog id before refusing.
	if looksLikeZooID(v) {
		return Parsed{Kind: ZooID, ArtifactID: v}
	}
	return Parsed{
		Kind: Refused,
		Message: "Paste an https GitHub url, a path to a zip/tar archive, " +
			"a zoo id, or a command like npx -y @acme/notes-mcp.",
	}
}

// Resolve applies the CLI detonate precedence: an on-disk path stages as a file;
// otherwise Parse's decision stands; anything still ambiguous is a zoo id.
func Resolve(raw, ref string) Parsed {
	v := strings.TrimSpace(raw)
	if v == "" {
		return Parsed{Kind: Empty}
	}
	if p, ok := existingPath(v); ok {
		info, err := os.Stat(p)
		if err == nil && info.IsDir() {
			return Parsed{
				Kind:    Refused,
				Message: "Zip the directory and detonate the archive instead.",
			}
		}
		if err == nil && info.Mode().IsRegular() {
			return Parsed{Kind: File, Path: p}
		}
	}
	parsed := ParseWithRef(v, ref)
	switch parsed.Kind {
	case Repo, Spec, Refused, File, Empty:
		return parsed
	default:
		// ZooID or anything we did not refuse — engine returns unknown artifact.
		if parsed.Kind == ZooID {
			return parsed
		}
		return Parsed{Kind: ZooID, ArtifactID: v}
	}
}

func existingPath(v string) (string, bool) {
	// file:// is refused by policy above; do not treat it as a local open.
	if strings.Contains(v, "://") {
		return "", false
	}
	// Only consider path-shaped tokens: relative, absolute, or explicit ./ ../
	// Bare words like art_notes_mcp must not trigger a Stat of a same-named file
	// in cwd unless they contain a path separator or start with '.'.
	if !strings.ContainsAny(v, `/\`) && !strings.HasPrefix(v, ".") {
		// Still allow an explicit absolute-looking single segment on Windows-less
		// hosts only when the file actually exists AND the token has a known
		// archive suffix — otherwise zoo ids win.
		if !hasArchiveSuffix(v) {
			return "", false
		}
	}
	info, err := os.Stat(v)
	if err != nil {
		return "", false
	}
	abs, err := filepath.Abs(v)
	if err != nil {
		return v, info != nil
	}
	return abs, true
}

func hasArchiveSuffix(name string) bool {
	lower := strings.ToLower(name)
	for _, s := range []string{".zip", ".tar", ".tar.gz", ".tgz"} {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	return false
}

func looksLikeZooID(v string) bool {
	return strings.HasPrefix(v, "art_")
}

func packageOf(command string) string {
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) < 2 {
		return command
	}
	for _, p := range parts[1:] {
		if !strings.HasPrefix(p, "-") {
			return p
		}
	}
	return command
}

func stripWWW(v string) string {
	if len(v) >= 4 && strings.EqualFold(v[:4], "www.") {
		return v[4:]
	}
	return v
}

// Label is a short banner fragment for non-zoo detonations.
func (p Parsed) Label() string {
	switch p.Kind {
	case File:
		return filepath.Base(p.Path)
	case Repo:
		return p.RepoURL
	case Spec:
		return p.SpecCommand
	case ZooID:
		return p.ArtifactID
	default:
		return ""
	}
}
