import { brand } from "@/config/brand";

export type ThemeMode = "light" | "dark";

export const themeConfig = {
  defaultMode: "light" as ThemeMode,
  storageKey: "app.theme",
  brand: {
    primary: brand.colors.primary,
    primaryForeground: brand.colors.primaryForeground,
    navy: brand.colors.navy,
    muted: brand.colors.lightGray,
    accent: brand.colors.primary,
    radius: "0.875rem",
  },
} as const;
