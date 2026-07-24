"use client";

import { useEffect } from "react";
import { THEME_STORAGE_KEY, applyTheme, loadTheme } from "@/lib/prefs";

/**
 * Keeps <html class="dark"> honest after first paint.
 *
 * The pre-paint script in app/layout.tsx sets the class before anything renders
 * so there is no flash; this component exists for the one case that script
 * cannot cover — theme "system", where the OS can change its mind while the tab
 * is open — and to re-apply when another tab edits the stored choice.
 */
export function ThemeSync() {
  useEffect(() => {
    applyTheme(loadTheme());

    const mq = window.matchMedia?.("(prefers-color-scheme: dark)");
    const onSystem = () => {
      if (loadTheme() === "system") applyTheme("system");
    };
    mq?.addEventListener("change", onSystem);

    const onStorage = (e: StorageEvent) => {
      if (e.key === null || e.key === THEME_STORAGE_KEY) applyTheme(loadTheme());
    };
    window.addEventListener("storage", onStorage);

    return () => {
      mq?.removeEventListener("change", onSystem);
      window.removeEventListener("storage", onStorage);
    };
  }, []);

  return null;
}
