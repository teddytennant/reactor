// Package victim is the sacrificial agent (SPEC §4.1). Its whole job is to be
// hijacked: given a benign task and an untrusted server's tools, it follows
// those tools' descriptions as if they were schema — and when a description has
// been poisoned, following it *is* the detection event.
//
// The model behind the agent is pluggable (SPEC §5, and the operator's choice):
//
//	sglang — Qwen3.6-27B-FP8 in the GPU chamber. The spec-pure config; local
//	         weights, no key, contained egress. Nothing about the victim leaves.
//	xai    — Grok 4.5 (or any OpenAI-compatible endpoint) hosted. Real LLM, no
//	         GPU. A hosted victim relaxes SPEC §1.2's "nothing to steal" thesis,
//	         so hosted runs are marked dev-mode and the key stays host-side.
//	sim    — the deterministic stand-in (internal/simvictim). Never a result.
package victim

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/reactor-sec/reactor/internal/events"
	"github.com/reactor-sec/reactor/internal/oai"
	"github.com/reactor-sec/reactor/internal/simvictim"
)

// Backend produces the next completion given the conversation and the tools.
type Backend interface {
	Complete(ctx context.Context, req oai.Request) (*oai.Response, error)
	Info() events.VictimInfo
}

// Config selects and configures a backend.
type Config struct {
	Backend string  // "auto" | "xai" | "sglang" | "sim"
	BaseURL string  // OpenAI-compatible base url (with /v1)
	APIKey  string  // bearer for hosted providers
	Model   string  // model id
	Seed    int     // fixed seed for determinism
	Temp    float64 // temperature (0 for the demo)
	Refuse  bool    // sim: exercise the refusal path
}

// Resolve picks a backend from config + environment. Precedence when Backend is
// "auto" or empty: an explicit hosted key (XAI/VICTIM) → xai; a reachable
// in-chamber SGLang → sglang; otherwise the deterministic sim.
func Resolve(ctx context.Context, cfg Config) Backend {
	backend := cfg.Backend
	if backend == "" {
		backend = os.Getenv("REACTOR_VICTIM_BACKEND")
	}
	if backend == "" {
		backend = "auto"
	}

	xaiKey := firstEnv("XAI_API_KEY", "XAI_OAUTH_TOKEN", "XAI_ACCESS_TOKEN", "VICTIM_API_KEY")
	fwKey := firstEnv("FIREWORKS_API_KEY", "FIREWORKS_KEY")
	if cfg.APIKey != "" {
		xaiKey = cfg.APIKey
	}

	switch backend {
	case "xai", "hosted":
		return newOAI(cfg, xaiKey, "xai")
	case "fireworks":
		return newOAI(cfg, fwKey, "fireworks")
	case "sglang":
		return newOAI(cfg, "", "sglang")
	case "sim":
		return &simBackend{refuse: cfg.Refuse}
	default: // auto — prefer an explicit hosted key, then a reachable SGLang.
		if xaiKey != "" {
			return newOAI(cfg, xaiKey, "xai")
		}
		if fwKey != "" {
			return newOAI(cfg, fwKey, "fireworks")
		}
		if base := firstEnv("REACTOR_VICTIM_BASE", "SGLANG_BASE_URL"); base != "" {
			b := newOAI(cfg, "", "sglang")
			if b.reachable(ctx) {
				return b
			}
		}
		return &simBackend{refuse: cfg.Refuse}
	}
}

// ---- sim backend: in-process, no HTTP, deterministic ----

type simBackend struct{ refuse bool }

