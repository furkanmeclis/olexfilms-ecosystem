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

/** Error codes of the transfer flow (TEC-190, reused by TEC-243). */
const TRANSFER_ERROR_KEYS: Record<string, string> = {
  VEHICLE_TRANSFER_LOCKED: "portal.transfer.error.locked",
  VEHICLE_TRANSFER_EXPIRED: "portal.transfer.error.expired",
  VEHICLE_TRANSFER_NOT_PENDING: "portal.transfer.error.not_pending",
  VEHICLE_TRANSFER_PENDING: "portal.transfer.error.pending",
  VEHICLE_TRANSFER_SAME_OWNER: "portal.transfer.error.same_owner",
  VEHICLE_TRANSFER_OWNER_NO_PHONE: "portal.transfer.error.no_phone",
  VEHICLE_TRANSFER_DELIVERY_FAILED: "portal.transfer.error.delivery",
  VEHICLE_TRANSFER_UNAVAILABLE: "portal.transfer.error.unavailable",
  VEHICLE_TRANSFER_OWNER_CHANGED: "portal.transfer.error.not_pending",
};

/**
 * The message of a failed transfer request (TEC-243): a wrong code carries
 * the attempts left (details[].code), the other codes have one text each.
 */
export function portalTransferError(err: {
  status?: number;
  code?: string | null;
  details?: { field?: string; code?: string }[];
}): { key: string; params?: Record<string, number> } {
  if (err.code === "VEHICLE_TRANSFER_INVALID_CODE") {
    const left = Number(err.details?.[0]?.code);
    return {
      key: "portal.transfer.error.invalid_code",
      params: { count: Number.isFinite(left) ? left : 0 },
    };
  }
  if (err.code && TRANSFER_ERROR_KEYS[err.code]) {
    return { key: TRANSFER_ERROR_KEYS[err.code] };
  }
  if (err.code === "VALIDATION_ERROR") {
    const field = err.details?.[0]?.field;
    if (field === "phone") return { key: "portal.transfer.error.phone" };
    if (field === "new_owner_name")
      return { key: "portal.transfer.error.name" };
    return { key: "portal.transfer.error.code_format" };
  }
  if (err.status === 429) return { key: "portal.transfer.error.rate" };
  if (err.status === 404) return { key: "portal.transfer.error.not_found" };
  return { key: "portal.transfer.error.generic" };
}

/** Public warranty page path (TEC-248); the QR code points here. */
export function publicWarrantyPath(publicCode: string): string {
  return `/garanti/${encodeURIComponent(publicCode)}`;
}

/** Query key of the portal session hydration (GET /v1/auth/me). */
export const PORTAL_ME_KEY = ["portal", "me"] as const;

/**
 * Whether the portal session is a read-only fleet account (TEC-245): the
 * fleet role without the customer role. The API refuses the writes with
 * 403 PORTAL_READ_ONLY; the UI hides them.
 */
export function isPortalReadOnly(
  roles: readonly string[] | undefined,
): boolean {
  if (!roles) return false;
  return roles.includes("fleet") && !roles.includes("customer");
}

/** Page size of /portal/contracts (TEC-245). */
export const PORTAL_CONTRACT_PAGE_SIZE = 20;
