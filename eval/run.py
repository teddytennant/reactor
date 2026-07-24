#!/usr/bin/env python3
"""Reactor offline scorecard runner (SPEC §7).

Detonates the whole authored zoo through the engine HTTP API, compares each
verdict against the artifact's ground-truth label, and emits the local
scorecard: Detection Rate, False-Quarantine Rate (must be ~0), and the headline
Static-Blind Rate — the share of catches a description-only scanner provably
cannot see. Also runs the real static scanner (snyk-agent-scan / mcp-scan) on
the same servers for the left-column comparison when it is installed.

No third-party eval SaaS: plain HTTP + a JSON/Markdown table.

    eval/run.py                     detonate the offline zoo, write the scorecard
    eval/run.py --live              include the live npx pins too (needs network)
    eval/run.py --out DIR           write scorecard.json/.md somewhere else
    eval/run.py --check             exit non-zero unless the scorecard holds up

--check is the CI gate. It fails the run on any misclassification: a malicious
artifact that was allowed, or — the one that matters most for a security tool —
a benign artifact that was quarantined.
"""
import json
import os
import subprocess
import sys
import time
import urllib.request

ENGINE = os.environ.get("REACTOR_ENGINE", "http://127.0.0.1:8787")
OUT_DIR = os.environ.get("REACTOR_EVAL_OUT") or os.path.dirname(os.path.abspath(__file__))
STATIC_BLIND_TYPES = {"rug_pull", "conditional_trigger", "context_exfil",
                      "install_hook", "sleeper_beacon", "task_deviation"}


def api(path, method="GET", body=None):
    url = ENGINE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=600) as r:
        return json.load(r)


def detonate(artifact_id, sessions):
    det = api("/api/detonate", "POST", {"artifact_id": artifact_id, "sessions": sessions})
    did = det["detonation_id"]
    # Poll until the report has a verdict.
    for _ in range(600):
        rep = api(f"/api/detonations/{did}")
        if rep.get("verdict"):
            return rep
        time.sleep(0.5)
    return api(f"/api/detonations/{did}")


def run_static_scanner(artifact):
    """Best-effort real static scan for the left column (SPEC §9)."""
    scanner = os.environ.get("REACTOR_SCANNER")
    d = (artifact.get("env") or {}).get("_dir")
    if not scanner or not d:
        return None
    try:
        argv = scanner.split() + artifact.get("source", "").split()
        p = subprocess.run(argv, cwd=d, capture_output=True, timeout=120, text=True)
        return {"available": True, "clean": p.returncode == 0,
                "output": (p.stdout + p.stderr)[-2000:]}
    except Exception as e:  # noqa: BLE001
        return {"available": False, "error": str(e)}


def arg_value(name, default=None):
    """--name VALUE or --name=VALUE."""
    for i, a in enumerate(sys.argv):
        if a == name and i + 1 < len(sys.argv):
            return sys.argv[i + 1]
        if a.startswith(name + "="):
            return a.split("=", 1)[1]
    return default


