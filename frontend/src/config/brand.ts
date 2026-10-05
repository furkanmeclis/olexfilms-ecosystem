/**
 * Olexfilms — garanti, depo ve bayi ağı platformu.
 * Hex aliases for JS (theme-color, icons). CSS tokens live in theme.css and
 * follow the olexfilms-warehouse theme (deep green + gold).
 *
 * Source lockup (olex-logo-on-dark.svg): gold crest #F2AC4A with deep-green
 * star details #003D25 and a white "OLEX" wordmark. On light surfaces the
 * white glyph maps to `glyph` / `--brand-glyph`.
 */
export const brand = {
  name: "Olexfilms",
  productName: "Olexfilms",
  subtitle: "Ecosystem",
  tagline: "Garanti, depo ve bayi ağı platformu",
  colors: {
    /** Brand deep green — light-mode primary, logo star details */
    primary: "#003D25",
    primaryForeground: "#F8FBF9",
    /** Logo gold — exact SVG fill, sidebar primary */
    accent: "#F2AC4A",
    /** Light-mode stand-in for the white wordmark glyph */
    glyph: "#003D25",
    /** Dark canvas (dark-mode background) */
    ink: "#091910",
    /** Light canvas (light-mode background) */
    mist: "#F4F9F5",
    white: "#FFFFFF",
    /** Compat aliases used by theme-switch / legacy tokens */
    navy: "#091910",
    lightGray: "#E2F0E7",
  },
} as const;

export type BrandTone = "auto" | "light" | "dark";
export type BrandVariant = "logo" | "wordmark" | "mark" | "icon";
