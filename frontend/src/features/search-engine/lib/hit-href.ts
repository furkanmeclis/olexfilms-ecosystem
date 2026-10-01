/**
 * Tenant-relative prefixes of search hit links. The catalog adapter
 * (TEC-145) links `/catalog/products/{uuid}`; the page lives under the tenant
 * shell at `/t/{slug}/catalog/products/{uuid}` (TEC-147).
 */
const TENANT_RELATIVE_PREFIXES = ["/catalog/"];

/** Resolves a search hit href for the shell the palette runs in. */
export function resolveSearchHitHref(
  href: string,
  tenantSlug?: string | null,
): string {
  if (!tenantSlug) return href;
  if (!TENANT_RELATIVE_PREFIXES.some((prefix) => href.startsWith(prefix))) {
    return href;
  }
  return `/t/${encodeURIComponent(tenantSlug)}${href}`;
}
