import type { PortalService } from "@/features/portal/lib/portal-client";

/** Page size of /portal/vehicles (TEC-241). */
export const PORTAL_VEHICLE_PAGE_SIZE = 12;

type Named = { name: string } | null | undefined;

/** "Brand Model · 2021" of a vehicle or a service; empty parts are dropped. */
export function portalVehicleTitle(v: {
  car_brand: Named;
  car_model: Named;
  model_year: number | null;
}): string {
  const name = [v.car_brand?.name, v.car_model?.name]
    .filter((x): x is string => Boolean(x && x.trim()))
    .join(" ");
  return [name, v.model_year ? String(v.model_year) : ""]
    .filter(Boolean)
    .join(" · ");
}

/**
 * WhatsApp click-to-chat link of an E.164 number (wa.me wants digits only);
 * null when the number is missing or too short to be real.
 */
export function whatsappUrl(e164: string | null | undefined): string | null {
  const digits = (e164 ?? "").replace(/\D/g, "");
  if (digits.length < 8) return null;
  return `https://wa.me/${digits}`;
}

/**
 * Parts to highlight on the drawing: the parts of the focused product, or
 * the union of every applied part when no product is focused.
 */
export function highlightedParts(
  service: Pick<PortalService, "applied_parts" | "products">,
  focusedItemUuid: string | null,
): Set<string> {
  if (focusedItemUuid) {
    const product = service.products.find(
      (p) => p.service_item_uuid === focusedItemUuid,
    );
    if (product) return new Set(product.applied_parts);
  }
  return new Set(service.applied_parts);
}

/** Public warranty page path (TEC-248); the QR code points here. */
export function publicWarrantyPath(publicCode: string): string {
  return `/garanti/${encodeURIComponent(publicCode)}`;
}
