// Console preferences — everything the settings page owns that is not a secret
// and not the engine origin. Credentials live in lib/credentials.ts, the engine
// origin in lib/engine.ts. All of it is localStorage-only: Reactor has no
// account system and the Vercel host never sees any of this.

export const THEME_STORAGE_KEY = "reactor-theme";
export const PREFS_STORAGE_KEY = "reactor-prefs";

// ---- theme ----------------------------------------------------------------
//
// The stored key predates "system" and held "dark" | "light" only; both still
// read back as themselves, and anything unrecognised (or absent) means dark.
// Dark is the default rather than the OS preference because this is a
// containment instrument and the design is dark-native (DESIGN §1) — see the
// pre-paint script in app/layout.tsx, which must agree with resolveTheme().

export type ThemeChoice = "dark" | "light" | "system";

export function loadTheme(): ThemeChoice {
  if (typeof window === "undefined") return "dark";
  try {
    const raw = localStorage.getItem(THEME_STORAGE_KEY);
    return raw === "light" || raw === "system" ? raw : "dark";
  } catch {
    return "dark";
  }
}

export function saveTheme(choice: ThemeChoice): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(THEME_STORAGE_KEY, choice);
  } catch {
    /* private mode */
  }
}

/** True when the OS is currently asking for a dark UI. */
export function systemPrefersDark(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return true;
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function resolveTheme(choice: ThemeChoice): "dark" | "light" {
  if (choice === "system") return systemPrefersDark() ? "dark" : "light";
  return choice;
}

/** Put the resolved theme on <html>. Safe to call on every render path. */
export function applyTheme(choice: ThemeChoice): void {
  if (typeof document === "undefined") return;
  document.documentElement.classList.toggle("dark", resolveTheme(choice) === "dark");
}

// ---- detonation defaults ---------------------------------------------------

export interface Prefs {
  /** Sessions per detonation. The engine clamps nothing, so the UI does. */
  sessions: number;
  /** Give the chamber outbound network. Off is the safe default. */
  network: boolean;
  /** Skip the sample-artifact rack and open straight into intake. */
  autoOpenZoo: boolean;
}

export const MIN_SESSIONS = 1;
export const MAX_SESSIONS = 12;

export const DEFAULT_PREFS: Prefs = {
  sessions: 5,
  network: false,
  autoOpenZoo: false,
};

export function clampSessions(n: number): number {
  if (!Number.isFinite(n)) return DEFAULT_PREFS.sessions;
  return Math.min(MAX_SESSIONS, Math.max(MIN_SESSIONS, Math.round(n)));
}

export function loadPrefs(): Prefs {
  if (typeof window === "undefined") return { ...DEFAULT_PREFS };
  try {
    const raw = localStorage.getItem(PREFS_STORAGE_KEY);
    if (!raw) return { ...DEFAULT_PREFS };
    const p = JSON.parse(raw) as Partial<Prefs>;
    return {
      sessions: clampSessions(typeof p.sessions === "number" ? p.sessions : DEFAULT_PREFS.sessions),
      network: p.network === true,
      autoOpenZoo: p.autoOpenZoo === true,
    };
  } catch {
    return { ...DEFAULT_PREFS };
  }
}

export function savePrefs(p: Prefs): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(
      PREFS_STORAGE_KEY,
      JSON.stringify({ ...p, sessions: clampSessions(p.sessions) }),
    );
  } catch {
    /* private mode */
  }
}

export function clearPrefs(): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.removeItem(PREFS_STORAGE_KEY);
  } catch {
    /* private mode */
  }
}
