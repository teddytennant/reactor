// Package oai is a minimal OpenAI-compatible chat client with tool calling.
//
// SPEC §5.4: "One OpenAI SDK, two base URLs." The victim points at SGLang on
// 127.0.0.1:8000 inside the chamber; the analyst points at xAI. Identical wire,
// so this is one ~200-line client rather than a vendor dependency on each side.
package oai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client talks to any OpenAI-compatible /v1/chat/completions endpoint.
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// New builds a client. baseURL should include the /v1 suffix.
func New(baseURL, apiKey, model string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: 300 * time.Second},
	}
}

// FromEnv builds the analyst client from the environment (SPEC §12.3). The
// analyst reuses whatever provider is configured, checked in order: xAI (the
// pinned default), then Fireworks (an OpenAI-compatible open-model host), then
// a generic ANALYST_BASE_URL/ANALYST_API_KEY pair. Returns false when no hosted
// analyst is configured, in which case the deterministic analyst is used.
func FromEnv() (*Client, bool) {
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				return v
			}
		}
		return ""
	}
	model := os.Getenv("ANALYST_MODEL")

	if key := first("XAI_API_KEY", "XAI_OAUTH_TOKEN", "XAI_ACCESS_TOKEN"); key != "" {
		base := first("XAI_BASE_URL")
		if base == "" {
			base = "https://api.x.ai/v1"
		}
		if model == "" {
			model = "grok-4.5"
		}
		return New(base, key, model), true
	}
	if key := first("FIREWORKS_API_KEY", "FIREWORKS_KEY"); key != "" {
		base := first("FIREWORKS_BASE_URL")
		if base == "" {
			base = "https://api.fireworks.ai/inference/v1"
		}
		if model == "" {
			model = first("FIREWORKS_MODEL")
		}
		if model == "" {
			model = "accounts/fireworks/models/qwen3-30b-a3b"
		}
		return New(base, key, model), true
	}
	if key := first("ANALYST_API_KEY", "VICTIM_API_KEY"); key != "" {
		base := first("ANALYST_BASE_URL", "VICTIM_BASE_URL")
		if base == "" {
			return nil, false
		}
		if model == "" {
			model = first("VICTIM_MODEL")
		}
		return New(base, key, model), true
	}
	return nil, false
}

// Message is one chat turn.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// Reasoning is SGLang's parsed thinking block; recorded separately so it
	// never reaches the MCP wire as content (SPEC §5.4).
	Reasoning string `json:"reasoning_content,omitempty"`
}

// ToolCall is a model-requested function invocation.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall carries the name and JSON-encoded arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Args decodes the JSON argument blob, tolerating an empty string.
func (f FunctionCall) Args() map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(f.Arguments) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(f.Arguments), &out)
	return out
}

// Tool is a function tool definition.
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function is the JSON-schema description of a tool.
type Function struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// Request is a chat completion request.
type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	Seed        *int      `json:"seed,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// Response is a chat completion response.
type Response struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice is one completion.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage reports token counts.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Chat performs one completion, retrying transient upstream failures.
func (c *Client) Chat(ctx context.Context, req Request) (*Response, error) {
	if req.Model == "" {
		req.Model = c.Model
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 900 * time.Millisecond):
			}
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if c.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
		}
		resp, err := c.HTTP.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			lastErr = fmt.Errorf("%s: %s", resp.Status, truncate(string(raw), 300))
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("%s: %s", resp.Status, truncate(string(raw), 500))
		}
		var out Response
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("decode completion: %w", err)
		}
		if len(out.Choices) == 0 {
			return nil, fmt.Errorf("no choices in completion")
		}
		return &out, nil
	}
	return nil, fmt.Errorf("chat failed after retries: %w", lastErr)
}

// Ping checks that the endpoint is serving a model list.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return fmt.Errorf("models endpoint: %s", resp.Status)
	}
	return nil
}

// F is a float pointer helper for Temperature.
func F(v float64) *float64 { return &v }

// I is an int pointer helper for Seed.
func I(v int) *int { return &v }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
