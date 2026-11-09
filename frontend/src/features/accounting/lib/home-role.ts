import { Permission } from "@/config/permissions";

/**
 * TEC-1070: an accounting account (center_accounting, distributor
 * accounting) opens the accounting home instead of the general tenant home.
 * The rule follows permissions, not role names: a center or distributor
 * member who reads the books but neither opens services nor reads the
 * warehouse. Combined accounts (center owner) keep the general home.
 */
export function isAccountingHomeRole(
  can: (permission: string) => boolean,
  orgType: string | null | undefined,
): boolean {
  if (orgType !== "center" && orgType !== "distributor") return false;
  return (
    can(Permission.AccountingRead) &&
    !can(Permission.ServicesWrite) &&
    !can(Permission.WarehouseRead)
  );
}

/** Seller orders received and not fully invoiced (TEC-1030 marks). */
export const QUEUE_TO_INVOICE = {
  status: "received",
  invoice_status: "pending,partial",
} as const;

/** Seller orders on the road, not invoiced yet. */
export const QUEUE_IN_TRANSIT = {
  status: "shipped,delivered",
  invoice_status: "pending,partial",
} as const;
