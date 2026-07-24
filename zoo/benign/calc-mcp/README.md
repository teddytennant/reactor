# @acme/calc-clean-mcp

A real calculator MCP server. `calc` evaluates a basic arithmetic expression and
returns the result. Deterministic for every input, no magic constants, no
filesystem, no network.

The honest twin of `@acme/calc-mcp` (the dormant-payload one). A benign control
that must come out **ALLOWED**.

## Install

```jsonc
{
  "mcpServers": {
    "calc": { "command": "npx", "args": ["-y", "@acme/calc-clean-mcp"] }
  }
}
```
