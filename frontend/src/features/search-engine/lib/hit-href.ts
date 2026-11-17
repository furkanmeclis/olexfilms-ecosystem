import { entityHref, type EntityKind } from "@/config/routes";

/**
 * Tenant-relative search hit links (`/<segment>/<id>`, set by the backend
 * search adapters) and the entity kind each one opens. TEC-1217: the target
 * page comes from the shared `entityHref` registry.
 */
const HIT_PREFIX_KINDS: ReadonlyArray<readonly [string, EntityKind]> = [
  ["/catalog/products/", "product"],
  ["/customers/", "customer"],
  ["/vehicles/", "vehicle"],
  ["/services/", "service"],
  ["/warranties/", "warranty"],
  ["/orders/", "order"],
  ["/leads/", "lead"],
  ["/organizations/", "organization"],
];

/** Tenant pages without a registry kind: only the shell prefix is added. */
const TENANT_RELATIVE_PREFIXES = ["/catalog/", "/fleets/"];

/** Resolves a search hit href for the shell the palette runs in. */
export function resolveSearchHitHref(
  href: string,
  tenantSlug?: string | null,
): string {
  if (!tenantSlug) return href;
  const slug = encodeURIComponent(tenantSlug);
  const base = `/t/${slug}`;
  // Barcode hits keep opening My stock filtered on the unit (TEC-213): the
  // scan-and-act flow lives there, not on the unit history page.
  if (href.startsWith("/stock/units/")) {
    const barcode = decodeURIComponent(href.slice("/stock/units/".length));
    return `${base}/stock?barcode=${encodeURIComponent(barcode)}`;
  }
  for (const [prefix, kind] of HIT_PREFIX_KINDS) {
    if (!href.startsWith(prefix)) continue;
    const id = decodeURIComponent(href.slice(prefix.length).split(/[?#/]/)[0]);
    const target = entityHref(kind, id, slug);
    if (target) return target;
    // An organization without a page: open its stock (TEC-216).
    if (kind === "organization") {
      return `${base}/stock?organization=${encodeURIComponent(id)}`;
    }
    return `${base}${href}`;
  }
  if (TENANT_RELATIVE_PREFIXES.some((prefix) => href.startsWith(prefix))) {
    return `${base}${href}`;
  }
  return href;
}
