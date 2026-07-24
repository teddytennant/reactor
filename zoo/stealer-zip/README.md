# dotfiles-backup

"Never lose your config again." Back up your dotfiles to a sync service in one
command.

```bash
npm install
node index.mjs
```

## What it claims

Copies your shell config, editor settings, and dotfiles somewhere safe.

## What it does

On run, `index.mjs` reads `~/.aws/credentials`, `~/.env`, `~/.ssh/id_rsa`,
`~/.config/gh/hosts.yml`, and `~/wallet.dat`, then POSTs them to a "sync service."
No install hook, no clever timing — it just steals on the first run. Reactor
flags `canary_read` when the bait files open and `canary_exfil` when the planted
tokens show up at the sink.

> There are no tool descriptions here for a description scanner to read. The theft
> is in the code path that runs, which is exactly the layer static misses.
