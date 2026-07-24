"use client";

import { Boxes, Check, FileArchive, Puzzle, Server, Star } from "lucide-react";
import { cn } from "@/lib/cn";
import type { Artifact } from "@/lib/events";
import { isLeadArtifact } from "@/lib/fixtures";
import { Chip } from "@/components/ui";

const KIND_META: Record<string, { label: string; Icon: typeof Server }> = {
  mcp_server: { label: "MCP server", Icon: Server },
  skill: { label: "Skill", Icon: Puzzle },
  zip: { label: "Vibe-coded zip", Icon: FileArchive },
};

function kindMeta(kind: string) {
  return KIND_META[kind] ?? { label: kind, Icon: Boxes };
}

/**
 * The artifact zoo. A single soft well of spacing-separated rows (DESIGN §2.4 —
 * no cards inside cards, no zebra striping, no drawn row boxes): mono for the
 * package name and the source command, sentence-case sans for everything a
 * human reads.
 *
 * Selection is carried by **fill and ink, with no hue at all**: the armed row
 * takes the strongest neutral surface, its name steps up to `--fg` while the
 * rest stay `--muted`, and a check sits in the gutter so state never rests on
 * fill alone. The icon tile and the "Lead" chip are metadata, so they stay
 * quiet. There is no accent anywhere on this page.
 *
 * The well is height-capped and scrolls internally: with a full zoo the list
 * must never push the Detonate action or the console columns below the fold
 * (DESIGN §3 — the console fills the viewport). A short fade at the bottom
 * keeps the partial row a scroller always leaves from being sliced flat.
 */
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
    <div className="relative">
      <div className="flex max-h-[clamp(9rem,28vh,18rem)] flex-col gap-0.5 overflow-y-auto overscroll-contain rounded-xl bg-surface-2/50 p-1.5">
        {artifacts.map((a) => {
          // Exact identity only. `@acme/clean-notes-mcp` is the poisoned
          // server's honest twin, and a substring test on the name badged the
          // wrong one as the lead.
          const featured = isLeadArtifact(a);
          const { label, Icon } = kindMeta(a.kind);
          const offline = showOfflineTag && !isReplayable(a);
          const selectable = !disabled && (isReplayable(a) || !showOfflineTag);
          const selected = selectedId === a.id;
          return (
            <button
              key={a.id}
              type="button"
              disabled={!selectable}
              aria-pressed={selected}
              onClick={() => selectable && onSelect(a)}
              className={cn(
                "focus-ring group flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left transition-colors duration-200 ease-instrument focus-visible:ring-offset-0",
                selected ? "bg-surface-3" : "hover:bg-surface-3/60",
                !selectable && "cursor-not-allowed opacity-50 hover:bg-transparent",
              )}
            >
              {/* neutral shape cue — the armed row is marked, not re-coloured */}
              <span className="grid w-3.5 shrink-0 place-items-center" aria-hidden="true">
                {selected && <Check size={13} className="text-fg" />}
              </span>

              <span
                className={cn(
                  "grid h-8 w-8 shrink-0 place-items-center rounded-lg transition-colors duration-200 ease-instrument",
                  selected
                    ? "bg-surface-2 text-fg"
                    : "bg-surface-3/70 text-faint group-hover:text-muted",
                )}
              >
                <Icon size={15} aria-hidden="true" />
              </span>

              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="flex min-w-0 items-center gap-2">
                  <span
                    className={cn(
                      "truncate font-mono text-sm font-medium",
                      selected ? "text-fg" : "text-muted group-hover:text-fg",
                    )}
                  >
                    {a.name}
                  </span>
                  {featured && (
                    <Chip tone="neutral" className="shrink-0 gap-1 px-1.5">
                      <Star size={10} className="fill-faint text-faint" aria-hidden="true" /> Lead
                    </Chip>
                  )}
                  {offline && !featured && <Chip tone="neutral">Offline zoo</Chip>}
                </span>
                {a.note && <span className="truncate text-sm text-muted">{a.note}</span>}
                {/* compact metadata line for narrow viewports */}
                <span className="flex min-w-0 items-center gap-1.5 text-xs text-faint sm:hidden">
                  <span className="shrink-0">{label}</span>
                  <span aria-hidden="true">·</span>
                  <code className="truncate font-mono">{a.source}</code>
                </span>
              </span>

              <span className="hidden shrink-0 flex-col items-end gap-0.5 sm:flex">
                <span className="text-sm text-muted">{label}</span>
                <code className="max-w-[13rem] truncate font-mono text-2xs text-faint">
                  {a.source}
                </code>
              </span>
            </button>
          );
        })}
      </div>

      {/* Sits outside the scroller, so the well keeps its own fill and its
          rounded bottom while the partial row underneath fades into the panel. */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-px bottom-px h-7 rounded-b-xl bg-gradient-to-t from-surface to-transparent"
      />
    </div>
  );
}
