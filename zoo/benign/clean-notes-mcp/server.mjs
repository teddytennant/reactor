#!/usr/bin/env node
// @acme/clean-notes-mcp — the honest control. Structurally a twin of notes-mcp
// (same three tools, same shape) but it never mutates a description, never reads
// a path it wasn't given a note id for, and never touches the network. It must
// come out ALLOWED: the proof that Reactor is a gate, not a scare machine.
import readline from 'node:readline';

const SERVER_NAME = 'clean-notes-mcp';

const NOTES = {
  n_18: { title: 'Standup', body: 'Blocked on migrations; notesd deploy Thursday.' },
  n_24: { title: 'Roadmap', body: 'Q3: tagging, full-text search, share links.' },
  n_31: { title: 'Oncall', body: 'Rotate weekly; pager runbook in the wiki.' },
};

// Descriptions are constants. They are byte-identical on every tools/list, in
// every session, forever. That stability is the whole point of this control.
function tools() {
  return [
    {
      name: 'search',
      description: 'Search notes by keyword and return matching note ids with snippet previews.',
      inputSchema: {
        type: 'object',
        properties: {
          q: { type: 'string', description: 'Keyword to search for.' },
          limit: { type: 'number', description: 'Max results (default 5).' },
        },
        required: ['q'],
      },
    },
    {
      name: 'fetch',
      description: 'Fetch a single note by its id and return the full note body.',
      inputSchema: { type: 'object', properties: { id: { type: 'string' } }, required: ['id'] },
    },
    {
      name: 'notes',
      description: 'List all note ids with their titles.',
      inputSchema: { type: 'object', properties: {}, required: [] },
    },
  ];
}

function text(t) { return { content: [{ type: 'text', text: t }] }; }

function callTool(params) {
  const name = params && params.name;
  const args = (params && params.arguments) || {};
  if (name === 'search') {
    const q = args.q || args.query || '';
    const hits = Object.entries(NOTES)
      .filter(([, v]) => !q || (v.title + ' ' + v.body).toLowerCase().includes(String(q).toLowerCase()))
      .map(([id, v]) => `[${id}] ${v.title}`);
    return text(`Found ${hits.length} note(s) for "${q}": ${hits.join(', ') || 'none'}.`);
  }
  if (name === 'fetch') {
    const note = NOTES[args.id];
    return note ? text(`${args.id}: ${note.title}\n${note.body}`) : text(`No note ${args.id}.`);
  }
  if (name === 'notes') {
    return text(Object.entries(NOTES).map(([id, v]) => `${id}: ${v.title}`).join('\n'));
  }
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
