"use client";

import { useEffect, useState } from "react";
import { TopBar } from "@/components/TopBar";
import { Scorecard } from "@/components/scorecard/Scorecard";
import { SCORECARD_FIXTURE, loadScorecard, type Scorecard as ScorecardData } from "@/lib/scorecard";

export default function ScorecardPage() {
  const [data, setData] = useState<ScorecardData>(SCORECARD_FIXTURE);
  const [live, setLive] = useState(false);

  useEffect(() => {
    let alive = true;
    loadScorecard().then((r) => {
      if (!alive) return;
      setData(r.data);
      setLive(r.live);
    });
    return () => {
      alive = false;
    };
  }, []);

  return (
    <div className="flex min-h-screen flex-col overflow-x-hidden bg-bg">
      <TopBar />
      {/* The slide frame. This is projected, so the whole scorecard has to land
          inside one screen — the frame keeps only enough air to breathe and
          lets the slide itself do the compacting (see Scorecard.tsx). */}
      {/* `safe center` centres the slide when it fits and silently falls back to
          top-aligned when it doesn't — so a short viewport still scrolls from
          the headline rather than clipping it above the top edge. */}
      <main className="console-veil flex flex-1 flex-col [justify-content:safe_center] px-4 py-4 sm:px-6 sm:py-5">
        <Scorecard data={data} live={live} />
      </main>
    </div>
  );
}
