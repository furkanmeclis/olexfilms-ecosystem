import { z } from "zod";

import type {
  AccountType,
  FinanceAccount,
  FinanceAccountCreateInput,
  FinanceAccountUpdateInput,
  FinanceSettlementInput,
} from "@/features/accounting/services/accounting.service";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Uppercase, spaces removed (the backend stores the same form). */
export function normalizeIban(raw: string): string {
  return raw.replace(/\s+/g, "").toUpperCase();
}

/**
 * Structural IBAN check (ISO 13616 mod-97). The backend also checks the
 * country length; this only catches typos before the request.
 */
export function isValidIban(raw: string): boolean {
  const iban = normalizeIban(raw);
  if (!/^[A-Z]{2}\d{2}[A-Z0-9]{11,30}$/.test(iban)) return false;
  const moved = iban.slice(4) + iban.slice(0, 4);
  let rest = 0;
  for (const ch of moved) {
    const code = ch.charCodeAt(0);
    const digits = code >= 65 ? String(code - 55) : ch;
    for (const d of digits) rest = (rest * 10 + Number(d)) % 97;
  }
  return rest === 1;
}

/**
 * Amount typed in any locale: "1.500,50", "1,500.50", "1500.5" → "1500.50".
 * The last separator followed by one or two digits is the decimal mark;
 * other separators are grouping. Returns null when it is not a positive
 * amount with at most two fraction digits (AccountingAmountInput).
 */
export function normalizeAmount(raw: string): string | null {
  const value = raw.replace(/[\s  ']/g, "");
  if (!/^[0-9.,]+$/.test(value)) return null;
  const match = /^(.*?)(?:[.,](\d{1,2}))?$/.exec(value);
  if (!match) return null;
  const intPart = (match[1] ?? "").replace(/[.,]/g, "");
  const frac = match[2] ?? "";
  if (!/^\d{1,16}$/.test(intPart)) return null;
  const amount = `${intPart.replace(/^0+(?=\d)/, "")}.${frac.padEnd(2, "0")}`;
  return Number(amount) > 0 ? amount : null;
}

// --- Account form ------------------------------------------------------------

export type AccountFormValues = {
  type: AccountType;
  name: string;
  iban: string;
  active: boolean;
};

export function accountFormSchema(t: Translate) {
  return z
    .object({
      type: z.enum(["cash", "bank"]),
      name: z
        .string()
        .trim()
        .min(1, t("accounting.validation.required"))
        .max(200, t("accounting.validation.too_long", { max: 200 })),
      iban: z.string(),
      active: z.boolean(),
    })
    .superRefine((v, ctx) => {
      if (v.type === "bank" && v.iban.trim() && !isValidIban(v.iban)) {
        ctx.addIssue({
          code: "custom",
          path: ["iban"],
          message: t("accounting.validation.iban"),
        });
      }
    });
}

export function accountDefaults(a?: FinanceAccount | null): AccountFormValues {
  return {
    type: a?.type ?? "cash",
    name: a?.name ?? "",
    iban: a?.iban ?? "",
    active: a?.active ?? true,
  };
}

export function accountCreateInput(
  v: AccountFormValues,
): FinanceAccountCreateInput {
  const iban = v.type === "bank" ? normalizeIban(v.iban) : "";
  return { type: v.type, name: v.name.trim(), iban: iban || null };
}

/** PATCH body: the type is fixed after creation; an empty IBAN clears it. */
export function accountUpdateInput(
  v: AccountFormValues,
): FinanceAccountUpdateInput {
  const iban = v.type === "bank" ? normalizeIban(v.iban) : "";
  return { name: v.name.trim(), iban: iban || null, active: v.active };
}

// --- Settlement (collection / payment) form ----------------------------------

export type SettlementFormValues = {
  account_uuid: string;
  cari_uuid: string;
  amount: string;
  currency: string;
  description: string;
};

export function settlementFormSchema(t: Translate) {
  return z.object({
    account_uuid: z.string().min(1, t("accounting.validation.account")),
    cari_uuid: z.string().min(1, t("accounting.validation.cari")),
    amount: z
      .string()
      .trim()
      .min(1, t("accounting.validation.required"))
      .refine((v) => normalizeAmount(v) !== null, {
        message: t("accounting.validation.amount"),
      }),
    currency: z
      .string()
      .trim()
      .regex(/^[A-Za-z]{3}$/, t("accounting.validation.currency")),
    description: z
      .string()
      .max(1000, t("accounting.validation.too_long", { max: 1000 })),
  });
}

export function settlementInput(
  v: SettlementFormValues,
  idempotencyKey: string,
): FinanceSettlementInput {
  const description = v.description.trim();
  return {
    account_uuid: v.account_uuid,
    cari_uuid: v.cari_uuid,
    amount: normalizeAmount(v.amount) ?? v.amount.trim(),
    currency: v.currency.trim().toUpperCase(),
    ...(description ? { description } : {}),
    idempotency_key: idempotencyKey,
  };
}

/** First message per field of a failed parse. */
export function fieldErrors(error: z.ZodError): Record<string, string> {
  const out: Record<string, string> = {};
  for (const issue of error.issues) {
    const key = String(issue.path[0] ?? "");
    if (key && !out[key]) out[key] = issue.message;
  }
  return out;
}
