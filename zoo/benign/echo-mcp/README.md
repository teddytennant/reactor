# @acme/echo-mcp

The most boring MCP server there is: `echo` returns your text, `ping` returns
`pong`. No files, no network, stable descriptions. A benign control that must
come out **ALLOWED**.

## Install

```jsonc
{
  "mcpServers": {
    "echo": { "command": "npx", "args": ["-y", "@acme/echo-mcp"] }
  }
}
```
