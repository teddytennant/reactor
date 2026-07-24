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
       * Tighter, denser scale. Display sizes lose a step and gain negative
       * tracking; body never goes below 12px (`text-xs`). `text-3xs` is for
       * mono gutter indices ONLY — never prose.
       */
      fontSize: {
        "3xs": ["0.625rem", { lineHeight: "0.875rem", letterSpacing: "0.1em" }],
        "2xs": ["0.6875rem", { lineHeight: "0.9375rem", letterSpacing: "0.01em" }],
        label: ["0.6875rem", { lineHeight: "1rem", letterSpacing: "0.14em" }],
        xs: ["0.75rem", { lineHeight: "1.0625rem" }],
        sm: ["0.8125rem", { lineHeight: "1.1875rem" }],
        base: ["0.9375rem", { lineHeight: "1.375rem" }],
        lg: ["1.0625rem", { lineHeight: "1.5rem", letterSpacing: "-0.01em" }],
        xl: ["1.1875rem", { lineHeight: "1.625rem", letterSpacing: "-0.014em" }],
        "2xl": ["1.4375rem", { lineHeight: "1.8125rem", letterSpacing: "-0.019em" }],
        "3xl": ["1.75rem", { lineHeight: "2.0625rem", letterSpacing: "-0.022em" }],
        "4xl": ["2.125rem", { lineHeight: "2.375rem", letterSpacing: "-0.026em" }],
        "5xl": ["2.75rem", { lineHeight: "2.9375rem", letterSpacing: "-0.03em" }],
      },
      letterSpacing: {
        label: "0.14em",
        "label-wide": "0.18em",
      },
      borderRadius: {
        lg: "0.5rem",
        xl: "0.75rem",
        "2xl": "1rem",
      },
      transitionTimingFunction: {
        // The Reactor easing. Purposeful, fast, physical.
        instrument: "cubic-bezier(0.16,1,0.3,1)",
      },
      boxShadow: {
        // Elevation is hairline + inner top highlight, not a drop shadow.
        panel: "var(--panel-shadow)",
        "panel-flat": "var(--panel-shadow-flat)",
        float:
          "var(--panel-shadow-flat), 0 18px 48px -26px rgb(var(--shadow) / 0.55), 0 2px 10px -6px rgb(var(--shadow) / 0.4)",
        card: "var(--panel-shadow)",
        "card-dark": "var(--panel-shadow)",
        // Ring + halo in a role's own hue. For lit rows, chips and markers.
        "glow-accent":
          "0 0 0 1px rgb(var(--accent) / 0.28), 0 0 26px -8px rgb(var(--accent) / 0.45)",
        "glow-live":
          "0 0 0 1px rgb(var(--live) / 0.28), 0 0 26px -8px rgb(var(--live) / 0.45)",
        "glow-danger":
          "0 0 0 1px rgb(var(--danger) / 0.34), 0 0 28px -6px rgb(var(--danger) / 0.5)",
        "glow-success":
          "0 0 0 1px rgb(var(--success) / 0.28), 0 0 26px -8px rgb(var(--success) / 0.42)",
        "glow-warning":
          "0 0 0 1px rgb(var(--warning) / 0.28), 0 0 26px -8px rgb(var(--warning) / 0.42)",
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
        "danger-pulse": {
          "0%": { boxShadow: "0 0 0 0 rgb(var(--danger) / 0.0)" },
          "35%": {
            boxShadow:
              "0 0 0 3px rgb(var(--danger) / 0.28), 0 0 30px -4px rgb(var(--danger) / 0.55)",
          },
          "100%": { boxShadow: "0 0 0 1px rgb(var(--danger) / 0.28)" },
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
        // also used directly by .core-bloom / .scan-sweep / .led).
        "core-breathe": "core-breathe 5.5s cubic-bezier(0.4,0,0.6,1) infinite",
        "scan-sweep": "scan-sweep 2.6s cubic-bezier(0.45,0,0.55,1) infinite",
        "led-pulse": "led-pulse 2.4s cubic-bezier(0.4,0,0.6,1) infinite",
      },
    },
  },
  plugins: [],
};

export default config;
