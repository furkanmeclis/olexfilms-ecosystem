import { permissions } from "@/config/permissions";

type Can = (permission: string) => boolean;

export type AccountingAccessInput = {
  can: Can;
  /** Type of the active organization (center | distributor | dealer). */
  orgType?: string | null;
  /** Enabled module keys of the active organization (null while unknown). */
  features?: readonly string[] | null;
};

export type AccountingAccess = {
  /** accounting.read: the book pages open. */
  canRead: boolean;
  /**
   * Accounts, collections and payments. Mirrors the backend write gate
   * (TEC-172 writeBook): accounting.write AND a center or distributor, or a
   * dealer with the dealer_accounting module (TEC-99 decision 7).
   */
  canWrite: boolean;
};

export function resolveAccountingAccess({
  can,
  orgType,
  features,
}: AccountingAccessInput): AccountingAccess {
  const canRead = can(permissions.accounting.read);
  const writableBook =
    orgType === "center" ||
    orgType === "distributor" ||
    (orgType === "dealer" && Boolean(features?.includes("dealer_accounting")));
  return {
    canRead,
    canWrite: canRead && writableBook && can(permissions.accounting.write),
  };
}
