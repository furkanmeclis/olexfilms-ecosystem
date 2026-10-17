import { z } from "zod";

import type {
  LabelLogoMode,
  LabelSymbology,
  LabelTemplate,
  LabelTemplateInput,
  LabelTemplateKind,
} from "@/features/stock/services/label-templates.service";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Backend limits (stock/usecase/label_templates.go). */
export const TEMPLATE_NAME_MAX = 100;
export const LOGO_TEXT_MAX = 100;
export const LOGO_IMAGE_MAX = 200_000;
export const WIDTH_MM = { min: 20, max: 200 } as const;
export const HEIGHT_MM = { min: 10, max: 200 } as const;
export const COLUMNS = { min: 1, max: 8 } as const;

export const TEMPLATE_KINDS: LabelTemplateKind[] = ["unit", "location"];
export const SYMBOLOGIES: LabelSymbology[] = ["code128", "qr"];
export const LOGO_MODES: LabelLogoMode[] = ["none", "text", "image"];

/** Built-in defaults of each kind (labels.DefaultUnit / DefaultLocation). */
export const KIND_DEFAULTS: Record<
  LabelTemplateKind,
  {
    symbology: LabelSymbology;
    width_mm: string;
    height_mm: string;
    columns: string;
  }
> = {
  unit: { symbology: "code128", width_mm: "70", height_mm: "37", columns: "3" },
  location: { symbology: "qr", width_mm: "50", height_mm: "50", columns: "4" },
};

const MM_RE = /^\d{1,3}(\.\d)?$/;
const INT_RE = /^\d+$/;
const LOGO_IMAGE_RE = /^data:image\/(png|jpeg);base64,[A-Za-z0-9+/=]+$/;

function mmField(t: Translate, range: { min: number; max: number }) {
  return z
    .string()
    .trim()
    .refine(
      (v) => MM_RE.test(v) && Number(v) >= range.min && Number(v) <= range.max,
      t("stock.label_templates.validation.mm", range),
    );
}

/**
 * Create / edit form of a label template. Mirrors templateArgs: name
 * 1-100, width 20-200 mm and height 10-200 mm (one decimal), 1-8 columns,
 * a logo text for logo_mode text, a PNG / JPEG data URI (at most 200 KB)
 * for logo_mode image; location labels are QR codes.
 */
export function labelTemplateSchema(t: Translate) {
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(1, t("stock.label_templates.validation.required"))
        .max(
          TEMPLATE_NAME_MAX,
          t("stock.label_templates.validation.too_long", {
            max: TEMPLATE_NAME_MAX,
          }),
        ),
      kind: z.enum(["unit", "location"]),
      symbology: z.enum(["code128", "qr"]),
      logo_mode: z.enum(["none", "text", "image"]),
      logo_text: z.string().trim(),
      logo_image: z.string(),
      width_mm: mmField(t, WIDTH_MM),
      height_mm: mmField(t, HEIGHT_MM),
      columns: z
        .string()
        .trim()
        .refine(
          (v) =>
            INT_RE.test(v) &&
            Number(v) >= COLUMNS.min &&
            Number(v) <= COLUMNS.max,
          t("stock.label_templates.validation.columns", COLUMNS),
        ),
      show_name: z.boolean(),
      show_code_text: z.boolean(),
      is_default: z.boolean(),
      active: z.boolean(),
    })
    .superRefine((v, ctx) => {
      if (v.kind === "location" && v.symbology !== "qr") {
        ctx.addIssue({
          code: "custom",
          path: ["symbology"],
          message: t("stock.label_templates.validation.location_qr"),
        });
      }
      if (v.logo_mode === "text") {
        if (!v.logo_text) {
          ctx.addIssue({
            code: "custom",
            path: ["logo_text"],
            message: t("stock.label_templates.validation.required"),
          });
        } else if (v.logo_text.length > LOGO_TEXT_MAX) {
          ctx.addIssue({
            code: "custom",
            path: ["logo_text"],
            message: t("stock.label_templates.validation.too_long", {
              max: LOGO_TEXT_MAX,
            }),
          });
        }
      }
      if (v.logo_mode === "image") {
        const msg = logoImageError(v.logo_image);
        if (msg) {
          ctx.addIssue({
            code: "custom",
            path: ["logo_image"],
            message: t(`stock.label_templates.validation.${msg}`),
          });
        }
      }
    });
}

