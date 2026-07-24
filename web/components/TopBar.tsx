"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/cn";
import { Wordmark } from "./ReactorMark";
import { ThemeToggle } from "./ThemeToggle";

const NAV = [
  { href: "/", label: "Console" },
  { href: "/scorecard", label: "Scorecard" },
];

export function TopBar() {
  const pathname = usePathname();
  return (
    <header className="chrome-edge sticky top-0 z-30 border-b border-line bg-bg/80 backdrop-blur-md">
      <div className="mx-auto flex h-12 max-w-[1400px] items-center justify-between gap-4 px-4 sm:px-6">
        <div className="flex min-w-0 items-center gap-3">
          <Link
            href="/"
            className="focus-ring rounded-md transition-opacity hover:opacity-80"
          >
            <Wordmark />
          </Link>
          <span className="hidden h-3.5 w-px bg-line sm:block" aria-hidden="true" />
          <span className="strip-label hidden truncate sm:inline">Detonation chamber</span>
        </div>

        <div className="flex items-center gap-1">
          {/* Mono nav labels with a hairline underline indicator — instrument
              chrome, not a pill switcher. */}
          <nav className="flex items-stretch self-stretch">
            {NAV.map((item) => {
              const active = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "focus-ring relative flex items-center px-3 font-mono text-label font-medium uppercase transition-colors duration-200",
                    active ? "text-fg" : "text-faint hover:text-muted",
                  )}
                >
                  {item.label}
                  <span
                    aria-hidden="true"
                    className={cn(
                      "pointer-events-none absolute inset-x-2.5 bottom-0 h-px transition-opacity duration-200",
                      active ? "bg-accent opacity-100" : "bg-fg opacity-0",
                    )}
                  />
                  {active && (
                    <span
                      aria-hidden="true"
                      className="pointer-events-none absolute inset-x-2.5 bottom-0 h-px bg-accent blur-[3px]"
                    />
                  )}
                </Link>
              );
            })}
          </nav>
          <span className="mx-1 hidden h-3.5 w-px bg-line sm:block" aria-hidden="true" />
          <ThemeToggle />
        </div>
      </div>
    </header>
  );
}
