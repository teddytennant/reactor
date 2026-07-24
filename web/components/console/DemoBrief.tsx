"use client";

import { useEffect, useState } from "react";
import { ArrowRight } from "lucide-react";
import { SCORECARD_FIXTURE, loadScorecard, type Scorecard } from "@/lib/scorecard";
import { SectionLabel } from "@/components/ui";

/**
 * The idle console's right-hand column.
 *
 * The intake panel is a form with a comfortable measure, so on a wide screen it
 * left roughly half the panel empty — the first thing anyone saw was a void.
 * This fills it with the only thing worth putting next to an unproven claim:
 * the measured one. Three numbers off the offline eval, then the sentence they
 * exist to support.
 *
 * Live scorecard when an engine answers, the bundled fixture otherwise — the
 * same contract the /scorecard page uses, so the two can never disagree.
 */
export function DemoBrief() {
  const [data, setData] = useState<Scorecard>(SCORECARD_FIXTURE);
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

  const blind = data.static_blind.static_blind_catches;

  return (
    <aside className="flex flex-col gap-4 rounded-2xl bg-surface-2/40 p-5">
      <SectionLabel
        right={
          <span className="telemetry whitespace-nowrap">{live ? "Live eval" : "Bundled eval"}</span>
        }
      >
        Measured over the zoo
      </SectionLabel>

      <dl className="flex flex-col">
        <Stat
          value={`${data.detection.caught}/${data.detection.total}`}
          label="Malicious artifacts caught"
        />
        <Stat
          value={`${data.false_quarantine.blocked}/${data.false_quarantine.total}`}
          label="Real servers falsely blocked"
        />
        <Stat value={String(blind)} label="Catches with no static signature" />
      </dl>

      <p className="text-sm leading-relaxed text-muted">
        <span className="text-fg">
          {blind} of those catches are things a description scanner cannot see.
        </span>{" "}
        Not doesn&rsquo;t — can&rsquo;t. A rug pull needs repetition, a prompt canary was never a
        file, and an install hook runs before anything reads a tool description.
      </p>

      <a
        href="/scorecard"
        className="focus-ring group inline-flex items-center gap-1.5 self-start rounded-lg text-sm font-medium text-fg"
      >
        See the full scorecard
        <ArrowRight
          size={14}
          aria-hidden="true"
          className="text-faint transition-transform duration-200 ease-instrument group-hover:translate-x-0.5"
        />
      </a>
    </aside>
  );
}

/** One measured figure: the number carries it, the label explains it. */
function Stat({ value, label }: { value: string; label: string }) {
  return (
    <div className="hairline flex items-baseline gap-3 py-2.5 first:border-t-0 first:pt-0">
      <dt className="sr-only">{label}</dt>
      <dd className="flex min-w-0 flex-1 items-baseline gap-3">
        <span className="tnum min-w-[3.5rem] shrink-0 font-mono text-2xl font-semibold leading-none tracking-tight text-fg">
          {value}
        </span>
        <span className="min-w-0 text-sm leading-snug text-muted">{label}</span>
      </dd>
    </div>
  );
}