export type LabelTemplateValues = z.output<
  ReturnType<typeof labelTemplateSchema>
>;

/** Why a logo data URI is refused (validation key), or null. */
export function logoImageError(
  img: string,
): "logo_required" | "logo_type" | "logo_size" | null {
  if (!img) return "logo_required";
  if (img.length > LOGO_IMAGE_MAX) return "logo_size";
  if (!LOGO_IMAGE_RE.test(img)) return "logo_type";
  return null;
}

/** Form values of a new template of `kind`. */
export function emptyTemplateValues(
  kind: LabelTemplateKind = "unit",
): LabelTemplateValues {
  const d = KIND_DEFAULTS[kind];
  return {
    name: "",
    kind,
    symbology: d.symbology,
    logo_mode: "none",
    logo_text: "",
    logo_image: "",
    width_mm: d.width_mm,
    height_mm: d.height_mm,
    columns: d.columns,
    show_name: true,
    show_code_text: true,
    is_default: false,
    active: true,
  };
}

/** "70.0" → "70" for the form; other values as served. */
function mm(value: string): string {
  return value.replace(/\.0$/, "");
}

/** Form values of a saved template. */
export function templateValues(tpl: LabelTemplate): LabelTemplateValues {
  return {
    name: tpl.name,
    kind: tpl.kind,
    symbology: tpl.symbology,
    logo_mode: tpl.logo_mode,
    logo_text: tpl.logo_text ?? "",
    logo_image: tpl.logo_image ?? "",
    width_mm: mm(tpl.width_mm),
    height_mm: mm(tpl.height_mm),
    columns: String(tpl.columns),
    show_name: tpl.show_name,
    show_code_text: tpl.show_code_text,
    is_default: tpl.is_default,
    active: tpl.active,
  };
}

/**
 * The request body (POST and PUT replace every field; kind only counts on
 * create). Logo fields go only with their mode.
 */
export function toTemplateInput(v: LabelTemplateValues): LabelTemplateInput {
  return {
    name: v.name.trim(),
    kind: v.kind,
    symbology: v.kind === "location" ? "qr" : v.symbology,
    logo_mode: v.logo_mode,
    ...(v.logo_mode === "text" ? { logo_text: v.logo_text.trim() } : {}),
    ...(v.logo_mode === "image" ? { logo_image: v.logo_image } : {}),
    width_mm: v.width_mm.trim(),
    height_mm: v.height_mm.trim(),
    columns: Number(v.columns),
    show_name: v.show_name,
    show_code_text: v.show_code_text,
    is_default: v.is_default,
    active: v.active,
  };
}

/** Sample content of the preview (a unit or a location label). */
export const PREVIEW_SAMPLE: Record<
  LabelTemplateKind,
  { code: string; title: string; subtitle: string }
> = {
  unit: { code: "OLEX-00000001", title: "Olex PPF 190", subtitle: "15.00 m" },
  location: { code: "WH1-R1-A-01-01", title: "01", subtitle: "WH1 · R1" },
};

/** QR payload the printer encodes (labels.qrPayload). */
export function qrPayload(kind: LabelTemplateKind, code: string): string {
  return `${kind === "location" ? "OFW:LOC:" : "OFW:UNIT:"}${code}`;
}

/**
 * QR edge in mm like the printer (labels.qrSide): the smaller label side
 * minus the paddings and the text lines, at least 10 mm.
 */
export function qrSideMm(v: {
  width: number;
  height: number;
  showName: boolean;
  showCodeText: boolean;
  logo: boolean;
}): number {
  let side = Math.min(v.width, v.height) - 4;
  if (v.showCodeText) side -= 4;
  if (v.showName) side -= 6;
  if (v.logo) side -= 6;
  return Math.max(side, 10);
}
