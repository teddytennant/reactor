"use client";

import { Boxes, FileArchive, Puzzle, Server, Star } from "lucide-react";
import { cn } from "@/lib/cn";
import type { Artifact } from "@/lib/events";
import { Chip } from "@/components/ui";

const KIND_META: Record<string, { label: string; Icon: typeof Server }> = {
  mcp_server: { label: "MCP server", Icon: Server },
  skill: { label: "Skill", Icon: Puzzle },
  zip: { label: "Vibe-coded zip", Icon: FileArchive },
};

function kindMeta(kind: string) {
  return KIND_META[kind] ?? { label: kind, Icon: Boxes };
}

export function ArtifactPicker({
  artifacts,
  selectedId,
  onSelect,
  isReplayable,
  showOfflineTag,
  disabled,
}: {
  artifacts: Artifact[];
  selectedId: string | null;
  onSelect: (a: Artifact) => void;
  isReplayable: (a: Artifact) => boolean;
  showOfflineTag: boolean;
  disabled: boolean;
}) {
  return (
    <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
      {artifacts.map((a) => {
        const featured = a.id === "art_notes" || a.name.includes("notes-mcp");
        const { label, Icon } = kindMeta(a.kind);
        const offline = showOfflineTag && !isReplayable(a);
        const selectable = !disabled && (isReplayable(a) || !showOfflineTag);
        const selected = selectedId === a.id;
        return (
          <button
            key={a.id}
            type="button"
            disabled={!selectable}
            onClick={() => selectable && onSelect(a)}
            className={cn(
              "group relative flex flex-col gap-2 rounded-xl border p-3.5 text-left transition-all duration-150",
              selected
                ? "border-accent/60 bg-accent/[0.06] shadow-glow-accent"
                : "border-line bg-surface hover:border-line-strong hover:bg-surface-2",
              featured && "sm:col-span-2 lg:col-span-1",
              !selectable && "cursor-not-allowed opacity-55 hover:border-line hover:bg-surface",
            )}
          >
            <div className="flex items-center justify-between gap-2">
              <span className="inline-flex items-center gap-1.5 text-2xs font-medium uppercase tracking-wide text-faint">
                <Icon size={13} className="text-muted" />
                {label}
              </span>
              {featured && (
                <span className="inline-flex items-center gap-1 rounded-md bg-accent/15 px-1.5 py-0.5 text-2xs font-semibold text-accent">
                  <Star size={10} className="fill-accent" /> Lead
                </span>
              )}
              {offline && !featured && <Chip tone="neutral">Offline zoo</Chip>}
            </div>

            <div className="font-mono text-[13px] font-medium leading-tight text-fg">{a.name}</div>
            {a.note && <p className="text-xs leading-snug text-muted">{a.note}</p>}

            <div className="mt-auto flex items-center gap-2 pt-1">
              <code className="truncate font-mono text-2xs text-faint">{a.source}</code>
            </div>

            {selected && (
              <span className="pointer-events-none absolute inset-x-0 -bottom-px mx-auto h-0.5 w-2/3 rounded-full bg-accent/70" />
            )}
          </button>
        );
      })}
    </div>
  );
}
