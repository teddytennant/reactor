"use client";

import { useEffect, useState } from "react";
import { Moon, Sun } from "lucide-react";

export function ThemeToggle() {
  const [dark, setDark] = useState(true);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
    setDark(document.documentElement.classList.contains("dark"));
  }, []);

  const toggle = () => {
    const next = !dark;
    setDark(next);
    document.documentElement.classList.toggle("dark", next);
    try {
      localStorage.setItem("reactor-theme", next ? "dark" : "light");
    } catch {
      /* private mode — ignore */
    }
  };

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label="Toggle color theme"
      className="focus-ring grid h-7 w-7 place-items-center rounded-md border border-line/80 bg-surface/60 text-faint transition-colors duration-200 hover:border-line-strong hover:text-fg"
    >
      {mounted && dark ? <Moon size={13} strokeWidth={1.9} /> : <Sun size={13} strokeWidth={1.9} />}
    </button>
  );
}
