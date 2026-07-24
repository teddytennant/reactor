// Presentation metadata for oracle signals (SPEC §4.4). Maps each signal.type
// to a human label, a short "what it means" gloss, and a tone. Tone drives
// color: red is reserved for fired malicious signals and the BLOCKED verdict,
// so every attack signal reads `danger`; benign_profile is the only `success`.

import type { Severity, SignalType } from "./events";

export type Tone = "danger" | "warning" | "success" | "neutral";

export interface SignalMeta {
  label: string;
  gloss: string;
  tone: Tone;
}

// The four flagship static-blind types a description scanner provably cannot
// produce (SPEC §4.4). CONTRACT.md also marks task_deviation + sleeper_beacon
// static-blind; the badge is driven off Signal.static_blind, not this set — this
// list is only for the scorecard's "flagship" framing.
export const FLAGSHIP_STATIC_BLIND: ReadonlySet<string> = new Set([
  "rug_pull",
  "conditional_trigger",
  "context_exfil",
  "install_hook",
]);

const META: Record<string, SignalMeta> = {
  context_exfil: {
    label: "Context exfiltration",
    gloss: "A system-prompt canary reached the sink — a secret that lived only in the agent's head.",
    tone: "danger",
  },
  canary_exfil: {
    label: "Credential exfiltration",
    gloss: "A file-bait canary reached the egress sink.",
    tone: "danger",
  },
  canary_read: {
    label: "Bait credential read",
    gloss: "The artifact opened a bait credential path.",
    tone: "warning",
  },
  task_deviation: {
    label: "Task deviation",
    gloss: "The victim called a tool with no causal link to its assigned task.",
    tone: "danger",
  },
  rug_pull: {
    label: "Rug pull",
    gloss: "A tool description changed bytes between detonations.",
    tone: "danger",
  },
  conditional_trigger: {
    label: "Conditional trigger",
    gloss: "Behavior differed across detonations under varied inputs.",
    tone: "danger",
  },
  shadowing: {
    label: "Cross-server shadowing",
    gloss: "A description referenced or redefined another server's tool.",
    tone: "danger",
  },
  install_hook: {
    label: "Install hook",
    gloss: "A write outside the install dir, or egress, before the first tool call.",
    tone: "danger",
  },
  analyst_injection: {
    label: "Analyst injection attempt",
    gloss: "Artifact text addressed the analyzer — logged as a finding, never obeyed.",
    tone: "danger",
  },
  sleeper_beacon: {
    label: "Sleeper beacon",
    gloss: "Egress only after a delay or N calls.",
    tone: "danger",
  },
  benign_profile: {
    label: "Benign profile",
    gloss: "Writes confined to the install dir, no bait touched, zero deviation.",
    tone: "success",
  },
};

export function signalMeta(type: SignalType | string): SignalMeta {
  return (
    META[type] ?? {
      label: type,
      gloss: "Oracle signal.",
      tone: "danger",
    }
  );
}

/** Tone for a severity, used by verdict and severity chips. */
export function severityTone(sev: Severity | string): Tone {
  switch (sev) {
    case "critical":
    case "high":
      return "danger";
    case "medium":
      return "warning";
    case "none":
      return "success";
    default:
      return "neutral";
  }
}

/** Human family label, e.g. "supply-chain" -> "Supply chain". */
export function familyLabel(family: string): string {
  return family
    .split("-")
    .map((w) => (w ? w[0].toUpperCase() + w.slice(1) : w))
    .join(" ");
}
