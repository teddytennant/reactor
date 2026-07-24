// Scorecard schema (SPEC §7). The engine's eval/ runner emits this JSON on
// GET /api/scorecard; the UI falls back to the bundled fixture when the engine
// is unreachable so the metrics slide is always presentable.

import scorecardRaw from "@/fixtures/scorecard.json";
import { getScorecard } from "./api";

export interface Scorecard {
  generated_at: string;
  zoo: { malicious: number; benign: number };
  detection: { caught: number; total: number; rate: number };
  false_quarantine: { blocked: number; total: number; rate: number };
  static_blind: {
    reactor_caught: number;
    static_caught: number;
    static_blind_catches: number;
    missed_by_both?: number;
    by_type: { type: string; reactor: number; static: number }[];
  };
  redteam: { mutations: number; escaped: number; rate: number };
  time_to_verdict_ms: { mean: number; p50: number; p95: number };
  cost_usd: { mean: number; total: number };
  comparison: {
    static: { name: string; caught: number; blind: number; false_blocks: number };
    reactor: { name: string; caught: number; false_blocks: number };
  };
}

export const SCORECARD_FIXTURE = scorecardRaw as unknown as Scorecard;

/** Live scorecard if the engine answers, else the bundled fixture. */
export async function loadScorecard(): Promise<{ data: Scorecard; live: boolean }> {
  const live = await getScorecard<Scorecard>();
  if (live && live.detection) return { data: live, live: true };
  return { data: SCORECARD_FIXTURE, live: false };
}