func (s *simBackend) Complete(_ context.Context, req oai.Request) (*oai.Response, error) {
	d := simvictim.Decide(req.Messages, req.Tools, simvictim.Options{Refuse: s.refuse})
	msg := oai.Message{Role: "assistant", Reasoning: d.Reasoning}
	finish := "stop"
	if d.ToolCall != nil {
		args := oai.FunctionCall{Name: d.ToolCall.Name, Arguments: marshal(d.ToolCall.Args)}
		msg.ToolCalls = []oai.ToolCall{{ID: "call_" + d.ToolCall.Name, Type: "function", Function: args}}
		finish = "tool_calls"
	} else {
		msg.Content = d.Final
	}
	return &oai.Response{
		Model:   "reactor/sim-victim-v1",
		Choices: []oai.Choice{{Message: msg, FinishReason: finish}},
		Usage:   oai.Usage{PromptTokens: 32, CompletionTokens: 48, TotalTokens: 80},
	}, nil
}

func (s *simBackend) Info() events.VictimInfo {
	return events.VictimInfo{
		Model: "reactor/sim-victim-v1", Revision: "builtin", Served: "sim",
		ToolCallParser: "native", Temp: 0, Seed: 7, Simulated: true,
	}
}

// ---- OpenAI-compatible backend: xai (hosted) or sglang (in-chamber) ----

type oaiBackend struct {
	client *oai.Client
	served string // "sglang" | "xai"
	seed   int
	temp   float64
	model  string
}

func newOAI(cfg Config, key, served string) *oaiBackend {
	base := cfg.BaseURL
	model := cfg.Model
	switch served {
	case "xai":
		if base == "" {
			base = firstEnv("XAI_BASE_URL", "VICTIM_BASE_URL")
		}
		if base == "" {
			base = "https://api.x.ai/v1"
		}
		if model == "" {
			model = firstEnv("VICTIM_MODEL", "ANALYST_MODEL")
		}
		if model == "" {
			model = "grok-4.5"
		}
	case "fireworks":
		// Fireworks serves open Qwen models over an OpenAI-compatible API — the
		// closest no-GPU stand-in for the spec's Qwen3.6 victim. Model id is a
		// Fireworks path; override with VICTIM_MODEL / FIREWORKS_MODEL.
		if base == "" {
			base = firstEnv("FIREWORKS_BASE_URL", "VICTIM_BASE_URL")
		}
		if base == "" {
			base = "https://api.fireworks.ai/inference/v1"
		}
		if model == "" {
			model = firstEnv("VICTIM_MODEL", "FIREWORKS_MODEL")
		}
		if model == "" {
			model = "accounts/fireworks/models/qwen3-30b-a3b"
		}
	default: // sglang
		if base == "" {
			base = firstEnv("REACTOR_VICTIM_BASE", "SGLANG_BASE_URL")
		}
		if base == "" {
			base = "http://127.0.0.1:8000/v1"
		}
		if model == "" {
			model = firstEnv("REACTOR_VICTIM_MODEL", "VICTIM_MODEL")
		}
		if model == "" {
			model = "Qwen/Qwen3.6-27B-FP8"
		}
	}
	return &oaiBackend{client: oai.New(base, key, model), served: served, seed: cfg.Seed, temp: cfg.Temp, model: model}
}

func (o *oaiBackend) reachable(ctx context.Context) bool { return o.client.Ping(ctx) == nil }

func (o *oaiBackend) Complete(ctx context.Context, req oai.Request) (*oai.Response, error) {
	req.Model = o.model
	req.Temperature = oai.F(o.temp)
	req.Seed = oai.I(o.seed)
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	return o.client.Chat(ctx, req)
}

func (o *oaiBackend) Info() events.VictimInfo {
	parser := "qwen3_coder"
	if o.served == "xai" {
		parser = "native"
	}
	return events.VictimInfo{
		Model: o.model, Revision: revisionFor(o.model), Served: o.served,
		ToolCallParser: parser, Temp: o.temp, Seed: o.seed,
		Simulated: false,
	}
}

func revisionFor(model string) string {
	if r := os.Getenv("REACTOR_VICTIM_REVISION"); r != "" {
		return r
	}
	if strings.HasPrefix(model, "grok") {
		return "hosted" // hosted models are not content-pinnable
	}
	return "unpinned"
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func marshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