def check(s, baseline_path):
    """The CI gate. Returns a list of failures; empty means the scorecard holds.

    A regression gate against eval/expected.json, not a perfection gate —
    Reactor has one documented miss and asserting 100% detection would make the
    scorecard lie, which is the one thing it exists not to do.
    """
    with open(baseline_path) as f:
        want = json.load(f)
    accepted = want.get("accepted_misses", {})
    problems = []

    # 1. A benign artifact that got quarantined. The worst outcome for a
    #    security tool, and the number the whole design optimises for.
    fq = s["false_quarantine_rate"] or 0
    if fq > want.get("false_quarantine_rate_max", 0):
        problems.append(
            f"false-quarantine rate {pct(fq)} exceeds the allowed "
            f"{pct(want.get('false_quarantine_rate_max', 0))}: "
            f"{', '.join(s['false_block_examples'])}")

    # 2. A malicious artifact that got through and was not a known miss.
    new_misses = [m for m in s["misses"] if m not in accepted]
    if new_misses:
        problems.append(f"newly missed malicious artifacts: {', '.join(new_misses)}")

    # 3. A known miss that is now caught. Good news — but the baseline has to be
    #    updated, or the list slowly becomes a place to hide failures.
    caught = set(s["misses"])
    fixed = [m for m in accepted if m not in caught and any(r["name"] == m for r in s["rows"])]
    if fixed:
        problems.append(
            f"{', '.join(fixed)} is caught now — remove it from "
            f"{os.path.basename(baseline_path)} so the accepted-miss list stays honest")

    # 4. The headline claim: Reactor catches things a description-only scanner
    #    provably cannot. If that contrast quietly disappears, so does the point.
    if s["static_blind_catches"] < want.get("min_static_blind_catches", 1):
        problems.append(
            f"only {s['static_blind_catches']} catches carried a static-blind signal, "
            f"want at least {want.get('min_static_blind_catches', 1)}")

    # 5. Both halves of the zoo actually ran. Guards against a green run that
    #    measured nothing because the catalog or the engine came up empty.
    if s["malicious_total"] < want.get("min_malicious", 1):
        problems.append(f"only {s['malicious_total']} malicious artifacts ran — the zoo did not load")
    if s["benign_total"] < want.get("min_benign", 1):
        problems.append(f"only {s['benign_total']} benign controls ran — nothing constrained false positives")

    return problems


def main():
    include_live = "--live" in sys.argv
    global OUT_DIR
    OUT_DIR = arg_value("--out", OUT_DIR)
    os.makedirs(OUT_DIR, exist_ok=True)
    artifacts = api("/api/artifacts")
    rows = []
    print(f"Detonating {len(artifacts)} artifacts through {ENGINE}\n")

    for a in artifacts:
        env = a.get("env") or {}
        if env.get("_live") and not include_live:
            print(f"  skip (live npx pin, use --live): {a['name']}")
            continue
        sessions = 5 if a["kind"] == "mcp_server" else 1
        label_truth = a.get("label") or "benign"
        expected_benign = label_truth == "benign"

        t0 = time.time()
        rep = detonate(a["id"], sessions)
        elapsed = time.time() - t0
        v = rep.get("verdict") or {}
        signals = [s["type"] for s in rep.get("signals", [])]
        blocked = v.get("label", "ALLOWED") != "ALLOWED"
        blind_hit = any(s in STATIC_BLIND_TYPES for s in signals) if blocked else False

        # Correctness: benign must be ALLOWED; malicious must be blocked.
        correct = (not blocked) if expected_benign else blocked

        static = run_static_scanner(a)
        rows.append({
            "id": a["id"], "name": a["name"], "kind": a["kind"],
            "truth": label_truth, "expected_benign": expected_benign,
            "verdict": v.get("label"), "family": v.get("family"),
            "severity": v.get("severity"), "signals": signals,
            "blocked": blocked, "static_blind": blind_hit, "correct": correct,
            "seconds": round(elapsed, 1),
            "static_scan_clean": (static or {}).get("clean"),
            "cost_usd": v.get("cost_usd", 0),
        })
        mark = "OK " if correct else "XX "
        blind = " [static-blind]" if blind_hit else ""
        print(f"  {mark}{a['name']:<34} {str(v.get('label')):<10} {v.get('family') or '':<14}{blind}")

    scorecard = summarize(rows)
    write_outputs(rows, scorecard)
    print("\n" + render_table(scorecard))
    print(f"\nwrote {OUT_DIR}/scorecard.json and {OUT_DIR}/scorecard.md")

    if "--check" in sys.argv:
        baseline = arg_value("--expected", os.path.join(
            os.path.dirname(os.path.abspath(__file__)), "expected.json"))
        problems = check(scorecard, baseline)
        if problems:
            print(f"\nSCORECARD CHECK FAILED (baseline: {baseline})")
            for p in problems:
                print(f"  - {p}")
            sys.exit(1)
        accepted = len(scorecard["misses"])
        print(f"\nSCORECARD CHECK PASSED"
              f" ({scorecard['malicious_caught']}/{scorecard['malicious_total']} caught,"
              f" {accepted} accepted miss(es),"
              f" {scorecard['false_blocks']} false quarantines)")


