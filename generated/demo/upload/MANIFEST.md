# Reactor demo screenshots — upload pack

Captured 2026-07-24 with system Chrome (Playwright driving `/usr/bin/google-chrome`)
against `http://localhost:3000`.

**Provenance — read this before shipping any of it.** The capture browser had its
engine URL pointed at a closed port (`http://127.0.0.1:9`), so the probe failed and
every console frame is the **bundled fixture replay** (`web/fixtures/notes-mcp-detonation.json`
and `benign-detonation.json`) rendered through the real UI. The pixels are real UI;
the run behind them is a recording, not a live detonation. The console says so
itself in-frame — "Bundled replay" sits in the arming strip of every console shot.

Every image is the browser viewport at **1620×1080 CSS px, 2× DPR → 3240×2160**,
so the whole pack is a uniform **3:2** and nothing needs cropping. Largest file is
586 KB. Dark theme except `12`. Next.js dev badge suppressed.

## The pack

| # | file | caption |
|---|------|---------|
| 01 | `01-console-idle.png` | The console at rest — drop an artifact, or take one off the rack. |
| 02 | `02-zoo-picker.png` | The zoo — five authored artifacts, `@acme/notes-mcp` armed. |
| 03 | `03-sessions.png` | Five fresh sandboxes ticking — one sacrificial victim agent each. |
| 04 | `04-streams.png` | Wire log and victim transcript streaming out of the chamber. |
| 05 | `05-rug-pull.png` | `rug_pull` fires — the tool description mutates after the client trusted it. |
| 06 | `06-canary-exfil.png` | The prompt canary reaches the sink — a token that was never on disk. |
| 07 | `07-analyst.png` | The analyst loop reads the evidence and writes the verdict. |
| 08 | `08-verdict-blocked.png` | **The money shot** — mcp-scan says CLEAN on the left, Reactor says BLOCKED on the right. |
| 09 | `09-allowed-control.png` | The control — the official filesystem server behaves, and is allowed through. |
| 10 | `10-scorecard.png` | Zoo scorecard — detection rate, false quarantines, static-blind count. |
| 11 | `11-settings.png` | Settings — engine URL, BYOK keys, detonation defaults. All device-local. |
| 12 | `12-console-light.png` | The same instrument in light — state is never carried by hue alone. |

## Do not drop

`08` carries the whole pitch. `05` and `06` are the two signals it rests on, and
`09` is the control that makes a BLOCKED verdict mean anything.

## If you can only ship 6

`08`, `05`, `06`, `09`, `10`, `01`.

## Re-capturing

The frames are driven by scripted clicks and waits on named UI text, not by fixed
sleeps, so a re-run reproduces the same twelve beats. Capturing against a **live**
engine instead means starting `./bin/reactor serve` on :8787, leaving the browser's
engine URL unset, and supplying Daytona + Fireworks keys — the run then costs real
sandbox time and the victim model is nondeterministic about taking the bait.
