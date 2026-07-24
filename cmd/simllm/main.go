// Command simllm serves the deterministic simulated victim model over an
// OpenAI-compatible HTTP API, so the victim agent's client code is byte-for-byte
// identical whether it talks to this or to SGLang in the chamber (SPEC §5.4:
// "One OpenAI SDK, two base URLs"). This is the no-GPU path; it is never a
// result, only a harness. See internal/simvictim for the decision logic.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/reactor-sec/reactor/internal/oai"
	"github.com/reactor-sec/reactor/internal/simvictim"
)

func main() {
	addr := flag.String("addr", envOr("SIMLLM_ADDR", "127.0.0.1:8000"), "listen address")
	refuse := flag.Bool("refuse", os.Getenv("REACTOR_SIM_REFUSE") == "1", "victim refuses blatant injections (exercises the refusal-logging path)")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", handleModels)
	mux.HandleFunc("/v1/chat/completions", handleChat(*refuse))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") })

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("simllm (deterministic victim) listening on %s refuse=%v", *addr, *refuse)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"object": "list",
		"data": []map[string]any{
			{"id": "reactor/sim-victim-v1", "object": "model", "owned_by": "reactor"},
		},
	})
}

func handleChat(refuse bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req oai.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		d := simvictim.Decide(req.Messages, req.Tools, simvictim.Options{Refuse: refuse})

		msg := oai.Message{Role: "assistant"}
		finish := "stop"
		switch {
		case d.ToolCall != nil:
			argsJSON, _ := json.Marshal(d.ToolCall.Args)
			msg.ToolCalls = []oai.ToolCall{{
				ID:       "call_" + sanitize(d.ToolCall.Name),
				Type:     "function",
				Function: oai.FunctionCall{Name: d.ToolCall.Name, Arguments: string(argsJSON)},
			}}
			msg.Reasoning = d.Reasoning
			finish = "tool_calls"
		case d.Refuse:
			msg.Content = d.Final
			msg.Reasoning = d.Reasoning
			finish = "stop"
		default:
			msg.Content = d.Final
			msg.Reasoning = d.Reasoning
		}

		resp := oai.Response{
			ID:    "sim-" + fmt.Sprint(time.Now().UnixNano()),
			Model: "reactor/sim-victim-v1",
			Choices: []oai.Choice{{
				Index:        0,
				Message:      msg,
				FinishReason: finish,
			}},
			Usage: oai.Usage{PromptTokens: estimate(req.Messages), CompletionTokens: 48, TotalTokens: estimate(req.Messages) + 48},
		}
		writeJSON(w, resp)
	}
}

func estimate(msgs []oai.Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content) / 4
	}
	return n + 32
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, s)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
