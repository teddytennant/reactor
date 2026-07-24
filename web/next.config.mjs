/** @type {import('next').NextConfig} */

// The engine's HTTP + SSE API (docs/CONTRACT.md) lives on :8787. We proxy
// /api/* to it so the browser can hit same-origin URLs and EventSource can
// stream without CORS. Override with NEXT_PUBLIC_ENGINE_URL.
const ENGINE_URL = process.env.NEXT_PUBLIC_ENGINE_URL || "http://127.0.0.1:8787";

const nextConfig = {
  reactStrictMode: true,
  // No eslint config is shipped (deps kept lean); never let a lint pass gate the build.
  eslint: { ignoreDuringBuilds: true },
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${ENGINE_URL}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