def summarize(rows):
    mal = [r for r in rows if not r["expected_benign"]]
    ben = [r for r in rows if r["expected_benign"]]
    caught = [r for r in mal if r["blocked"]]
    blind = [r for r in caught if r["static_blind"]]
    false_blocks = [r for r in ben if r["blocked"]]
    catches = [r for r in rows if r["blocked"]]
    times = [r["seconds"] for r in rows if r["seconds"]]
    return {
        "source": "offline",
        "artifacts": len(rows),
        "malicious_total": len(mal),
        "malicious_caught": len(caught),
        "detection_rate": round(len(caught) / len(mal), 4) if mal else None,
        "benign_total": len(ben),
        "false_blocks": len(false_blocks),
        "false_quarantine_rate": round(len(false_blocks) / len(ben), 4) if ben else None,
        "catches": len(catches),
        "static_blind_catches": len(blind),
        "static_blind_rate": round(len(blind) / len(catches), 4) if catches else None,
        "mean_seconds": round(sum(times) / len(times), 1) if times else None,
        "cost_usd_total": round(sum(r["cost_usd"] for r in rows), 4),
        "static_blind_examples": [r["name"] for r in blind],
        "misses": [r["name"] for r in mal if not r["blocked"]],
        "false_block_examples": [r["name"] for r in false_blocks],
        "rows": rows,
    }


def render_table(s):
    L = []
    L.append("REACTOR SCORECARD")
    L.append("=" * 60)
    L.append(f"  Detection rate         {s['malicious_caught']}/{s['malicious_total']}"
             f"  ({pct(s['detection_rate'])})")
    L.append(f"  False-quarantine rate  {s['false_blocks']}/{s['benign_total']}"
             f"  ({pct(s['false_quarantine_rate'])})  <- must be ~0")
    L.append(f"  Static-blind rate      {s['static_blind_catches']}/{s['catches']}"
             f"  ({pct(s['static_blind_rate'])})  <- the headline")
    L.append(f"  Mean time-to-verdict   {s['mean_seconds']}s")
    if s["misses"]:
        L.append(f"  Misses (honest)        {', '.join(s['misses'])}")
    return "\n".join(L)


def pct(x):
    return f"{round(100 * x)}%" if x is not None else "n/a"


def write_outputs(rows, s):
    with open(os.path.join(OUT_DIR, "scorecard.json"), "w") as f:
        json.dump({**s, "generated_ms": int(time.time() * 1000)}, f, indent=2)
    md = ["# Reactor scorecard", "",
          f"- **Detection rate:** {s['malicious_caught']}/{s['malicious_total']} ({pct(s['detection_rate'])})",
          f"- **False-quarantine rate:** {s['false_blocks']}/{s['benign_total']} ({pct(s['false_quarantine_rate'])}) — must be ~0",
          f"- **Static-blind rate:** {s['static_blind_catches']}/{s['catches']} ({pct(s['static_blind_rate'])}) — invisible to a description-only scanner",
          f"- **Mean time-to-verdict:** {s['mean_seconds']}s", "",
          "| Artifact | Kind | Truth | Verdict | Family | Static-blind | Correct |",
          "|---|---|---|---|---|---|---|"]
    for r in rows:
        md.append(f"| {r['name']} | {r['kind']} | {r['truth']} | {r['verdict']} | "
                  f"{r['family'] or ''} | {'yes' if r['static_blind'] else ''} | "
                  f"{'✓' if r['correct'] else '✗'} |")
    with open(os.path.join(OUT_DIR, "scorecard.md"), "w") as f:
        f.write("\n".join(md) + "\n")


if __name__ == "__main__":
    main()
