import { z } from "zod";

import { seriesError } from "@/features/einvoice/lib/einvoice";
import type {
  EinvoiceSettings,
  EinvoiceSettingsRequest,
} from "@/features/einvoice/services/einvoice.service";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const V = "einvoice.settings.validation";

function seriesField(t: Translate) {
  return z
    .string()
    .transform((v) => v.trim().toUpperCase())
    .superRefine((v, ctx) => {
      const err = seriesError(v);
      if (err)
        ctx.addIssue({ code: "custom", message: t(`${V}.series_${err}`) });
    });
}

const optional = (max: number, t: Translate) =>
  z
    .string()
    .trim()
    .max(max, t(`${V}.too_long`, { max }));

/**
 * Seller profile and series (PUT /v1/einvoices/settings). Mirrors the
 * backend checks so a wrong series never leaves the form: exactly 3
 * characters A-Z / 0-9, not the reserved TMP, the two series differ.
 */
export function settingsFormSchema(t: Translate) {
  const required = (max: number) =>
    z
      .string()
      .trim()
      .min(1, t(`${V}.required`))
      .max(max, t(`${V}.too_long`, { max }));
  return z
    .object({
      vkn: z
        .string()
        .trim()
        .regex(/^[0-9]{10}$/, t(`${V}.vkn`)),
      tax_office: required(120),
      legal_name: required(255),
      address: required(2000),
      city: required(100),
      district: required(100),
      iban: optional(34, t),
      email: optional(255, t),
      phone: optional(20, t),
      website: optional(255, t),
      trade_registry_no: optional(64, t),
      mersis_no: z
        .string()
        .trim()
        .regex(/^([0-9]{16})?$/, t(`${V}.mersis`)),
      default_note: optional(4000, t),
      earchive_series: seriesField(t),
      efatura_series: seriesField(t),
      pdf_enabled: z.boolean(),
    })
    .superRefine((v, ctx) => {
      if (v.earchive_series && v.earchive_series === v.efatura_series) {
        ctx.addIssue({
          code: "custom",
          path: ["efatura_series"],
          message: t(`${V}.series_same`),
        });
      }
    });
}

export type SettingsFormValues = z.infer<ReturnType<typeof settingsFormSchema>>;

export function settingsDefaults(s?: EinvoiceSettings): SettingsFormValues {
  return {
    vkn: s?.vkn ?? "",
    tax_office: s?.tax_office ?? "",
    legal_name: s?.legal_name ?? "",
    address: s?.address ?? "",
    city: s?.city ?? "",
    district: s?.district ?? "",
    iban: s?.iban ?? "",
    email: s?.email ?? "",
    phone: s?.phone ?? "",
    website: s?.website ?? "",
    trade_registry_no: s?.trade_registry_no ?? "",
    mersis_no: s?.mersis_no ?? "",
    default_note: s?.default_note ?? "",
    earchive_series: s?.earchive_series ?? "EAR",
    efatura_series: s?.efatura_series ?? "EFN",
    pdf_enabled: s?.pdf_enabled ?? true,
  };
}

/** Request body: empty optional fields are left out. */
export function settingsBody(v: SettingsFormValues): EinvoiceSettingsRequest {
  const body: EinvoiceSettingsRequest = {
    vkn: v.vkn.trim(),
    tax_office: v.tax_office.trim(),
    legal_name: v.legal_name.trim(),
    address: v.address.trim(),
    city: v.city.trim(),
    district: v.district.trim(),
    earchive_series: v.earchive_series.trim().toUpperCase(),
    efatura_series: v.efatura_series.trim().toUpperCase(),
    pdf_enabled: v.pdf_enabled,
  };
  const optionalKeys = [
    "iban",
    "email",
    "phone",
    "website",
    "trade_registry_no",
    "mersis_no",
    "default_note",
  ] as const;
  for (const key of optionalKeys) {
    const value = v[key].trim();
    if (value) body[key] = value;
  }
  return body;
}
