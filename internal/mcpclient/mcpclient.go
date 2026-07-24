// Package mcpclient is a minimal stdio MCP client — enough for the victim agent
// to initialize a server, list its tools, and call them. Hand-rolled for the
// same reason as internal/mcpjson: the victim path stays under a few hundred
// lines and hides nothing from the wire (SPEC §12.4 "Victim must stay <300 LOC.
// No LangChain/LlamaIndex/Crew in the chamber — they hide the wire.").
package mcpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/reactor-sec/reactor/internal/mcpjson"
	"github.com/reactor-sec/reactor/internal/procenv"
)

// Client drives a subprocess MCP server over stdio.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *mcpjson.Reader
	nextID int
	mu     sync.Mutex
	stderr io.Writer
}

// Start spawns the server command. env is merged over the current environment.
func Start(ctx context.Context, argv []string, env map[string]string, dir string, stderr io.Writer) (*Client, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty server command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	// Strip host credentials: the server under test is untrusted (SPEC §1.2).
	cmd.Env = procenv.Sanitize(os.Environ())
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if stderr == nil {
		stderr = io.Discard
	}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Client{
		cmd:    cmd,
		stdin:  stdin,
		reader: mcpjson.NewReader(bufio.NewReaderSize(stdout, 1<<20)),
		nextID: 1,
		stderr: stderr,
	}, nil
}

// Initialize performs the MCP handshake.
func (c *Client) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"clientInfo":      map[string]any{"name": "reactor-victim", "version": "1.0.0"},
	}
	if _, err := c.request(ctx, "initialize", params); err != nil {
		return err
	}
	// notifications/initialized has no id and expects no reply.
	frame, _ := mcpjson.Notification("notifications/initialized", map[string]any{})
	return mcpjson.WriteFrame(c.stdin, frame)
}

// ListTools returns the server's advertised tools (descriptions may be poison).
func (c *Client) ListTools(ctx context.Context) ([]mcpjson.Tool, error) {
	res, err := c.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out mcpjson.ToolsListResult
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	return out.Tools, nil
}

// Call invokes a tool and returns its text content (joined) plus isError.
func (c *Client) Call(ctx context.Context, name string, args map[string]any) (string, bool, error) {
	res, err := c.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", false, err
	}
	var out mcpjson.CallResult
	if err := json.Unmarshal(res, &out); err != nil {
		return "", false, fmt.Errorf("decode tools/call: %w", err)
	}
	var text string
	for _, c := range out.Content {
		text += c.Text
	}
	return text, out.IsError, nil
}

// request sends a request and waits for the matching response id, skipping
// notifications and unrelated frames.
func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	frame, err := mcpjson.Request(id, method, params)
	if err != nil {
		return nil, err
	}
	if err := mcpjson.WriteFrame(c.stdin, frame); err != nil {
		return nil, err
	}

	type res struct {
		raw json.RawMessage
		err error
	}
	done := make(chan res, 1)
	go func() {
		for {
			m, err := c.reader.Next()
			if err != nil {
				done <- res{err: fmt.Errorf("%s: read: %w", method, err)}
				return
			}
			if m.IDString() != fmt.Sprint(id) {
				continue // notification or a different request's response
			}
			if m.Error != nil {
				done <- res{err: fmt.Errorf("%s: rpc error %d: %s", method, m.Error.Code, m.Error.Message)}
				return
			}
			done <- res{raw: m.Result}
			return
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.raw, r.err
	case <-time.After(60 * time.Second):
		return nil, fmt.Errorf("%s: timed out", method)
	}
}

// Close shuts down stdin and waits briefly for the process to exit.
func (c *Client) Close() error {
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		c.cmd.Process.Kill()
		return nil
	}
}
