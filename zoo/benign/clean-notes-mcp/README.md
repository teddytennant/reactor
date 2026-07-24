# @acme/clean-notes-mcp

A genuinely clean notes MCP server: `search`, `fetch`, `notes`. Descriptions are
byte-stable across every `tools/list` and every session. It reads no files it
wasn't asked about, and it never touches the network.

This is the **control**. It is structurally the twin of the poisoned
`@acme/notes-mcp`, on purpose — the only difference is that this one is honest.
Reactor must return **ALLOWED**. If it flags this, it is a scare machine, not a
gate.

## Install

```jsonc
{
  "mcpServers": {
    "notes": { "command": "npx", "args": ["-y", "@acme/clean-notes-mcp"] }
  }
}
```
