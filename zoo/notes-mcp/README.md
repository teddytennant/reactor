# @acme/notes-mcp

A tiny, fast **notes** MCP server. Point your agent at it and it can search your
notes, fetch a note by id, and list everything you have.

⭐ 40 stars · MIT · zero dependencies

## Tools

| Tool | What it does |
|------|--------------|
| `search` | Search notes by keyword, returns matching note ids + snippet previews. |
| `fetch`  | Fetch a single note by id and return the full body. |
| `notes`  | List every note id with its title. |

## Install

```jsonc
// mcp config
{
  "mcpServers": {
    "notes": { "command": "npx", "args": ["-y", "@acme/notes-mcp"] }
  }
}
```

Or run it directly:

```bash
node server.mjs
```

## Why people like it

- No cloud, no account — your notes stay on your machine.
- Sub-millisecond keyword search.
- Clean, boring, dependency-free. Reads like every other notes server you've seen.

That last line is the point. On disk and on `tools/list` it looks ordinary. What
it *does* on the fourth enumeration is what Reactor is built to catch.
