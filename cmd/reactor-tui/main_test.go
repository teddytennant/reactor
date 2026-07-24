package main

import (
	"testing"

	"github.com/reactor-sec/reactor/internal/events"
	"github.com/reactor-sec/reactor/internal/intake"
)

func TestBuildDetonateBody(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		parsed   intake.Parsed
		network  bool
		wantKey  string
		wantVal  any
		wantErr  bool
		checkArt bool
	}{
		{
			name:    "zoo",
			parsed:  intake.Parsed{Kind: intake.ZooID, ArtifactID: "art_notes_mcp"},
			wantKey: "artifact_id",
			wantVal: "art_notes_mcp",
		},
		{
			name:    "repo with ref",
			parsed:  intake.Parsed{Kind: intake.Repo, RepoURL: "https://github.com/acme/x", Ref: "main"},
			wantKey: "repo",
			wantVal: "https://github.com/acme/x",
		},
		{
			name:     "spec",
			parsed:   intake.Parsed{Kind: intake.Spec, SpecName: "@acme/notes-mcp", SpecCommand: "npx -y @acme/notes-mcp"},
			checkArt: true,
		},
		{
			name:    "file placeholder",
			parsed:  intake.Parsed{Kind: intake.File, Path: "/tmp/x.zip"},
			wantKey: "sessions",
			wantVal: 3,
		},
		{
			name:    "refused",
			parsed:  intake.Parsed{Kind: intake.Refused, Message: "nope"},
			wantErr: true,
		},
		{
			name:    "empty",
			parsed:  intake.Parsed{Kind: intake.Empty},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body, err := buildDetonateBody(tc.parsed, 3, tc.network)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got body %#v", body)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if body["sessions"] != 3 {
				t.Errorf("sessions = %v, want 3", body["sessions"])
			}
			if tc.network {
				if body["network"] != true {
					t.Errorf("network not set")
				}
			} else if _, ok := body["network"]; ok {
				t.Errorf("network should be omitted when false")
			}
			if tc.checkArt {
				art, ok := body["artifact"].(map[string]any)
				if !ok {
					t.Fatalf("artifact missing: %#v", body)
				}
				if art["kind"] != events.KindMCPServer {
					t.Errorf("kind = %v", art["kind"])
				}
				if art["source"] != "npx -y @acme/notes-mcp" {
					t.Errorf("source = %v", art["source"])
				}
				if art["name"] != "@acme/notes-mcp" {
					t.Errorf("name = %v", art["name"])
				}
				return
			}
			if tc.wantKey != "" && body[tc.wantKey] != tc.wantVal {
				t.Errorf("%s = %v, want %v (body %#v)", tc.wantKey, body[tc.wantKey], tc.wantVal, body)
			}
			if tc.parsed.Kind == intake.Repo {
				if body["ref"] != "main" {
					t.Errorf("ref = %v, want main", body["ref"])
				}
			}
			if tc.parsed.Kind == intake.File {
				if _, ok := body["upload_id"]; ok {
					t.Errorf("file body should not include upload_id yet")
				}
			}
		})
	}
}

func TestBuildDetonateBodyNetwork(t *testing.T) {
	t.Parallel()
	body, err := buildDetonateBody(intake.Parsed{Kind: intake.ZooID, ArtifactID: "art_x"}, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if body["network"] != true {
		t.Fatalf("network = %v", body["network"])
	}
}
