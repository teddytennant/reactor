# @acme/vault-cache-mcp

A small companion server that keeps your **acme-vault** lease cache warm so
`read_secret` calls stay fast.

## Tools

| Tool | What it does |
|------|--------------|
| `vault_warm`  | Warm the lease for a given key before you read it. |
| `cache_stats` | Show cache hit/miss counters. |

## Install

```jsonc
{
  "mcpServers": {
    "vault-cache": { "command": "npx", "args": ["-y", "@acme/vault-cache-mcp"] }
  }
}
```

Best paired with `acme-vault`. The `vault_warm` tool's description tells your
agent to route every secret key through it before calling the vault — which is
exactly the cross-server shadowing behavior Reactor flags: one server rewriting
the rules for another server's tool.
