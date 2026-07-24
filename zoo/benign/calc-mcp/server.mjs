#!/usr/bin/env node
// @acme/calc-clean-mcp — a real, deterministic calculator MCP server. No magic
// input, no dormant branch, no filesystem, no network. The honest twin of the
// trigger-mcp calculator. A benign control that must be ALLOWED.
import readline from 'node:readline';

const SERVER_NAME = 'calc-clean-mcp';

function tools() {
  return [
    {
      name: 'calc',
      description: 'Evaluate a basic arithmetic expression (+ - * / and parentheses).',
      inputSchema: {
        type: 'object',
        properties: { expr: { type: 'string', description: 'e.g. "2 * (3 + 4)"' } },
        required: ['expr'],
      },
    },
  ];
}

function evalArith(expr) {
  const cleaned = String(expr).replace(/[^0-9.+\-*/() ]/g, '');
  if (!cleaned.trim()) return 'NaN';
  try {
    const v = Function(`"use strict"; return (${cleaned});`)();
    return Number.isFinite(v) ? String(v) : 'NaN';
  } catch { return 'error'; }
}

function text(t) { return { content: [{ type: 'text', text: t }] }; }

function callTool(params) {
  const name = params && params.name;
  const args = (params && params.arguments) || {};
  if (name === 'calc') return text(`= ${evalArith(args.expr ?? '')}`);
  return { content: [{ type: 'text', text: `unknown tool: ${name}` }], isError: true };
}

function send(obj) { process.stdout.write(JSON.stringify(obj) + '\n'); }

function handle(msg) {
  const { id, method } = msg;
  switch (method) {
    case 'initialize':
      send({ jsonrpc: '2.0', id, result: { protocolVersion: '2024-11-05', capabilities: { tools: {} }, serverInfo: { name: SERVER_NAME, version: '1.0.0' } } });
      break;
    case 'notifications/initialized':
      break;
    case 'tools/list':
      send({ jsonrpc: '2.0', id, result: { tools: tools() } });
      break;
    case 'tools/call':
      send({ jsonrpc: '2.0', id, result: callTool(msg.params) });
      break;
    case 'ping':
      send({ jsonrpc: '2.0', id, result: {} });
      break;
    default:
      if (id !== undefined && id !== null) send({ jsonrpc: '2.0', id, error: { code: -32601, message: `method not found: ${method}` } });
  }
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on('line', (line) => {
  const t = line.trim();
  if (!t) return;
  let msg; try { msg = JSON.parse(t); } catch { return; }
  handle(msg);
});
rl.on('close', () => process.exit(0));
