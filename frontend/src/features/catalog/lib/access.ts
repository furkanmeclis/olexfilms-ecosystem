import { permissions } from "@/config/permissions";

type Can = (permission: string) => boolean;

export type CatalogAccessInput = {
  can: Can;
  /** Type of the active organization (center | distributor | dealer). */
  orgType?: string | null;
};

export type CatalogAccess = {
  canRead: boolean;
  /** catalog.write AND the active organization is the brand center (K4). */
  canWrite: boolean;
};

export type PricingAccess = {
  /** Any pricing.*.read grant: the effective price card is shown. */
  canView: boolean;
  /** Center edits its list price (purchase + sale to distributors). */
  canWriteList: boolean;
  /** Center also edits the recommended retail price. */
  canWriteRecommended: boolean;
  /** Center lists distributor-specific prices (pricing.sale.read). */
  canReadDistributorPrices: boolean;
  /** Center writes distributor-specific prices. */
  canWriteDistributorPrices: boolean;
  /** Distributor writes its price to its dealers. */
  canWriteDealerPrice: boolean;
};

/**
 * Catalog controls mirror the API (TEC-145): reads need catalog.read; writes
 * need catalog.write and the brand center as active organization, so a
 * distributor or dealer with a stray write grant still sees a read-only UI.
 */
export function resolveCatalogAccess({
  can,
  orgType,
}: CatalogAccessInput): CatalogAccess {
  const canRead = can(permissions.catalog.read);
  return {
    canRead,
    canWrite: canRead && orgType === "center" && can(permissions.catalog.write),
  };
}

/** Pricing controls mirror the TEC-146 routes (org type + permission). */
export function resolvePricingAccess({
  can,
  orgType,
}: CatalogAccessInput): PricingAccess {
  const p = permissions.pricing;
  const center = orgType === "center";
  const distributor = orgType === "distributor";
  const saleWrite = can(p.saleWrite);
  return {
    canView: can(p.purchaseRead) || can(p.saleRead) || can(p.recommendedRead),
    canWriteList: center && saleWrite,
    canWriteRecommended: center && saleWrite && can(p.recommendedWrite),
    canReadDistributorPrices: center && can(p.saleRead),
    canWriteDistributorPrices: center && saleWrite,
    canWriteDealerPrice: distributor && saleWrite,
  };
}
