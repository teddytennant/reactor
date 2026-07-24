# Deploy Reactor web to Vercel

Public site: **https://reactor.teddytennant.com**  
Vercel alias: **https://reactor-liard.vercel.app** (307s to the custom domain)

The Next.js console is a **static export** (`web/out`). The Go engine is **not**
hosted anywhere — **each visitor runs it on their own machine** and the deployed
console talks to it on loopback (`http://127.0.0.1:8787` by default).

That is a deliberate choice, not a limitation. The engine spawns process trees,
tails collector logs and holds a multi-minute SSE stream per detonation, so it
cannot live on a static or serverless host. Running it visitor-side also means
their API keys never transit anything of ours, their artifacts never leave their
machine, and there is no shared endpoint to rate-limit or defend.

### Browser support

An `https://` page fetching `http://127.0.0.1` is mixed content, and browsers
disagree about it.

| Browser | Live detonation | Notes |
|---|---|---|
| Chrome, Edge, Brave | ✅ | Chromium treats loopback as trustworthy. Needs the Private Network Access preflight grant, which `internal/engine/api.go` sends. |
| Safari (and **all** iOS browsers) | ❌ replay only | WebKit blocks loopback from https outright. iOS Chrome/Firefox are WebKit underneath, so they inherit it. |
| Firefox | ⚠️ varies | Not asserted either way — the console probes and reports what actually happened. |

The console never predicts this from a browser allowlist. It probes the engine,
and only explains the failure afterwards (`unreachableReason()` in
`web/lib/engine.ts`). Anyone without a reachable engine gets the bundled replay
of a real detonation, which is a complete demo on its own.

## Why you saw `404: NOT_FOUND`

Vercel was treating the **Go monorepo root** as a Next.js app. There is no
`package.json` / `.next` at `/`, so the production alias served Vercel's
NOT_FOUND card even though the deployment status said "Completed".

This repo now ships a root `vercel.json` that always:

1. `npm ci --prefix web`
2. `npm run build --prefix web` (static export when `VERCEL=1`)
3. Publishes **`web/out`**

No dashboard Root Directory change is required for that path to work. If Root
Directory is already set to `web`, `web/vercel.json` does the same with
`outputDirectory: out`.

## One-time setup

1. Import / reconnect https://github.com/teddytennant/reactor.
2. **No env vars are required.** The static export defaults the engine origin to
   `http://127.0.0.1:8787` (`NEXT_PUBLIC_STATIC_EXPORT=1`, set by
   `next.config.mjs`), and `NEXT_PUBLIC_SITE_URL` already defaults to the public
   URL in `web/app/layout.tsx`. Two optional overrides:

| Name | When you'd set it |
|---|---|
| `NEXT_PUBLIC_ENGINE_URL` | You *do* run a shared engine and want it to be the default for everyone. Visitors can still override it in Settings. |
| `NEXT_PUBLIC_SITE_URL` | The site moves off `reactor.teddytennant.com`. |

3. Domains → add `reactor.teddytennant.com`, then at Cloudflare create
   **A `reactor` → `76.76.21.21`, proxy status DNS only (grey cloud)**. Proxied
   breaks ACME issuance. ✅ *Done — cert issued, live.*
4. Redeploy Production (push to `main`, or Deployments → … → Redeploy).

### CLI

```bash
# from repo root (uses ./vercel.json → web/out)
npx vercel login
npx vercel --prod

# or from web/ if Root Directory = web
cd web && npx vercel --prod
```

## Running the engine (what a visitor does)

```bash
git clone https://github.com/teddytennant/reactor && cd reactor
make build
./bin/reactor serve          # 127.0.0.1:8787
```

Then open https://reactor.teddytennant.com in Chrome, Edge or Brave, paste
Daytona + Fireworks keys into Settings, and detonate. Keys stay in that
browser's localStorage and go only to the engine on their own machine.

No TLS or reverse proxy is needed for the loopback case — the browser exempts
`127.0.0.1` from mixed-content blocking (except WebKit, see above).

If you *do* want a shared engine on a public host, it must be HTTPS: you would
be accepting custody of other people's API keys in transit, and browsers block
plain-http from an https page anyway. You would also want a rate limit on
`/api/detonate`, since each call is a multi-minute run that pins memory.

## What visitors see

1. **Onboarding** asks for Daytona + Fireworks keys and the engine URL
   (localStorage only; engine defaults to `http://127.0.0.1:8787`).
2. They can **skip** and run the bundled replay demo with no keys and no engine.
3. With keys + their own running engine, Detonate provisions live Daytona
   sandboxes under *their* account, billed to *them*.
4. **Settings** (gear) re-opens the form any time.
5. If no engine answers, the console says why — and names Safari explicitly when
   that is the cause.

## Verify

```bash
curl -sI https://reactor.teddytennant.com | head -5
# expect HTTP/2 200, content-type text/html — not x-vercel-error: NOT_FOUND
```

Local static export smoke test:

```bash
cd web && REACTOR_EXPORT=1 npm run build && ls out/index.html
```
