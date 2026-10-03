import { z } from "zod";

import type { WarehouseLocationType } from "@/features/warehouse/services/warehouse.service";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Warehouse, room and location codes (WarehouseCode, TEC-201). */
export const WAREHOUSE_CODE_RE = /^[A-Z0-9_]{1,32}$/;
/** Barcode prefix (BarcodeBatchInput.prefix, TEC-202). */
export const BARCODE_PREFIX_RE = /^[A-Z0-9]{2,8}$/;
/** Roll length: positive decimal with up to two fraction digits. */
export const METERS_RE = /^\d{1,6}(\.\d{1,2})?$/;
export const BATCH_MAX = 1000;

/** The code as the backend stores it (trimmed, upper-cased). */
export function normalizeCode(value: string): string {
  return value.trim().toUpperCase();
}

function codeField(t: Translate) {
  return z
    .string()
    .transform(normalizeCode)
    .refine((v) => v.length > 0, t("warehouse.validation.required"))
    .refine(
      (v) => v.length === 0 || WAREHOUSE_CODE_RE.test(v),
      t("warehouse.validation.code"),
    );
}

function nameField(t: Translate) {
  return z
    .string()
    .trim()
    .max(200, t("warehouse.validation.too_long", { max: 200 }));
}

/**
 * One form for a warehouse, a room or a location (create and edit). The
 * address only matters for a warehouse, the type only for a new location;
 * `allowedTypes` is the set the chosen parent permits (allowedChildTypes).
 */
export function nodeFormSchema(
  t: Translate,
  allowedTypes: WarehouseLocationType[] = [],
) {
  return z
    .object({
      code: codeField(t),
      name: nameField(t),
      address: z
        .string()
        .trim()
        .max(500, t("warehouse.validation.too_long", { max: 500 })),
      type: z.string(),
      active: z.boolean(),
    })
    .superRefine((v, ctx) => {
      if (
        allowedTypes.length > 0 &&
        !allowedTypes.includes(v.type as WarehouseLocationType)
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["type"],
          message: t("warehouse.validation.type"),
        });
      }
    });
}

export type NodeFormInput = z.input<ReturnType<typeof nodeFormSchema>>;
export type NodeFormValues = z.output<ReturnType<typeof nodeFormSchema>>;

/** Parsed stock entry "generate new barcodes" line (center only, K14). */
export function generateLinesSchema(t: Translate, isRoll: boolean) {
  return z.object({
    product_uuid: z.string().min(1, t("warehouse.validation.product")),
    quantity: z
      .string()
      .trim()
      .regex(/^\d+$/, t("warehouse.validation.quantity", { max: BATCH_MAX }))
      .refine(
        (v) => Number(v) >= 1 && Number(v) <= BATCH_MAX,
        t("warehouse.validation.quantity", { max: BATCH_MAX }),
      ),
    meters: z
      .string()
      .trim()
      .refine(
        (v) => (isRoll ? METERS_RE.test(v) && Number(v) > 0 : true),
        t("warehouse.validation.meters"),
      ),
    prefix: z
      .string()
      .transform(normalizeCode)
      .refine(
        (v) => v === "" || BARCODE_PREFIX_RE.test(v),
        t("warehouse.validation.prefix"),
      ),
  });
}

export type GenerateLinesValues = z.output<
  ReturnType<typeof generateLinesSchema>
>;

/** Request body of a generate line / barcode batch. */
export function generateBody(v: GenerateLinesValues, isRoll: boolean) {
  return {
    product_uuid: v.product_uuid,
    quantity: Number(v.quantity),
    ...(isRoll ? { meters: v.meters } : {}),
    ...(v.prefix ? { prefix: v.prefix } : {}),
  };
}

export type EntryMode = "with_existing" | "generate_new";

/**
 * New stock entry (TEC-204): an active warehouse of the organization, a
 * mode the organization may use (generate_new only at the center, K14)
 * and an optional note.
 */
export function entryFormSchema(t: Translate, modes: EntryMode[]) {
  return z.object({
    warehouse_uuid: z.string().min(1, t("warehouse.validation.warehouse")),
    mode: z
      .string()
      .refine(
        (v) => modes.includes(v as EntryMode),
        t("warehouse.validation.mode"),
      ),
    note: z
      .string()
      .trim()
      .max(500, t("warehouse.validation.too_long", { max: 500 })),
  });
}

export type EntryFormValues = z.output<ReturnType<typeof entryFormSchema>>;

/** Barcodes typed or pasted one per line (or comma/space separated). */
export function parseBarcodes(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of text.split(/[\s,;]+/)) {
    const code = raw.trim();
    if (!code || seen.has(code)) continue;
    seen.add(code);
    out.push(code);
  }
  return out;
}

// --- TEC-232 (slice 2) -----------------------------------------------------

/**
 * New warehouse transfer (TEC-205): two different active warehouses of the
 * organization and an optional note.
 */
export function transferFormSchema(t: Translate) {
  return z
    .object({
      from_warehouse_uuid: z
        .string()
        .min(1, t("warehouse.validation.warehouse")),
      to_warehouse_uuid: z.string().min(1, t("warehouse.validation.warehouse")),
      note: z
        .string()
        .trim()
        .max(500, t("warehouse.validation.too_long", { max: 500 })),
    })
    .superRefine((v, ctx) => {
      if (
        v.from_warehouse_uuid &&
        v.from_warehouse_uuid === v.to_warehouse_uuid
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["to_warehouse_uuid"],
          message: t("warehouse.validation.same_warehouse"),
        });
      }
    });
}

export type TransferFormValues = z.output<
  ReturnType<typeof transferFormSchema>
>;

/** Quantity of a count scan: a whole number from 1 (empty means 1). */
export const COUNT_QTY_MAX = 100000;

export function countQuantityError(value: string, t: Translate): string | null {
  const v = value.trim();
  if (v === "") return null;
  if (!/^\d+$/.test(v) || Number(v) < 1 || Number(v) > COUNT_QTY_MAX) {
    return t("warehouse.validation.quantity", { max: COUNT_QTY_MAX });
  }
  return null;
}

export function countMetersError(value: string, t: Translate): string | null {
  const v = value.trim();
  if (v === "") return null;
  return METERS_RE.test(v) ? null : t("warehouse.validation.meters");
}
