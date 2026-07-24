#!/usr/bin/env python3
"""End-to-end smoke test for artifact ingest (docs/CONTRACT.md "Artifact ingest").

`/api/upload` + `{"upload_id": ...}` is the path a person actually takes with
their own artifact — not one of ours out of the zoo. internal/engine covers the
unpacking in unit tests; this covers the whole chain once, for real: archive in,
staged on the host, copied into a chamber, driven by the victim, verdict out.

    eval/smoke_ingest.py [--engine http://127.0.0.1:8787]

Exits non-zero with a reason on the first thing that does not hold.
"""
import io
import json
import os
import sys
import time
import urllib.error
import urllib.request
import uuid
import zipfile

ENGINE = os.environ.get("REACTOR_ENGINE", "http://127.0.0.1:8787")
for i, a in enumerate(sys.argv):
    if a == "--engine" and i + 1 < len(sys.argv):
        ENGINE = sys.argv[i + 1]

# A minimal, genuinely benign MCP server. Kept inline rather than zipping one of
# the zoo directories so this test fails for its own reasons, not because a
# fixture moved.
SERVER_MJS = r"""
const send = (m) => process.stdout.write(JSON.stringify(m) + "\n");
const TOOLS = [{
  name: "echo",
  description: "Echo a message back to the caller.",
  inputSchema: { type: "object", properties: { text: { type: "string" } } },
}];
let buf = "";
process.stdin.on("data", (d) => {
  buf += d;
  let i;
  while ((i = buf.indexOf("\n")) >= 0) {
    const line = buf.slice(0, i); buf = buf.slice(i + 1);
    if (!line.trim()) continue;
    let m; try { m = JSON.parse(line); } catch { continue; }
    if (m.id === undefined) continue;
    if (m.method === "initialize") {
      send({ jsonrpc: "2.0", id: m.id, result: {
        protocolVersion: "2024-11-05", capabilities: { tools: {} },
        serverInfo: { name: "smoke-echo", version: "1.0.0" } } });
    } else if (m.method === "tools/list") {
      send({ jsonrpc: "2.0", id: m.id, result: { tools: TOOLS } });
    } else if (m.method === "tools/call") {
      const text = (m.params && m.params.arguments && m.params.arguments.text) || "";
      send({ jsonrpc: "2.0", id: m.id, result: { content: [{ type: "text", text: String(text) }] } });
    }
  }
});
"""

PACKAGE_JSON = json.dumps({"name": "smoke-echo-mcp", "version": "1.0.0", "type": "module"}, indent=2)


def die(msg):
    print(f"FAIL: {msg}")
    sys.exit(1)


def api(path, method="GET", body=None):
    req = urllib.request.Request(
        ENGINE + path,
        data=json.dumps(body).encode() if body is not None else None,
        method=method, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=120) as r:
        return json.load(r)


def make_zip():
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("smoke-echo/server.mjs", SERVER_MJS)
        z.writestr("smoke-echo/package.json", PACKAGE_JSON)
        z.writestr("smoke-echo/README.md", "# smoke-echo\n\nEchoes a message.\n")
    return buf.getvalue()


def upload(blob, filename="smoke-echo.zip"):
    """multipart/form-data with one file part, as the console sends it."""
    boundary = "----reactor" + uuid.uuid4().hex
    body = b"".join([
        f'--{boundary}\r\nContent-Disposition: form-data; name="file"; '
        f'filename="{filename}"\r\nContent-Type: application/zip\r\n\r\n'.encode(),
        blob, b"\r\n",
        f'--{boundary}\r\nContent-Disposition: form-data; name="source"\r\n\r\n'.encode(),
        b"node server.mjs\r\n",
        f"--{boundary}--\r\n".encode(),
    ])
    req = urllib.request.Request(
        ENGINE + "/api/upload", data=body, method="POST",
        headers={"Content-Type": f"multipart/form-data; boundary={boundary}"})
    with urllib.request.urlopen(req, timeout=120) as r:
        return json.load(r)


def await_verdict(det_id, timeout_s=420):
    deadline = time.time() + timeout_s
    while time.time() < deadline:
        rep = api(f"/api/detonations/{det_id}")
        if rep.get("verdict"):
            return rep
        if rep.get("error"):
            die(f"detonation failed: {rep['error']}")
        time.sleep(0.5)
    die(f"no verdict for {det_id} within {timeout_s}s")


def main():
    blob = make_zip()

    print(f"1. POST /api/upload ({len(blob)} bytes)")
    up = upload(blob)
    for key in ("upload_id", "sha256", "archive", "kind", "artifact"):
        if not up.get(key):
            die(f"upload response is missing {key}: {up}")
    if up["archive"] != "zip":
        die(f"archive type = {up['archive']!r}, want 'zip' (decided by content, not filename)")
    # server.mjs in the tree means this is an MCP server, not a bare zip.
    if up["kind"] != "mcp_server":
        die(f"kind = {up['kind']!r}, want 'mcp_server' inferred from server.mjs")
    if up["files"] != 3:
        die(f"files = {up['files']}, want the 3 entries in the archive")
    if up["skipped_entries"]:
        die(f"{up['skipped_entries']} entries were refused from a clean archive")
    # The digest is of exactly the bytes we sent, so it is checkable here.
    import hashlib
    want_sha = hashlib.sha256(blob).hexdigest()
    if up["sha256"] != want_sha:
        die(f"sha256 = {up['sha256']}, want {want_sha}")
    # No response may leak where the host staged the bytes.
    blob_str = json.dumps(up)
    for leak in ("/tmp/", "/home/", "/var/folders"):
        if leak in blob_str:
            die(f"upload response leaks a host path ({leak}): {blob_str}")
    print(f"   upload_id={up['upload_id']} kind={up['kind']} sha256={up['sha256'][:12]}…")

    print("2. POST /api/detonate with the upload_id")
    det = api("/api/detonate", "POST", {"upload_id": up["upload_id"], "sessions": 2})
    det_id = det.get("detonation_id") or die(f"no detonation_id: {det}")
    print(f"   detonation_id={det_id}")

    print("3. awaiting the verdict")
    rep = await_verdict(det_id)
    v = rep["verdict"]
    print(f"   {v['label']} · {v.get('family')} · {v.get('severity')}  ({v.get('time_to_verdict_ms')}ms)")

    # An artifact that echoes a string and touches nothing must come out ALLOWED.
    # A false quarantine here is the failure this whole project optimises against.
    if v["label"] != "ALLOWED":
        die(f"a benign uploaded server was quarantined: {v['label']} — {v.get('explanation')}")
    # And the run must have genuinely happened, not been short-circuited.
    if rep.get("sessions") != 2:
        die(f"sessions = {rep.get('sessions')}, want 2")
    wire = [e for e in rep.get("events", []) if e.get("kind") == "wire"]
    if not wire:
        die("no wire events — the artifact was never actually driven in a chamber")
    listed = [e for e in wire if (e.get("wire") or {}).get("method") == "tools/list"]
    if not listed:
        die("the victim never pulled tools/list from the uploaded server")

    print(f"\nPASS: uploaded archive detonated end to end ({len(wire)} wire events, ALLOWED)")


if __name__ == "__main__":
    try:
        main()
    except urllib.error.HTTPError as e:
        die(f"{e.code} {e.reason}: {e.read().decode(errors='replace')[:400]}")
    except urllib.error.URLError as e:
        die(f"engine unreachable at {ENGINE}: {e}")
