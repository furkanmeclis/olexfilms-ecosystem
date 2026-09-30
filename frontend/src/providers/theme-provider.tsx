"use client";

import { ThemeProvider as NextThemesProvider } from "next-themes";
import type { ReactNode } from "react";

import { themeConfig } from "@/config/theme";

// next-themes injects an inline <script> to prevent theme FOUC. React 19 warns
// about script tags inside Client Components; the script still runs correctly
// during SSR. Filter this known false-positive in development only.
if (typeof window !== "undefined" && process.env.NODE_ENV === "development") {
  const orig = console.error;
  console.error = (...args: unknown[]) => {
    const message = args
      .map((arg) => {
        if (typeof arg === "string") return arg;
        if (arg instanceof Error) return arg.message;
        return "";
      })
      .join(" ");
    if (message.includes("Encountered a script tag")) return;
    orig.apply(console, args);
  };
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme={themeConfig.defaultMode}
      enableSystem
      storageKey={themeConfig.storageKey}
      disableTransitionOnChange
    >
      {children}
    </NextThemesProvider>
  );
}
