// Bundled fixtures — the replay/demo backbone (DEMO.md §7 backup). These make
// the whole UI demoable with zero backend and are authored to match the frozen
// event schema exactly (see fixtures/generate.mjs, which computes real sha256
// digests and IDGen evidence ids).

import type { Artifact, ReactorEvent, ScanResult } from "./events";
import notesRaw from "@/fixtures/notes-mcp-detonation.json";
import benignRaw from "@/fixtures/benign-detonation.json";
import artifactsRaw from "@/fixtures/artifacts.json";
import scanRaw from "@/fixtures/scan-clean.json";

export const NOTES_MONEY_SHOT = notesRaw as unknown as ReactorEvent[];
export const BENIGN_RUN = benignRaw as unknown as ReactorEvent[];
export const FIXTURE_ARTIFACTS = artifactsRaw as unknown as Artifact[];
export const SCAN_CLEAN = scanRaw as unknown as ScanResult;

// The star of the demo: the poisoned notes server, which passes a real
// mcp-scan and rug-pulls on detonation 4. The engine names it `art_notes_mcp`
// and the bundled fixture names it `art_notes` — the console has to arm the
// same artifact either way, so both ids are canonical here rather than spelled
// out at each call site. `@acme/clean-notes-mcp` is its *honest twin* and must
// never match: substring tests on the name pick the wrong server.
export const LEAD_ARTIFACT_IDS: readonly string[] = ["art_notes_mcp", "art_notes"];

/** Is this the demo's lead artifact — the one the console arms by default? */
export function isLeadArtifact(a: Artifact | null): boolean {
  return !!a && (LEAD_ARTIFACT_IDS.includes(a.id) || a.name === "@acme/notes-mcp");
}

/**
 * Pick the artifact the console opens on: the money shot when it is in the
 * list, otherwise whatever came first. Without this a live engine — whose ids
 * differ from the fixtures' — silently armed the alphabetically-first benign
 * calculator, and the demo opened on the boring one.
 */
export function defaultArtifact(artifacts: Artifact[]): Artifact | null {
  return artifacts.find(isLeadArtifact) ?? artifacts[0] ?? null;
}

// The console-replayable artifacts (SPEC §10: authored on stage, real corpus
// offline). Others in the picker are the offline zoo. Both id vocabularies
// again: `art_fs`/`art_pin_filesystem` are the same clean third-party control.
export const REPLAYABLE_IDS: ReadonlySet<string> = new Set([
  "art_notes",
  "art_notes_mcp",
  "art_fs",
  "art_pin_filesystem",
]);

/** Pick the fixture stream that matches a selected artifact in replay mode. */
export function fixtureFor(artifact: Artifact | null): ReactorEvent[] {
  if (!artifact) return NOTES_MONEY_SHOT;
  if (artifact.id === "art_fs" || artifact.id === "art_pin_filesystem") return BENIGN_RUN;
  // Benign artifacts play the ALLOWED run. The bundled fixtures leave `label`
  // empty for those; the engine spells it "benign".
  if (!artifact.label || artifact.label === "benign") return BENIGN_RUN;
  return NOTES_MONEY_SHOT;
}
