// Run with: npm test  (node --experimental-strip-types --test)
//
// Engine-origin resolution decides whether the console talks to the visitor's
// own machine, a shared engine, or nothing at all. Getting it wrong on the
// deployed static export means every visitor silently falls to replay — which
// looks identical to "no engine running", so it would not be noticed.

import assert from "node:assert/strict";
import { test } from "node:test";

import {
  DEFAULT_LOCAL_ENGINE,
  isLoopbackEngine,
  isWebKit,
  normalizeOrigin,
} from "./engine.ts";

test("normalizeOrigin trims whitespace and trailing slashes", () => {
  assert.equal(normalizeOrigin("  http://127.0.0.1:8787/  "), "http://127.0.0.1:8787");
  assert.equal(normalizeOrigin("https://engine.example.com///"), "https://engine.example.com");
  assert.equal(normalizeOrigin("http://127.0.0.1:8787"), "http://127.0.0.1:8787");
  // Empty stays empty — that is the "same-origin" signal, not a missing value.
  assert.equal(normalizeOrigin(""), "");
  assert.equal(normalizeOrigin("   "), "");
});

test("the default local engine matches the engine's own listen address", () => {
  // cmd/reactor defaults to 127.0.0.1:8787; if one moves the other must too.
  assert.equal(DEFAULT_LOCAL_ENGINE, "http://127.0.0.1:8787");
});

// Loopback detection drives the "why is this unreachable" message. A false
// positive tells someone to switch browsers when their engine is simply down.
test("isLoopbackEngine recognises plain-http loopback only", () => {
  for (const yes of [
    "http://127.0.0.1:8787",
    "http://localhost:8787",
    "http://localhost",
    "http://[::1]:8787",
  ]) {
    assert.equal(isLoopbackEngine(yes), true, yes);
  }
  for (const no of [
    "https://127.0.0.1:8787", // https loopback is not mixed content
    "http://192.168.1.50:8787", // LAN, not loopback
    "http://engine.example.com",
    "https://engine.example.com",
    "", // same-origin
    "not a url",
  ]) {
    assert.equal(isLoopbackEngine(no), false, no);
  }
});

// WebKit is the one engine we state definitively cannot reach a loopback engine
// from an https page. Everything else is left to the runtime probe, so a false
// positive here would wrongly tell a Chrome user to switch browsers.
test("isWebKit identifies Safari and every iOS browser", () => {
  const webkit = [
    // macOS Safari
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
    // iOS Safari
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
    // Chrome and Firefox on iOS are WebKit underneath — same block applies.
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/124.0 Mobile/15E148 Safari/604.1",
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/125.0 Mobile/15E148 Safari/605.1.15",
    "Mozilla/5.0 (iPad; CPU OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) EdgiOS/124.0 Mobile/15E148 Safari/605.1.15",
  ];
  const notWebKit = [
    // Desktop Chromium browsers all carry "Safari" in the UA — the reason this
    // cannot be a naive /Safari/ test.
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 OPR/110.0.0.0",
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Brave/124",
    "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36",
    // Desktop Firefox: not WebKit. Whether it reaches a loopback engine is left
    // to the probe rather than asserted here.
    "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0",
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:125.0) Gecko/20100101 Firefox/125.0",
  ];

  for (const ua of webkit) assert.equal(isWebKit(ua), true, ua);
  for (const ua of notWebKit) assert.equal(isWebKit(ua), false, ua);
  assert.equal(isWebKit(""), false);
});
