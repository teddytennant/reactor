import type { Metadata, Viewport } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-sans",
  display: "swap",
});

const mono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  // Absolute base for OG/twitter image URLs (overridden in deploy by host).
  metadataBase: new URL(
    process.env.NEXT_PUBLIC_SITE_URL ?? "https://reactor.teddytennant.com",
  ),
  title: "Reactor — detonation console",
  description:
    "A detonation chamber for untrusted agent artifacts. Static scanners read the label; Reactor watches it behave.",
  applicationName: "Reactor",
  icons: {
    // app/favicon.ico + app/icon.png + app/apple-icon.png are file-convention
    // routes. public/icon-32.png is an extra static size for older clients.
    icon: [
      { url: "/favicon.ico", sizes: "any" },
      { url: "/icon.png", type: "image/png", sizes: "512x512" },
      { url: "/icon-32.png", type: "image/png", sizes: "32x32" },
    ],
    apple: [{ url: "/apple-icon.png", sizes: "180x180", type: "image/png" }],
  },
  openGraph: {
    title: "Reactor — detonation console",
    description:
      "Static scanners read the label. Reactor watches it behave.",
    url: "https://reactor.teddytennant.com",
    siteName: "Reactor",
    type: "website",
    images: [{ url: "/og.png", width: 2816, height: 1584, alt: "Reactor" }],
  },
  twitter: {
    card: "summary_large_image",
    title: "Reactor — detonation console",
    description:
      "Static scanners read the label. Reactor watches it behave.",
    images: ["/og.png"],
  },
  alternates: { canonical: "/" },
};

export const viewport: Viewport = {
  themeColor: [
    // Must track --bg in globals.css: dark #1D1B18 / light #F8F5F1.
    { media: "(prefers-color-scheme: dark)", color: "#1d1b18" },
    { media: "(prefers-color-scheme: light)", color: "#f8f5f1" },
  ],
};

// Set the theme class before first paint: manual choice wins, else follow the
// OS preference, defaulting to dark (security-console feel) when unspecified.
const THEME_INIT = `(function(){try{var t=localStorage.getItem('reactor-theme');var d;if(t==='dark')d=true;else if(t==='light')d=false;else d=!window.matchMedia('(prefers-color-scheme: light)').matches;document.documentElement.classList.toggle('dark',d);}catch(e){document.documentElement.classList.add('dark');}})();`;

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" suppressHydrationWarning className={`${inter.variable} ${mono.variable}`}>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_INIT }} />
      </head>
      <body className="min-h-screen font-sans antialiased">{children}</body>
    </html>
  );
}
