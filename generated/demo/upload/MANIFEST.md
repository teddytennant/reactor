# Reactor demo screenshots — upload pack (≤15)

Captured with Playwright (Chromium) against `http://localhost:3000` in **fixture replay** mode
(bundled notes-mcp money shot + filesystem ALLOWED control + scorecard).
Dark theme. Next.js dev badge hidden. Engine was pointed at a dead port so the console
used the authored fixtures (reliable mid-run stills).

## Ranked pack

| # | file | dims | size | caption |
|---|------|------|------|---------|
| 01 | `01-hero-blocked.png` | 1600×1100 | 223KB | Money shot — mcp-scan CLEAN left, Reactor BLOCKED right (@acme/notes-mcp) |
| 02 | `02-scan-clean.png` | 1600×1100 | 147KB | Static scanner says CLEAN while chamber arms |
| 03 | `03-byte-diff-rug.png` | 1600×1100 | 181KB | Session 4 rug_pull — tools/list description mutates (+47B) |
| 04 | `04-canary-exfil.png` | 1600×1100 | 206KB | Prompt canary REACTOR-a1b2 hits the egress sink (never on disk) |
| 05 | `05-scorecard.png` | 1440×1100 | 134KB | Zoo scorecard — 24/25 caught, 0 false quarantine, 9 static-blind |
| 06 | `06-intake.png` | 1600×1100 | 187KB | Artifact intake — drop/upload/repo + Detonate |
| 07 | `07-sessions.png` | 1600×1100 | 166KB | Five fresh sandboxes ticking (session ladder) |
| 08 | `08-allowed-control.png` | 1440×900 | 183KB | Control — official filesystem server ALLOWED / benign |
| 09 | `09-zoo-picker.png` | 1600×1100 | 187KB | Sample zoo with @acme/notes-mcp lead selected |
| 10 | `10-blocked-crop.png` | 776×338 | 36KB | BLOCKED verdict banner crop (supply-chain) |
| 11 | `11-clean-crop.png` | 520×277 | 7KB | CLEAN stamp crop (mcp-scan column) |
| 12 | `12-rug-crop.png` | 847×417 | 42KB | rug_pull finding crop |
| 13 | `13-final-two-col.png` | 1600×1100 | 223KB | Final two-column console (same beat as hero) |
| 14 | `14-settings.png` | 1440×2087 | 201KB | Settings — engine URL, BYOK, detonation defaults |
| 15 | `15-allowed-crop.png` | 637×299 | 26KB | ALLOWED banner crop (benign control) |

## Do not drop

- `01-hero-blocked.png`
- `02-scan-clean.png` / `11-clean-crop.png`
- `03-byte-diff-rug.png` / `12-rug-crop.png`
- `04-canary-exfil.png`
- `05-scorecard.png`

## If you can only ship 8

`01`, `02`, `03`, `04`, `05`, `07`, `08`, `15` (or `10` instead of `15`).

## Raw takes

Everything under `generated/demo/` (A–J series + numbered drafts) is the untrimmed roll.
Only `generated/demo/upload/` is the curated ≤15 set.

## Restore live engine

Settings → Engine URL → `http://127.0.0.1:8787` (already restored in the capture browser).
