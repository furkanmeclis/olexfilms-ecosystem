import { permissions } from "@/config/permissions";

type Can = (permission: string) => boolean;

export type AccountingAccessInput = {
  can: Can;
  /** Type of the active organization (center | distributor | dealer). */
  orgType?: string | null;
  /** Enabled module keys of the active organization (null while unknown). */
  features?: readonly string[] | null;
  /** Parent organization of the active one (null for the center). */
  parentUuid?: string | null;
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
  /**
   * accounting.dispute and a parent to dispute against (TEC-174): only a
   * child organization disputes what its parent posted (K24).
   */
  canDispute: boolean;
  /**
   * accounting.resolve (TEC-174). The backend still limits it to the parent
   * the disputed row came from; the UI checks that per dispute.
   */
  canResolve: boolean;
  /**
   * A dealer without dealer_accounting: it reads its own cari and statement
   * and disputes rows, but writes nothing (TEC-195).
   */
  readOnlyDealer: boolean;
};

export function resolveAccountingAccess({
  can,
  orgType,
  features,
  parentUuid,
}: AccountingAccessInput): AccountingAccess {
  const canRead = can(permissions.accounting.read);
  const dealerWrites =
    orgType === "dealer" && Boolean(features?.includes("dealer_accounting"));
  const writableBook =
    orgType === "center" || orgType === "distributor" || dealerWrites;
  return {
    canRead,
    canWrite: canRead && writableBook && can(permissions.accounting.write),
    canDispute:
      canRead && Boolean(parentUuid) && can(permissions.accounting.dispute),
    canResolve:
      canRead &&
      (orgType === "center" || orgType === "distributor") &&
      can(permissions.accounting.resolve),
    readOnlyDealer: canRead && orgType === "dealer" && !dealerWrites,
  };
}
