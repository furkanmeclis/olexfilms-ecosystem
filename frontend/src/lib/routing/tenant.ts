/** Extract tenant slug from a path such as `/t/acme` or `/t/acme/finance`. */
export function parseTenantSlugFromPath(
  path: string | null | undefined,
): string | null {
  if (!path || !path.startsWith("/") || path.startsWith("//")) return null;
  const match = path.match(/^\/t\/([^/]+)/);
  return match?.[1] ?? null;
}
