import type { Metadata, Viewport } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { ThemeSync } from "@/components/ThemeSync";

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
  // Must track --bg in globals.css (dark #1D1B18). Not a media-query pair:
  // the theme now defaults to dark rather than following the OS, and a
  // media-query themeColor cannot read the stored choice, so a light-mode OS
  // was colouring the browser chrome for a page that renders dark.
  themeColor: "#1d1b18",
};

// Set the theme class before first paint. A manual choice always wins; with no
// choice on record the console opens dark regardless of the OS preference.
// This is a containment instrument and the design is dark-native (DESIGN §1) —
// a judge on a light-mode laptop was getting the pale theme for the demo, where
// the whole point is a dark console with two coloured stamps on it. The light
// theme stays first-class, it is just no longer what a first visit lands on.
//
// "system" is the third choice offered in Settings → Appearance; it is the only
// one this script cannot decide once and forget, so ThemeSync keeps watching
// the media query after hydration. Must agree with resolveTheme() in lib/prefs.
const THEME_INIT = `(function(){try{var t=localStorage.getItem('reactor-theme');var d=t==='light'?false:t==='system'?!!(window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches):true;document.documentElement.classList.toggle('dark',d);}catch(e){document.documentElement.classList.add('dark');}})();`;

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" suppressHydrationWarning className={`${inter.variable} ${mono.variable}`}>
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_INIT }} />
      </head>
      <body className="min-h-screen font-sans antialiased">
        <ThemeSync />
        {children}
      </body>
    </html>
  );
}
