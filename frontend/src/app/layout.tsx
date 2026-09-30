import type { Metadata, Viewport } from "next";
import { JetBrains_Mono, Outfit, Plus_Jakarta_Sans } from "next/font/google";

import { brand } from "@/config/brand";
import { site } from "@/config/site";
import { AppProviders } from "@/providers/app-providers";

import "./globals.css";

const plusJakarta = Plus_Jakarta_Sans({
  variable: "--font-jakarta",
  subsets: ["latin", "latin-ext"],
  weight: ["400", "500", "600", "700"],
  display: "swap",
});

const outfit = Outfit({
  variable: "--font-outfit",
  subsets: ["latin", "latin-ext"],
  weight: ["500", "600", "700"],
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  variable: "--font-jetbrains",
  subsets: ["latin", "latin-ext"],
  weight: ["400", "500"],
  display: "swap",
});

export const metadata: Metadata = {
  metadataBase: new URL(site.url),
  title: {
    default: brand.productName,
    template: `%s · ${brand.name}`,
  },
  description: brand.tagline,
  applicationName: brand.productName,
  openGraph: {
    type: "website",
    siteName: site.name,
    locale: site.locale,
  },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: brand.colors.mist },
    { media: "(prefers-color-scheme: dark)", color: brand.colors.ink },
  ],
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="tr" suppressHydrationWarning className="h-full">
      <body
        className={`${plusJakarta.variable} ${outfit.variable} ${jetbrainsMono.variable} min-h-full font-sans antialiased`}
      >
        <AppProviders>{children}</AppProviders>
      </body>
    </html>
  );
}
