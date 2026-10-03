import { z } from "zod";

export const PAPER_SIZES = ["A4", "A3", "Letter", "Legal"] as const;

export type PaperSize = (typeof PAPER_SIZES)[number];

/** i18n keys of the coordinate errors (TEC-242), shown by the map field. */
export const COORDINATE_ERRORS = {
  latitude: "settings.coordinates.errors.latitude",
  longitude: "settings.coordinates.errors.longitude",
  pair: "settings.coordinates.errors.pair",
} as const;

/**
 * Parses a typed coordinate ("41.0082", "41,0082"); null for an empty
 * value, NaN for anything that is not a plain decimal number.
 */
export function parseCoordinate(value: string | undefined): number | null {
  const v = (value ?? "").trim().replace(",", ".");
  if (v === "") return null;
  return /^[-+]?\d+(\.\d+)?$/.test(v) ? Number(v) : Number.NaN;
}

const settingsFormObject = z.object({
  company_name: z.string().trim().min(1),
  tagline: z.string().trim(),
  primary_color: z
    .string()
    .trim()
    .regex(/^#[0-9A-Fa-f]{6}$/, "Invalid color"),
  paper_size: z.enum(PAPER_SIZES),
  address: z.string().trim(),
  city: z.string().trim(),
  district: z.string().trim(),
  phone: z.string().trim(),
  email: z.string().trim(),
  website: z.string().trim(),
  footer_text: z.string().trim(),
  /** TEC-242 map position as typed (empty = not set); tenant scope only. */
  latitude: z.string(),
  longitude: z.string(),
});

/**
 * Latitude -90..90 and longitude -180..180 (same bounds as Go, TEC-240);
 * both empty clears the position, one without the other is an error.
 */
export const settingsFormSchema = settingsFormObject.superRefine(
  (values, ctx) => {
    const lat = parseCoordinate(values.latitude);
    const lng = parseCoordinate(values.longitude);
    if (lat === null && lng === null) return;
    if (lat !== null && (Number.isNaN(lat) || lat < -90 || lat > 90)) {
      ctx.addIssue({
        code: "custom",
        path: ["latitude"],
        message: COORDINATE_ERRORS.latitude,
      });
    }
    if (lng !== null && (Number.isNaN(lng) || lng < -180 || lng > 180)) {
      ctx.addIssue({
        code: "custom",
        path: ["longitude"],
        message: COORDINATE_ERRORS.longitude,
      });
    }
    if (lat === null || lng === null) {
      ctx.addIssue({
        code: "custom",
        path: [lat === null ? "latitude" : "longitude"],
        message: COORDINATE_ERRORS.pair,
      });
    }
  },
);

export type SettingsFormValues = z.infer<typeof settingsFormObject>;

export function normalizeHexColor(value: string, fallback: string): string {
  const trimmed = value.trim();
  if (/^#[0-9A-Fa-f]{6}$/.test(trimmed)) return trimmed;
  if (/^[0-9A-Fa-f]{6}$/.test(trimmed)) return `#${trimmed}`;
  return fallback;
}

function formatCoordinate(value: number | null | undefined): string {
  return typeof value === "number" && Number.isFinite(value)
    ? String(value)
    : "";
}

/** Rounds to 6 decimals (about 10 cm), enough for a shop's position. */
export function roundCoordinate(value: number): number {
  return Math.round(value * 1e6) / 1e6;
}

export function toSettingsFormValues(
  data: {
    company_name: string;
    tagline: string;
    primary_color: string;
    paper_size: string;
    address: string;
    city?: string;
    district?: string;
    phone: string;
    email: string;
    website: string;
    footer_text: string;
    latitude?: number | null;
    longitude?: number | null;
  },
  fallbackColor: string,
): SettingsFormValues {
  const paperSize = PAPER_SIZES.includes(data.paper_size as PaperSize)
    ? (data.paper_size as PaperSize)
    : "A4";

  return {
    company_name: data.company_name,
    tagline: data.tagline,
    primary_color: normalizeHexColor(data.primary_color, fallbackColor),
    paper_size: paperSize,
    address: data.address,
    city: data.city ?? "",
    district: data.district ?? "",
    phone: data.phone,
    email: data.email,
    website: data.website,
    footer_text: data.footer_text,
    latitude: formatCoordinate(data.latitude),
    longitude: formatCoordinate(data.longitude),
  };
}

/**
 * PATCH body. Coordinates are sent only for the tenant scope
 * (`withCoordinates`): both numbers, or both null to clear.
 */
export function toPatchPayload(
  values: SettingsFormValues,
  options: { withCoordinates?: boolean } = {},
) {
  const base = {
    company_name: values.company_name,
    tagline: values.tagline,
    primary_color: values.primary_color,
    paper_size: values.paper_size,
    address: values.address,
    city: values.city,
    district: values.district,
    phone: values.phone,
    email: values.email,
    website: values.website,
    footer_text: values.footer_text,
  };
  if (!options.withCoordinates) return base;
  const lat = parseCoordinate(values.latitude);
  const lng = parseCoordinate(values.longitude);
  return {
    ...base,
    latitude: lat === null ? null : roundCoordinate(lat),
    longitude: lng === null ? null : roundCoordinate(lng),
  };
}
