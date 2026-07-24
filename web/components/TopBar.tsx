"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Settings } from "lucide-react";
import { cn } from "@/lib/cn";
import type { Credentials } from "@/lib/credentials";
import { Wordmark } from "./ReactorMark";
import { ThemeToggle } from "./ThemeToggle";
import { CredentialsBadge } from "./CredentialsModal";

const NAV = [
  { href: "/", label: "Console" },
  { href: "/scorecard", label: "Scorecard" },
];

export function TopBar({
  credentials,
  onOpenSettings,
}: {
  credentials?: Credentials;
  onOpenSettings?: () => void;
} = {}) {
  const pathname = usePathname();
  return (
    <header className="chrome-edge sticky top-0 z-30 border-b border-line bg-bg/85 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-[1400px] items-center justify-between gap-4 px-4 sm:px-6">
        <div className="flex min-w-0 items-center gap-3">
          <Link href="/" className="focus-ring rounded-lg transition-opacity hover:opacity-80">
            <Wordmark />
          </Link>
          <span className="hidden truncate text-sm text-faint sm:inline">
            Detonation chamber
          </span>
        </div>

        <div className="flex items-center gap-2">
          <nav className="flex items-center gap-0.5 rounded-xl bg-surface-2/70 p-1">
            {NAV.map((item) => {
              const active = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "focus-ring rounded-lg px-3 py-1.5 text-sm transition-colors duration-200",
                    active
                      ? "bg-surface text-fg shadow-panel"
                      : "text-muted hover:bg-surface/60 hover:text-fg",
                  )}
                >
                  {item.label}
                </Link>
              );
            })}
          </nav>
          {credentials && onOpenSettings && (
            <CredentialsBadge credentials={credentials} onClick={onOpenSettings} />
          )}
          {onOpenSettings && (
            <button
              type="button"
              onClick={onOpenSettings}
              aria-label="API keys and settings"
              title="API keys"
              className="focus-ring grid h-9 w-9 place-items-center rounded-xl text-muted transition-colors duration-200 hover:bg-surface-2 hover:text-fg"
            >
              <Settings size={16} strokeWidth={1.75} aria-hidden="true" />
            </button>
          )}
          <ThemeToggle />
        </div>
      </div>
    </header>
  );
}
