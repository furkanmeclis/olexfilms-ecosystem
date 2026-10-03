import { brand } from "@/config/brand";

/**
 * Public site URL used for canonical links, Open Graph, robots and sitemap.
 * Baked in at build time (static pages): the frontend Docker image receives
 * it as the `SITE_URL` build arg (defaults to `AUTH_URL` in compose.prod.yml).
 */
function resolveSiteUrl(): string {
  const raw =
    process.env.NEXT_PUBLIC_SITE_URL ||
    process.env.AUTH_URL ||
    "http://localhost:3000";
  try {
    return new URL(raw).origin;
  } catch {
    return "http://localhost:3000";
  }
}

export const site = {
  url: resolveSiteUrl(),
  name: brand.productName,
  locale: "tr_TR",
  title: "Olexfilms",
  description: "Olexfilms garanti, depo ve bayi ağı platformu.",
  keywords: ["olexfilms", "ppf", "garanti", "bayi"],
  /** TEC-251: default Open Graph image (local asset, 1200x630). */
  ogImage: {
    url: "/images/og-default.png",
    width: 1200,
    height: 630,
    alt: "Olex Films",
  },
} as const;
