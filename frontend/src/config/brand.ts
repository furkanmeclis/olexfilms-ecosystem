/**
 * Olexfilms — garanti, depo ve bayi ağı platformu.
 * TODO: brand colours/artwork are inherited from the base and not final.
 * Hex aliases for JS (theme-color, icons). CSS tokens live in theme.css.
 *
 * Source lockup: terracotta #EA6E43 + white (dark artboard). In light mode
 * the white glyph maps to `glyph` / `--brand-glyph`.
 */
export const brand = {
  name: "Olexfilms",
  productName: "Olexfilms",
  subtitle: "Ecosystem",
  tagline: "Garanti, depo ve bayi ağı platformu",
  colors: {
    /** Logo terracotta — exact SVG fill */
    primary: "#EA6E43",
    primaryForeground: "#FFF8F5",
    /** Warm espresso — light-mode stand-in for SVG white */
    glyph: "#241C18",
    /** Dark canvas behind the lockup */
    ink: "#1A1412",
    /** Warm cream surface */
    mist: "#FBF7F4",
    white: "#FFFFFF",
    /** Compat aliases used by theme-switch / legacy tokens */
    navy: "#1A1412",
    lightGray: "#F3EBE6",
  },
} as const;

export type BrandTone = "auto" | "light" | "dark";
export type BrandVariant = "logo" | "wordmark" | "mark" | "icon";
