/** @type {import('next').NextConfig} */

// The engine's HTTP + SSE API (docs/CONTRACT.md) lives on :8787 locally.
//
// Local `next dev`: rewrite /api/* → engine so the browser stays same-origin.
// Vercel / static export: no rewrites (unsupported). Set NEXT_PUBLIC_ENGINE_URL
// to the public engine origin; the browser talks directly via lib/engine.ts.
// VERCEL=1 is set by the Vercel build environment.
const ENGINE_URL = (process.env.NEXT_PUBLIC_ENGINE_URL || "http://127.0.0.1:8787").replace(
  /\/$/,
  "",
);

const isStaticExport =
  process.env.VERCEL === "1" || process.env.REACTOR_EXPORT === "1";

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // No eslint config is shipped (deps kept lean); never let a lint pass gate the build.
  eslint: { ignoreDuringBuilds: true },
};

if (isStaticExport) {
  // Static HTML so monorepo-root Vercel deploys serve real files from web/out
  // instead of looking for a Next.js app at the Go repo root (404 NOT_FOUND).
  nextConfig.output = "export";
  nextConfig.images = { unoptimized: true };
} else {
  nextConfig.rewrites = async () => [
    {
      source: "/api/:path*",
      destination: `${ENGINE_URL}/api/:path*`,
    },
  ];
}

export default nextConfig;
