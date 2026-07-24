import type { Config } from "tailwindcss";

/**
 * Colors are declared as CSS custom properties (space-separated RGB channels)
 * in globals.css and swapped under `.dark`. Referencing them through the
 * `rgb(var(--x) / <alpha-value>)` form keeps every Tailwind opacity utility
 * (bg-surface/60, border-line/40, text-live/70, …) working while giving one
 * source of truth for the light/dark palette. See DESIGN.md §6.
 */
const rgb = (v: string) => `rgb(var(${v}) / <alpha-value>)`;

const config: Config = {
  darkMode: "class",
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
  /**
   * The signature primitives from DESIGN.md §2 live in `@layer components` in
   * globals.css, which Tailwind tree-shakes against `content`. They are part of
   * the platform, not of any one feature, so they are pinned here — otherwise a
   * primitive silently vanishes from the bundle until some file happens to use
   * it. Keep this list in sync with DESIGN.md §6.
   */
  safelist: [
    "panel",
    "panel-flat",
    "panel-float",
    "hairline",
    "rule",
    "bezel",
    "core-bloom",
    "instrument-grid",
    "instrument-grid-lines",
    "rail",
    "rail-tight",
    "rail-row",
    "rail-time",
    "rail-node",
    "scan-sweep",
    "led",
    "led-sm",
    "led-lg",
    "tnum",
    "strip-label",
    "telemetry",
    "code-chip",
    "chrome-edge",
    "focus-ring",
    "console-veil",
    "sweep-bg",
  ],
  theme: {
    extend: {
      colors: {
        bg: rgb("--bg"),
        surface: rgb("--surface"),
        "surface-2": rgb("--surface-2"),
        "surface-3": rgb("--surface-3"),
        line: rgb("--line"),
        "line-strong": rgb("--line-strong"),
        fg: rgb("--fg"),
        muted: rgb("--muted"),
        faint: rgb("--faint"),
        accent: rgb("--accent"),
        "accent-fg": rgb("--accent-fg"),
        "accent-dim": rgb("--accent-dim"),
        live: rgb("--live"),
        "live-fg": rgb("--live-fg"),
        "live-dim": rgb("--live-dim"),
        danger: rgb("--danger"),
        "danger-fg": rgb("--danger-fg"),
        "danger-dim": rgb("--danger-dim"),
        success: rgb("--success"),
        "success-fg": rgb("--success-fg"),
        "success-dim": rgb("--success-dim"),
        warning: rgb("--warning"),
        "warning-fg": rgb("--warning-fg"),
        "warning-dim": rgb("--warning-dim"),
      },
      fontFamily: {
        sans: ["var(--font-sans)", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["var(--font-mono)", "ui-monospace", "SFMono-Regular", "monospace"],
      },
      /**
       * Comfortable, not compressed — the reference breathes. Sans carries the
       * interface: `text-sm` (13px) is the section-label size, `text-base`
       * (15px / 24px) is body prose. `text-3xs` and `text-2xs` are for mono
       * machine data only (evidence ids, byte counts) — never prose.
       */
      fontSize: {
        "3xs": ["0.625rem", { lineHeight: "0.9375rem", letterSpacing: "0.005em" }],
        "2xs": ["0.6875rem", { lineHeight: "1rem", letterSpacing: "0.005em" }],
        label: ["0.8125rem", { lineHeight: "1.25rem" }],
        xs: ["0.75rem", { lineHeight: "1.125rem" }],
        sm: ["0.8125rem", { lineHeight: "1.25rem" }],
        base: ["0.9375rem", { lineHeight: "1.5rem" }],
        lg: ["1.0625rem", { lineHeight: "1.625rem", letterSpacing: "-0.008em" }],
        xl: ["1.1875rem", { lineHeight: "1.75rem", letterSpacing: "-0.012em" }],
        "2xl": ["1.375rem", { lineHeight: "1.875rem", letterSpacing: "-0.016em" }],
        "3xl": ["1.6875rem", { lineHeight: "2.125rem", letterSpacing: "-0.02em" }],
        "4xl": ["2.0625rem", { lineHeight: "2.5rem", letterSpacing: "-0.022em" }],
        "5xl": ["2.625rem", { lineHeight: "3rem", letterSpacing: "-0.026em" }],
      },
      /**
       * Retained names, softened hard. Uppercase letterspacing is now reserved
       * for genuine status stamps (BLOCKED, CLEAN) — never routine labels.
       */
      letterSpacing: {
        label: "0.02em",
        "label-wide": "0.08em",
      },
      // Generous radii. Nothing sharp.
      borderRadius: {
        md: "0.4375rem",
        lg: "0.625rem",
        xl: "0.75rem",
        "2xl": "1rem",
      },
      transitionTimingFunction: {
        // The Reactor easing. Purposeful, fast, physical.
        instrument: "cubic-bezier(0.16,1,0.3,1)",
      },
      boxShadow: {
        // Soft, wide, low-opacity — or absent. Never a hard drop shadow.
        panel: "var(--panel-shadow)",
        "panel-flat": "var(--panel-shadow-flat)",
        float:
          "0 2px 6px -2px rgb(var(--shadow) / 0.18), 0 24px 60px -30px rgb(var(--shadow) / 0.5)",
        card: "var(--panel-shadow)",
        "card-dark": "var(--panel-shadow)",
        /**
         * `shadow-glow-*` — names retained because siblings use them, but the
         * neon rim is gone. These are now soft, wide, low-opacity casts tinted
         * by the role. No `0 0 0 1px` ring, no visible halo. Prefer a border
         * tint (`border-danger/30`) over reaching for these at all.
         */
        "glow-accent": "0 8px 28px -14px rgb(var(--accent) / 0.35)",
        "glow-live": "0 8px 28px -14px rgb(var(--live) / 0.3)",
        "glow-danger": "0 8px 28px -14px rgb(var(--danger) / 0.4)",
        "glow-success": "0 8px 28px -14px rgb(var(--success) / 0.32)",
        "glow-warning": "0 8px 28px -14px rgb(var(--warning) / 0.32)",
      },
      keyframes: {
        "fade-slide-up": {
          "0%": { opacity: "0", transform: "translateY(8px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
        "fade-in": {
          "0%": { opacity: "0" },
          "100%": { opacity: "1" },
        },
        // A row ticking in from the rail — the session ladder cadence.
        "tick-in": {
          "0%": { opacity: "0", transform: "translateX(-6px)" },
          "100%": { opacity: "1", transform: "translateX(0)" },
        },
        "signal-snap": {
          "0%": { opacity: "0", transform: "translateY(6px) scale(0.985)" },
          "60%": { opacity: "1", transform: "translateY(0) scale(1.004)" },
          "100%": { opacity: "1", transform: "translateY(0) scale(1)" },
        },
        // Softened: a brief warm danger cast that settles, not a neon rim.
        "danger-pulse": {
          "0%": { boxShadow: "0 0 0 0 rgb(var(--danger) / 0)" },
          "35%": { boxShadow: "0 6px 26px -12px rgb(var(--danger) / 0.55)" },
          "100%": { boxShadow: "0 6px 26px -16px rgb(var(--danger) / 0.25)" },
        },
        "verdict-in": {
          "0%": { opacity: "0", transform: "translateY(14px) scale(0.97)" },
          "100%": { opacity: "1", transform: "translateY(0) scale(1)" },
        },
        "spin-slow": {
          to: { transform: "rotate(360deg)" },
        },
        "pulse-dot": {
          "0%, 100%": { opacity: "1" },
          "50%": { opacity: "0.35" },
        },
        sweep: {
          "0%": { backgroundPosition: "-160% 0" },
          "100%": { backgroundPosition: "260% 0" },
        },
        "bar-grow": {
          "0%": { transform: "scaleX(0)" },
          "100%": { transform: "scaleX(1)" },
        },
      },
      animation: {
        "fade-slide-up": "fade-slide-up 0.34s cubic-bezier(0.16,1,0.3,1) both",
        "fade-in": "fade-in 0.4s ease-out both",
        "tick-in": "tick-in 0.3s cubic-bezier(0.16,1,0.3,1) both",
        "signal-snap":
          "signal-snap 0.42s cubic-bezier(0.16,1,0.3,1) both, danger-pulse 1.1s ease-out both",
        "verdict-in": "verdict-in 0.5s cubic-bezier(0.16,1,0.3,1) both",
        "spin-slow": "spin-slow 1.1s linear infinite",
        "pulse-dot": "pulse-dot 1.3s ease-in-out infinite",
        sweep: "sweep 1.6s ease-in-out infinite",
        "bar-grow": "bar-grow 0.9s cubic-bezier(0.16,1,0.3,1) both",
        // These three reference @keyframes declared in globals.css (they are
        // also used directly by .core-bloom / .scan-sweep / .led). All three
        // are now the same slow opacity breath — no travel, no flash.
        "core-breathe": "core-breathe 6s cubic-bezier(0.4,0,0.6,1) infinite",
        "scan-sweep": "scan-sweep 3.2s cubic-bezier(0.4,0,0.6,1) infinite",
        "led-pulse": "led-pulse 2.8s cubic-bezier(0.4,0,0.6,1) infinite",
      },
    },
  },
  plugins: [],
};

export default config;
