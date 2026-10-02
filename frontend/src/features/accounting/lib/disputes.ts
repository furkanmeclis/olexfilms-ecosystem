import { z } from "zod";

import { normalizeAmount } from "@/features/accounting/lib/form";
import type {
  AccountingDispute,
  AccountingDisputeResolveInput,
  AccountingDisputeStatus,
  CariAccount,
  CariStatementLine,
  FinanceEntry,
} from "@/features/accounting/services/accounting.service";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

export const DISPUTE_TEXT_MAX = 2000;

function sourced(sourceType: string | null, sourceUuid: string | null) {
  return Boolean(sourceType) && sourceType !== "manual" && Boolean(sourceUuid);
}

/**
 * Mirrors OpenDispute (TEC-174): a sourced (not manual), original (not a
 * reversal) and still open (not reversed) row on the active organization's
 * cari with its parent. `parentUuid` is null for the center, which has no
 * one to dispute against.
 */
export function isEntryDisputable(
  entry: FinanceEntry,
  parentUuid: string | null | undefined,
): boolean {
  if (!parentUuid || !entry.cari_uuid) return false;
  if (entry.counterparty_organization?.uuid !== parentUuid) return false;
  if (entry.reversal_of_uuid || entry.voided || entry.reversed_by_uuid) {
    return false;
  }
  return sourced(entry.source_type, entry.source_uuid);
}

/** The same rule for a statement row of `cari`. */
export function isStatementLineDisputable(
  line: CariStatementLine,
  cari: CariAccount,
  parentUuid: string | null | undefined,
): boolean {
  if (!parentUuid) return false;
  if (
    cari.counterparty.type !== "organization" ||
    cari.counterparty.uuid !== parentUuid
  ) {
    return false;
  }
  if (line.reversal_of_uuid || line.reversed) return false;
  return sourced(line.source_type, line.source_uuid);
}

/**
 * The active organization resolves an open dispute addressed to it (the
 * parent the row came from), with accounting.resolve.
 */
export function canResolveDispute(
  dispute: AccountingDispute,
  orgUuid: string,
  canResolve: boolean,
): boolean {
  return (
    canResolve &&
    dispute.status === "open" &&
    Boolean(orgUuid) &&
    dispute.counterparty_organization.uuid === orgUuid
  );
}

export function disputeStatusTone(
  status: AccountingDisputeStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "open":
      return "warning";
    case "rejected":
      return "danger";
    default:
      return "success";
  }
}

// --- Open dispute form -------------------------------------------------------

export function disputeReasonSchema(t: Translate) {
  return z.object({
    reason: z
      .string()
      .trim()
      .min(1, t("accounting.validation.required"))
      .max(
        DISPUTE_TEXT_MAX,
        t("accounting.validation.too_long", { max: DISPUTE_TEXT_MAX }),
      ),
  });
}

// --- Resolve form ------------------------------------------------------------

export type DisputeResolution = AccountingDisputeResolveInput["resolution"];

export const DISPUTE_RESOLUTIONS: DisputeResolution[] = [
  "reversal",
  "revision",
  "reject",
];

export type ResolveFormValues = {
  resolution: DisputeResolution;
  corrected_amount: string;
  note: string;
};

/**
 * reversal: an optional note; revision: a positive corrected amount (in the
 * entry's original currency); reject: a note is required (TEC-174).
 */
export function resolveFormSchema(t: Translate) {
  return z
    .object({
      resolution: z.enum(["reversal", "revision", "reject"]),
      corrected_amount: z.string(),
      note: z
        .string()
        .max(
          DISPUTE_TEXT_MAX,
          t("accounting.validation.too_long", { max: DISPUTE_TEXT_MAX }),
        ),
    })
    .superRefine((v, ctx) => {
      if (v.resolution === "revision") {
        if (!v.corrected_amount.trim()) {
          ctx.addIssue({
            code: "custom",
            path: ["corrected_amount"],
            message: t("accounting.validation.required"),
          });
        } else if (normalizeAmount(v.corrected_amount) === null) {
          ctx.addIssue({
            code: "custom",
            path: ["corrected_amount"],
            message: t("accounting.validation.amount"),
          });
        }
      }
      if (v.resolution === "reject" && !v.note.trim()) {
        ctx.addIssue({
          code: "custom",
          path: ["note"],
          message: t("accounting.disputes.validation.note_required"),
        });
      }
    });
}

export function resolveInput(
  v: ResolveFormValues,
): AccountingDisputeResolveInput {
  const note = v.note.trim();
  return {
    resolution: v.resolution,
    ...(note ? { note } : {}),
    ...(v.resolution === "revision"
      ? {
          corrected_amount:
            normalizeAmount(v.corrected_amount) ?? v.corrected_amount.trim(),
        }
      : {}),
  };
}
