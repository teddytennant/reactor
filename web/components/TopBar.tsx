"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/cn";
import { Wordmark } from "./ReactorMark";

// Two destinations, and that is the whole of the chrome. Keys, engine URL and
// theme used to live here as separate controls; they are settings, so they live
// on the settings page now (app/settings/page.tsx).
const NAV = [
  { href: "/", label: "Console" },
  { href: "/settings", label: "Settings" },
];

export function TopBar() {
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
      </div>
    </header>
  );
}
