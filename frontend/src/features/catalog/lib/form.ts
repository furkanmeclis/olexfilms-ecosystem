import { z } from "zod";

import type {
  CatalogCategory,
  CatalogCategoryInput,
  CatalogProduct,
  CatalogProductInput,
  CatalogUnitType,
} from "@/features/catalog/services/catalog.service";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** One entry per non-empty line, trimmed, duplicates dropped. */
export function parseLines(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || seen.has(line)) continue;
    seen.add(line);
    out.push(line);
  }
  return out;
}

export function categoryFormSchema(t: Translate) {
  return z.object({
    name: z
      .string()
      .trim()
      .min(1, t("catalog.validation.required"))
      .max(200, t("catalog.validation.too_long", { max: 200 })),
    available_parts: z
      .string()
      .refine((v) => parseLines(v).length <= 100, {
        message: t("catalog.validation.too_many", { max: 100 }),
      })
      .refine((v) => parseLines(v).every((p) => p.length <= 100), {
        message: t("catalog.validation.too_long", { max: 100 }),
      }),
    sort: z
      .string()
      .trim()
      .regex(/^-?\d{0,9}$/, t("catalog.validation.integer")),
    active: z.boolean(),
  });
}

export type CategoryFormValues = z.infer<ReturnType<typeof categoryFormSchema>>;

export function categoryDefaults(
  c?: CatalogCategory | null,
): CategoryFormValues {
  return {
    name: c?.name ?? "",
    available_parts: (c?.available_parts ?? []).join("\n"),
    sort: String(c?.sort ?? 0),
    active: c?.active ?? true,
  };
}

export function categoryInput(v: CategoryFormValues): CatalogCategoryInput {
  return {
    name: v.name.trim(),
    available_parts: parseLines(v.available_parts),
    sort: v.sort ? Number.parseInt(v.sort, 10) : 0,
    active: v.active,
  };
}

export function productFormSchema(t: Translate) {
  return z.object({
    category_uuid: z.string().min(1, t("catalog.validation.required")),
    sku: z
      .string()
      .trim()
      .min(1, t("catalog.validation.required"))
      .max(64, t("catalog.validation.too_long", { max: 64 })),
    name: z
      .string()
      .trim()
      .min(1, t("catalog.validation.required"))
      .max(200, t("catalog.validation.too_long", { max: 200 })),
    unit_type: z.enum(["piece", "roll_meter"]),
    uses_fixed_barcode: z.boolean(),
    warranty_duration_months: z
      .string()
      .trim()
      .refine(
        (v) => v === "" || (/^\d{1,3}$/.test(v) && Number(v) <= 600),
        t("catalog.validation.warranty"),
      ),
    micron_thickness: z
      .string()
      .trim()
      .refine((v) => {
        if (v === "") return true;
        const n = Number(v.replace(",", "."));
        return Number.isFinite(n) && n > 0 && n <= 999999.99;
      }, t("catalog.validation.micron")),
    images: z
      .string()
      .refine((v) => parseLines(v).length <= 20, {
        message: t("catalog.validation.too_many", { max: 20 }),
      })
      .refine((v) => parseLines(v).every((k) => k.length <= 512), {
        message: t("catalog.validation.too_long", { max: 512 }),
      }),
    description_md: z
      .string()
      .max(20000, t("catalog.validation.too_long", { max: 20000 })),
    active: z.boolean(),
  });
}

export type ProductFormValues = z.infer<ReturnType<typeof productFormSchema>>;

export function productDefaults(p?: CatalogProduct | null): ProductFormValues {
  const images = [...(p?.images ?? [])].sort((a, b) => a.sort - b.sort);
  return {
    category_uuid: p?.category.uuid ?? "",
    sku: p?.sku ?? "",
    name: p?.name ?? "",
    unit_type: (p?.unit_type ?? "piece") as CatalogUnitType,
    uses_fixed_barcode: p?.uses_fixed_barcode ?? false,
    warranty_duration_months:
      p?.warranty_duration_months === null ||
      p?.warranty_duration_months === undefined
        ? ""
        : String(p.warranty_duration_months),
    micron_thickness:
      p?.micron_thickness === null || p?.micron_thickness === undefined
        ? ""
        : String(p.micron_thickness),
    images: images.map((img) => img.key).join("\n"),
    description_md: p?.description_md ?? "",
    active: p?.active ?? true,
  };
}

/** Form values to the API body; empty optional numbers are sent as null (cleared). */
export function productInput(v: ProductFormValues): CatalogProductInput {
  const warranty = v.warranty_duration_months.trim();
  const micron = v.micron_thickness.trim().replace(",", ".");
  return {
    category_uuid: v.category_uuid,
    sku: v.sku.trim(),
    name: v.name.trim(),
    unit_type: v.unit_type,
    uses_fixed_barcode: v.uses_fixed_barcode,
    warranty_duration_months: warranty === "" ? null : Number(warranty),
    micron_thickness: micron === "" ? null : Number(micron),
    images: parseLines(v.images).map((key, sort) => ({ key, sort })),
    description_md: v.description_md,
    active: v.active,
  };
}
