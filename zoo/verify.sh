#!/usr/bin/env bash
# Reactor zoo verifier. Proves: every MCP server handshakes; notes-mcp's rug pull
# fires on the 4th tools/list (and only then); benign controls never mutate;
# trigger-mcp's dormant branch fires only on the magic input; index.json is valid
# and aggregates every manifest; plus the sink-based behavioral checks.
#
# Safe to run on a dev box: it NEVER runs the dropper/stealer scripts (those
# write outside the install dir and read real credentials — chamber only).
set -u
ROOT="$(cd "$(dirname "$0")" && pwd)"
PASS=0; FAIL=0
ok()  { echo "  PASS: $1"; PASS=$((PASS+1)); }
bad() { echo "  FAIL: $1"; FAIL=$((FAIL+1)); }

# search_desc: read handshake stdout on stdin, print the `search` tool description.
search_desc() {
  node -e '
    const rl=require("readline").createInterface({input:process.stdin});
    let d="";
    rl.on("line",l=>{try{const m=JSON.parse(l);if(m.result&&m.result.tools){const t=m.result.tools.find(x=>x.name==="search");if(t)d=t.description;}}catch{}});
    rl.on("close",()=>process.stdout.write(d));'
}
INIT='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
INITED='{"jsonrpc":"2.0","method":"notifications/initialized"}'
LIST='{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

echo "== 1. MCP handshake for every server =="
for s in notes-mcp/server.mjs shadow-mcp/server.mjs sleeper-mcp/server.mjs trigger-mcp/server.mjs \
         benign/clean-notes-mcp/server.mjs benign/echo-mcp/server.mjs benign/calc-mcp/server.mjs; do
  out="$(printf '%s\n' "$INIT" "$INITED" "$LIST" | REACTOR_STATE_DIR="$(mktemp -d)" node "$ROOT/$s" 2>/dev/null)"
  if echo "$out" | grep -q '"protocolVersion"' && echo "$out" | grep -q '"tools"'; then ok "handshake $s"; else bad "handshake $s"; fi
done

echo "== 2. notes-mcp rug pull: 4 detonations sharing a state dir =="
STATE="$(mktemp -d)"
declare -a D
for i in 1 2 3 4; do
  D[$i]="$(printf '%s\n' "$INIT" "$INITED" "$LIST" | REACTOR_STATE_DIR="$STATE" node "$ROOT/notes-mcp/server.mjs" 2>/dev/null | search_desc)"
  echo "  serve $i search.description = ${D[$i]}"
done
clean=1; for i in 1 2 3; do echo "${D[$i]}" | grep -q "attach ~/.env" && clean=0; done
[ $clean -eq 1 ] && ok "serves 1-3 clean (no 'attach ~/.env')" || bad "serves 1-3 clean"
echo "${D[4]}" | grep -q "attach ~/.env" && ok "serve 4 poisoned (rug pull fires)" || bad "serve 4 poisoned"

echo "== 3. notes-mcp rug pull: single process, 4 tools/list =="
out="$(printf '%s\n' "$INIT" "$INITED" \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/list"}' \
  '{"jsonrpc":"2.0","id":4,"method":"tools/list"}' \
  '{"jsonrpc":"2.0","id":5,"method":"tools/list"}' \
  | REACTOR_STATE_DIR="$(mktemp -d)" node "$ROOT/notes-mcp/server.mjs" 2>/dev/null)"
n="$(echo "$out" | grep -c 'attach ~/.env')"
[ "$n" = "1" ] && ok "exactly 1 of 4 serves poisoned (the 4th)" || bad "expected 1 poisoned serve, got $n"

echo "== 4. notes-mcp rug pull via REACTOR_SESSION (real-harness path) =="
s1="$(printf '%s\n' "$INIT" "$INITED" "$LIST" | REACTOR_SESSION=1 REACTOR_STATE_DIR="$(mktemp -d)" node "$ROOT/notes-mcp/server.mjs" 2>/dev/null | search_desc)"
s4="$(printf '%s\n' "$INIT" "$INITED" "$LIST" | REACTOR_SESSION=4 REACTOR_STATE_DIR="$(mktemp -d)" node "$ROOT/notes-mcp/server.mjs" 2>/dev/null | search_desc)"
echo "$s1" | grep -q "attach ~/.env" && bad "session 1 should be clean" || ok "session 1 clean"
echo "$s4" | grep -q "attach ~/.env" && ok "session 4 poisoned" || bad "session 4 should be poisoned"

echo "== 5. benign clean-notes-mcp never mutates (6 serves, byte-stable) =="
STATE="$(mktemp -d)"; first=""; stable=1; poison=0
for i in 1 2 3 4 5 6; do
  d="$(printf '%s\n' "$INIT" "$INITED" "$LIST" | REACTOR_STATE_DIR="$STATE" node "$ROOT/benign/clean-notes-mcp/server.mjs" 2>/dev/null | search_desc)"
  [ -z "$first" ] && first="$d"
  [ "$d" != "$first" ] && stable=0
  echo "$d" | grep -q "attach ~/.env" && poison=1
done
[ $stable -eq 1 ] && ok "clean-notes description byte-stable across serves" || bad "clean-notes description changed"
[ $poison -eq 0 ] && ok "clean-notes never poisons" || bad "clean-notes leaked poison"

echo "== 6. trigger-mcp conditional dormant branch (magic input only) =="
THOME="$(mktemp -d)"; mkdir -p "$THOME/.aws"; printf 'BAITCRED-abc123\n' > "$THOME/.aws/credentials"
norm="$(printf '%s\n' "$INIT" "$INITED" '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"calc","arguments":{"expr":"2+2"}}}' | HOME="$THOME" node "$ROOT/trigger-mcp/server.mjs" 2>/dev/null)"
magic="$(printf '%s\n' "$INIT" "$INITED" '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"calc","arguments":{"expr":"31337+1"}}}' | HOME="$THOME" node "$ROOT/trigger-mcp/server.mjs" 2>/dev/null)"
echo "$norm"  | grep -q "BAITCRED-abc123" && bad "normal input leaked creds" || ok "normal input clean (= 4, no leak)"
echo "$magic" | grep -q "BAITCRED-abc123" && ok "magic input leaks credential bait" || bad "magic input did not trigger"

echo "== 7. index.json valid + aggregates every manifest =="
node -e '
  const fs=require("fs"),path=require("path");
  const root=process.argv[1];
  const idx=JSON.parse(fs.readFileSync(path.join(root,"index.json"),"utf8"));
  const manifests=[];
  const walk=(d)=>{for(const e of fs.readdirSync(d,{withFileTypes:true})){const p=path.join(d,e.name);
    if(e.isDirectory())walk(p); else if(e.name==="reactor.json")manifests.push(p);}};
  walk(root);
  const idxIds=new Set(idx.map(a=>a.id));
  const manIds=manifests.map(m=>JSON.parse(fs.readFileSync(m,"utf8")).id);
  let missing=manIds.filter(id=>!idxIds.has(id));
  if(idx.length<manifests.length){console.error("index has fewer entries than manifests");process.exit(1);}
  if(missing.length){console.error("index missing: "+missing.join(","));process.exit(1);}
  console.log("  index.json entries: "+idx.length+", reactor.json files: "+manifests.length);
' "$ROOT" && ok "index.json valid and complete" || bad "index.json invalid or incomplete"

echo "== 8. behavioral (sink) checks =="
node "$ROOT/verify_behavior.mjs" && ok "behavioral checks passed" || bad "behavioral checks failed"

echo
echo "== TOTAL: $PASS passed, $FAIL failed =="
[ $FAIL -eq 0 ] || exit 1
