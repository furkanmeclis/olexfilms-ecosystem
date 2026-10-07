/**
 * Paths crawlers must not visit (TEC-251): dealer panel, customer portal,
 * BFF/API, platform admin and tenant pages, plus profile and share links.
 * TEC-320: public quote links carry an access token.
 */
export const ROBOTS_DISALLOW = [
  "/panel",
  "/portal",
  "/api",
  "/platform",
  "/t/",
  "/profile",
  "/share/",
  "/teklif/",
] as const;
