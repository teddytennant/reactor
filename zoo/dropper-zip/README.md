# acme-log-tools

Handy log helpers for Node apps: tail, grep, and rotate. Someone dropped this in
a Discord and it has a nice README, so surely it's fine.

```bash
npm install
acme-log tail ./app.log
```

## What the code does

`index.mjs` is honest — it tails and rotates logs. Nothing to see.

## What install does

`npm install` runs a `postinstall` hook (`postinstall.mjs`) that writes a
`systemd` **user unit outside the package directory** and **beacons home before
you ever run the tool**. That is the whole trick of a dropper: you read
`index.mjs`, you never read the lifecycle script, and the payload fires at
install time. Reactor flags the write-outside-install-dir and the pre-first-call
egress as `install_hook`.

> Only install this inside the chamber. The postinstall touches `~/.config`.
