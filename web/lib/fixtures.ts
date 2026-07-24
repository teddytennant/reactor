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

// The two console-replayable artifacts (SPEC §10: authored on stage, real
// corpus offline). Others in the picker are the offline zoo.
export const REPLAYABLE_IDS: ReadonlySet<string> = new Set(["art_notes", "art_fs"]);

/** Pick the fixture stream that matches a selected artifact in replay mode. */
export function fixtureFor(artifact: Artifact | null): ReactorEvent[] {
  if (!artifact) return NOTES_MONEY_SHOT;
  if (artifact.id === "art_fs") return BENIGN_RUN;
  // Benign artifacts (no ground-truth label) play the ALLOWED run.
  if (!artifact.label) return BENIGN_RUN;
  return NOTES_MONEY_SHOT;
}
