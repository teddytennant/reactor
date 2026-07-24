# Deploy Reactor web to Vercel

Public site: **https://reactor.teddytennant.com**  
Current Vercel alias: **https://reactor-liard.vercel.app**

The Next.js console is a **static export** (`web/out`). The Go engine is **not**
on Vercel — point the UI at it with `NEXT_PUBLIC_ENGINE_URL`.

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
2. Env vars (Project → Settings → Environment Variables):

| Name | Value |
|---|---|
| `NEXT_PUBLIC_ENGINE_URL` | `https://<your-engine-host>` (no trailing slash) |
| `NEXT_PUBLIC_SITE_URL` | `https://reactor.teddytennant.com` |

3. Domains → add `reactor.teddytennant.com` (DNS is currently **NXDOMAIN** —
   create a CNAME at your DNS host → `cname.vercel-dns.com`, or the target
   Vercel shows in the domain panel).
4. Redeploy Production (push to `main`, or Deployments → … → Redeploy).

### CLI

```bash
# from repo root (uses ./vercel.json → web/out)
npx vercel login
npx vercel --prod

# or from web/ if Root Directory = web
cd web && npx vercel --prod
```

## Engine host (not Vercel)

```bash
cp .env.example .env
# Optional host defaults; visitors can also BYOK in the UI.
# DAYTONA_API_KEY=...
# FIREWORKS_API_KEY=...
# REACTOR_DRIVER=daytona

make build
./bin/reactor serve -addr 0.0.0.0:8787
```

Put TLS in front (Caddy/nginx/Fly). Visitor keys ride on `POST /api/detonate`
as `credentials` (or `X-Reactor-*` on upload) and never touch Vercel.

## What visitors see

1. **Onboarding** asks for Daytona + Fireworks keys (localStorage only).
2. They can **skip** and run the bundled replay demo with no keys.
3. With keys + a reachable engine, Detonate runs live sandboxes under *their* accounts.
4. **Settings** (gear) re-opens the form any time.

## Verify

```bash
curl -sI https://reactor-liard.vercel.app | head -5
# expect HTTP/2 200, content-type text/html — not x-vercel-error: NOT_FOUND
```

Local static export smoke test:

```bash
cd web && REACTOR_EXPORT=1 npm run build && ls out/index.html
```
