/**
 * Paths crawlers must not visit (TEC-251): dealer panel, customer portal,
 * BFF/API, platform admin and tenant pages, plus profile and share links.
 */
export const ROBOTS_DISALLOW = [
  "/panel",
  "/portal",
  "/api",
  "/platform",
  "/t/",
  "/profile",
  "/share/",
] as const;
