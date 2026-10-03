import { z } from "zod";

import type {
  GlorianConnection,
  GlorianConnectionInput,
} from "@/features/integrations/glorian/services/glorian.service";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Limits of PutInput.validate (glorianadmin usecase, TEC-273). */
export const BASE_URL_MAX = 500;
export const API_KEY_MAX = 500;
export const API_VERSION = "1" as const;

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function isHttpURL(value: string) {
  try {
    const u = new URL(value);
    return (u.protocol === "https:" || u.protocol === "http:") && !!u.host;
  } catch {
    return false;
  }
}

export type GlorianFormValues = {
  base_url: string;
  api_key: string;
  default_warehouse_uuid: string;
  active: boolean;
};

/**
 * Connection form schema. `apiKeySet` is whether a key is already stored:
 * a blank key keeps it, so activating needs either one.
 */
export function glorianFormSchema(t: Translate, apiKeySet: boolean) {
  const v = (key: string, params?: Record<string, string | number>) =>
    t(`integrations.glorian.validation.${key}`, params);
  return z
    .object({
      base_url: z
        .string()
        .trim()
        .min(1, v("required"))
        .max(BASE_URL_MAX, v("too_long", { max: BASE_URL_MAX }))
        .refine((s) => s === "" || isHttpURL(s), v("url")),
      api_key: z
        .string()
        .trim()
        .max(API_KEY_MAX, v("too_long", { max: API_KEY_MAX })),
      default_warehouse_uuid: z
        .string()
        .trim()
        .refine((s) => s === "" || UUID_RE.test(s), v("uuid")),
      active: z.boolean(),
    })
    .refine((d) => !d.active || apiKeySet || d.api_key.length > 0, {
      path: ["api_key"],
      message: v("api_key_required"),
    });
}

/** Form values of a connection; the key field always starts empty. */
export function toFormValues(conn: GlorianConnection): GlorianFormValues {
  return {
    base_url: conn.base_url ?? "",
    api_key: "",
    default_warehouse_uuid: conn.default_warehouse_uuid ?? "",
    active: conn.active,
  };
}

/** PUT body: a blank key is left out so the stored key stays. */
export function toPutBody(values: GlorianFormValues): GlorianConnectionInput {
  const key = values.api_key.trim();
  const warehouse = values.default_warehouse_uuid.trim();
  return {
    base_url: values.base_url.trim(),
    active: values.active,
    api_version: API_VERSION,
    default_warehouse_uuid: warehouse === "" ? null : warehouse,
    ...(key ? { api_key: key } : {}),
  };
}
