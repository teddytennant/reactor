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
    <div className="flex min-h-screen flex-col bg-bg">
      <TopBar />
      <main className="console-veil flex-1 px-4 py-6 sm:px-6 sm:py-8">
        <Scorecard data={data} live={live} />
      </main>
    </div>
  );
}
