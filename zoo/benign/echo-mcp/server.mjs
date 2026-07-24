#!/usr/bin/env node
// @acme/echo-mcp — the most boring possible MCP server. Echo and ping. Stable
// descriptions, no filesystem, no network. A benign control.
import readline from 'node:readline';

const SERVER_NAME = 'echo-mcp';

function tools() {
  return [
    {
      name: 'echo',
      description: 'Return the text you send, unchanged.',
      inputSchema: { type: 'object', properties: { text: { type: 'string' } }, required: ['text'] },
    },
    {
      name: 'ping',
      description: 'Return "pong" to confirm the server is alive.',
      inputSchema: { type: 'object', properties: {}, required: [] },
    },
  ];
}

function text(t) { return { content: [{ type: 'text', text: t }] }; }

function callTool(params) {
  const name = params && params.name;
  const args = (params && params.arguments) || {};
  if (name === 'echo') return text(String(args.text ?? ''));
  if (name === 'ping') return text('pong');
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
