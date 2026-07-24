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
  title: "Reactor — detonation console",
  description:
    "A detonation chamber for untrusted agent artifacts. Static scanners read the label; Reactor watches it behave.",
};

export const viewport: Viewport = {
  themeColor: [
    // Must track --bg in globals.css: dark #090B10 / light #F6F8FA.
    { media: "(prefers-color-scheme: dark)", color: "#090b10" },
    { media: "(prefers-color-scheme: light)", color: "#f6f8fa" },
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
