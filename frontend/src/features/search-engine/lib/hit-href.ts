/**
 * Tenant-relative prefixes of search hit links. The catalog adapter
 * (TEC-145) links `/catalog/products/{uuid}`; the page lives under the tenant
 * shell at `/t/{slug}/catalog/products/{uuid}` (TEC-147). The TEC-213 record
 * indexes link their detail pages the same way.
 */
const TENANT_RELATIVE_PREFIXES = [
  "/catalog/",
  "/customers/",
  "/vehicles/",
  "/services/",
  "/warranties/",
  "/orders/",
];

/** Resolves a search hit href for the shell the palette runs in. */
export function resolveSearchHitHref(
  href: string,
  tenantSlug?: string | null,
): string {
  if (!tenantSlug) return href;
  const base = `/t/${encodeURIComponent(tenantSlug)}`;
  // Stock units have no detail page: open My stock filtered on the barcode.
  if (href.startsWith("/stock/units/")) {
    const barcode = decodeURIComponent(href.slice("/stock/units/".length));
    return `${base}/stock?barcode=${encodeURIComponent(barcode)}`;
  }
  // The tenant shell has no organization page yet: open that
  // organization's stock (a distributor's dealer, TEC-216).
  if (href.startsWith("/organizations/")) {
    const id = href.slice("/organizations/".length);
    return `${base}/stock?organization=${encodeURIComponent(id)}`;
  }
  if (!TENANT_RELATIVE_PREFIXES.some((prefix) => href.startsWith(prefix))) {
    return href;
  }
  return `${base}${href}`;
}
