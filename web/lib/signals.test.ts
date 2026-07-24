// Run with: npm test  (node --experimental-strip-types --test)

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

import { FLAGSHIP_STATIC_BLIND, familyLabel, severityTone, signalMeta } from "./signals.ts";

const REPO = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

/** The oracle signal types the Go engine can actually emit (SPEC §4.4). */
function goSignalTypes(): string[] {
  const src = readFileSync(join(REPO, "internal", "events", "events.go"), "utf8");
  return [...src.matchAll(/^\tSig[A-Za-z]+\s*=\s*"([a-z_]+)"$/gm)].map((m) => m[1]);
}

// The engine and the console are separate languages with one shared vocabulary.
// A signal type added in Go and not here renders as a raw slug like
// "conditional_trigger" in the middle of the demo.
test("every signal type the engine can emit has console presentation metadata", () => {
  const types = goSignalTypes();
  assert.ok(types.length >= 11, `only found ${types.length} Sig* constants — the regex has drifted`);

  const missing = types.filter((t) => signalMeta(t).label === t);
  assert.deepEqual(missing, [], "signal types with no entry in signals.ts META");

  for (const t of types) {
    const meta = signalMeta(t);
    assert.ok(meta.gloss.length > 20, `${t} needs a real "what it means" gloss, got ${JSON.stringify(meta.gloss)}`);
    assert.notEqual(meta.gloss, "Oracle signal.", `${t} fell through to the default meta`);
  }
});

// Colour carries meaning in this UI: red is reserved for fired malicious
// signals and the BLOCKED verdict, so benign_profile must be the only success
// tone and nothing malicious may be neutral.
test("tones separate the benign profile from every attack signal", () => {
  const types = goSignalTypes();
  const success = types.filter((t) => signalMeta(t).tone === "success");
  assert.deepEqual(success, ["benign_profile"]);

  for (const t of types) {
    if (t === "benign_profile") continue;
    assert.notEqual(signalMeta(t).tone, "neutral", `${t} must read as an attack signal, not neutral`);
    assert.notEqual(signalMeta(t).tone, "success", `${t} must not read as benign`);
  }
});

// FLAGSHIP_STATIC_BLIND is scorecard framing only — the per-signal badge is
// driven off Signal.static_blind. It still has to name signals that exist.
test("the flagship static-blind set only names real signal types", () => {
  const types = new Set(goSignalTypes());
  for (const t of FLAGSHIP_STATIC_BLIND) {
    assert.ok(types.has(t), `flagship set names "${t}", which the engine never emits`);
  }
});

test("an unknown signal type degrades to a readable fallback instead of throwing", () => {
  const meta = signalMeta("some_future_oracle");
  assert.equal(meta.label, "some_future_oracle");
  assert.equal(meta.tone, "danger", "an unrecognised signal must not be shown as safe");
});

test("severityTone never reads a high severity as safe", () => {
  assert.equal(severityTone("critical"), "danger");
  assert.equal(severityTone("high"), "danger");
  assert.equal(severityTone("medium"), "warning");
  assert.equal(severityTone("none"), "success");
  assert.equal(severityTone("low"), "neutral");
  assert.equal(severityTone("nonsense"), "neutral");
});

test("familyLabel humanises the engine's hyphenated families", () => {
  assert.equal(familyLabel("supply-chain"), "Supply Chain");
  assert.equal(familyLabel("cross-server"), "Cross Server");
  assert.equal(familyLabel("c2"), "C2");
  assert.equal(familyLabel("benign"), "Benign");
  assert.equal(familyLabel(""), "");
});
