import {
  parsePhoneNumberFromString,
  type CountryCode,
} from "libphonenumber-js";

import type { AppLocale } from "@/config/i18n";
import type { Country, District, Province } from "@/features/geo/types";
import {
  parseRetryAfter,
  type PublicWarrantyRequest,
  type UpstreamFetcher,
} from "@/features/warranty/lib/public-warranty";
import type { components } from "@/generated/api";
import { translate } from "@/lib/i18n/messages";
import en from "@/locales/en";

import { publicUpstreamHeaders } from "./public-quote";

/**
 * Public dealer application form (TEC-320) on top of the TEC-317 endpoints:
 * `GET /v1/public/dealer-applications/config` (open or not) and
 * `POST /v1/public/dealer-applications` (202, 404 closed, 429 limited).
 */
export type DealerApplicationInput =
  components["schemas"]["DealerApplicationInput"];

export const DEALER_APPLICATION_PATH = "/bayi-basvuru";

/** Field caps of DealerApplicationInput. */
export const LIMITS = {
  company_name: 200,
  contact_name: 200,
  email: 255,
  message: 5000,
} as const;

export type DealerApplicationValues = {
  company_name: string;
  contact_name: string;
  phone: string;
  email: string;
  /** Picker values: string ids, "" = none. */
  country_id: string;
  province_id: string;
  district_id: string;
  message: string;
  kvkk_consent: boolean;
  /** Honeypot: hidden from people, bots fill it. */
  website: string;
};

export const EMPTY_APPLICATION: DealerApplicationValues = {
  company_name: "",
  contact_name: "",
  phone: "",
  email: "",
  country_id: "",
  province_id: "",
  district_id: "",
  message: "",
  kvkk_consent: false,
  website: "",
};

export type ApplicationField = keyof DealerApplicationValues;

/** Message key plus params. */
export type FieldError = { key: string; params?: Record<string, number> };

export type ApplicationErrors = Partial<Record<ApplicationField, FieldError>>;

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * E.164 of a phone typed in any notation, read in the selected country
 * (same rule as Go, K29); null when it is not a valid number.
 */
export function normalizePhone(
  raw: string,
  countryIso2: string | null | undefined,
): string | null {
  const value = raw.trim();
  if (!value) return null;
  const parsed = parsePhoneNumberFromString(
    value,
    (countryIso2?.toUpperCase() || undefined) as CountryCode | undefined,
  );
  return parsed?.isValid() ? parsed.number : null;
}

/** Client checks before the request; Go validates again. */
export function validateApplication(
  values: DealerApplicationValues,
  countryIso2: string | null | undefined,
): ApplicationErrors {
  const errors: ApplicationErrors = {};
  const text = (field: keyof typeof LIMITS, required: boolean) => {
    const v = values[field].trim();
    if (required && !v)
      errors[field] = { key: "landing.dealer_application.errors.required" };
    else if (v.length > LIMITS[field])
      errors[field] = {
        key: "landing.dealer_application.errors.too_long",
        params: { max: LIMITS[field] },
      };
  };
  text("company_name", true);
  text("contact_name", true);
  text("email", false);
  text("message", false);
  if (!errors.email && values.email.trim() && !EMAIL_RE.test(values.email))
    errors.email = { key: "landing.dealer_application.errors.email_invalid" };
  if (!values.country_id)
    errors.country_id = { key: "landing.dealer_application.errors.required" };
  if (!values.phone.trim())
    errors.phone = { key: "landing.dealer_application.errors.required" };
  else if (!normalizePhone(values.phone, countryIso2))
    errors.phone = { key: "landing.dealer_application.errors.phone_invalid" };
  if (!values.kvkk_consent)
    errors.kvkk_consent = {
      key: "landing.dealer_application.errors.kvkk_required",
    };
  return errors;
}

function positiveId(value: string): number | null {
  const n = Number(value);
  return Number.isInteger(n) && n > 0 ? n : null;
}

/** Request body; the phone goes as E.164 when it parses. */
export function toApplicationPayload(
  values: DealerApplicationValues,
  countryIso2: string | null | undefined,
  locale: AppLocale,
): DealerApplicationInput {
  const province = positiveId(values.province_id);
  const payload: DealerApplicationInput = {
    company_name: values.company_name.trim(),
    contact_name: values.contact_name.trim(),
    phone: normalizePhone(values.phone, countryIso2) ?? values.phone.trim(),
    country_id: positiveId(values.country_id) ?? 0,
    province_id: province,
    district_id: province ? positiveId(values.district_id) : null,
    kvkk_consent: values.kvkk_consent,
    language: locale,
    website: values.website,
  };
  const email = values.email.trim();
  if (email) payload.email = email;
  const message = values.message.trim();
  if (message) payload.message = message;
  return payload;
}

export type SubmitResult =
  | { kind: "ok" }
  | { kind: "validation"; fields: ApplicationField[] }
  | { kind: "rate_limited"; retryAfter: number | null }
  | { kind: "closed" }
  | { kind: "error" };

const APPLICATION_FIELDS = new Set<string>(Object.keys(EMPTY_APPLICATION));

/** Posts the form through the panel BFF (public paths need no session). */
export async function submitDealerApplication(
  payload: DealerApplicationInput,
  fetchImpl: typeof fetch = fetch,
): Promise<SubmitResult> {
  let res: Response;
  try {
    res = await fetchImpl("/api/v1/public/dealer-applications", {
      method: "POST",
      credentials: "same-origin",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    });
  } catch {
    return { kind: "error" };
  }
  if (res.status === 202 || res.status === 200) return { kind: "ok" };
  if (res.status === 404) return { kind: "closed" };
  if (res.status === 429) {
    return {
      kind: "rate_limited",
      retryAfter: parseRetryAfter(res.headers.get("retry-after")),
    };
  }
  if (res.status === 400) {
    const body = (await res.json().catch(() => null)) as {
      error?: { details?: { field?: string }[] };
    } | null;
    const fields = (body?.error?.details ?? [])
      .map((d) => d.field ?? "")
      .filter((f): f is ApplicationField => APPLICATION_FIELDS.has(f));
    return { kind: "validation", fields };
  }
  return { kind: "error" };
}

async function geoItems<T>(
  path: string,
  fetchImpl: typeof fetch,
  signal?: AbortSignal,
): Promise<T[]> {
  const res = await fetchImpl(`/api/v1/public/geo/${path}`, {
    method: "GET",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    signal,
  });
  if (!res.ok) throw new Error(`geo ${res.status}`);
  const body = (await res.json()) as { data?: { items?: T[] } };
  return body.data?.items ?? [];
}

/** Anonymous geo pickers (TEC-320 public routes). */
export const publicGeo = {
  countries: (fetchImpl: typeof fetch = fetch, signal?: AbortSignal) =>
    geoItems<Country>("countries", fetchImpl, signal),
  provinces: (
    iso2: string,
    fetchImpl: typeof fetch = fetch,
    signal?: AbortSignal,
  ) =>
    geoItems<Province>(
      `countries/${encodeURIComponent(iso2)}/provinces`,
      fetchImpl,
      signal,
    ),
  districts: (
    provinceId: number,
    fetchImpl: typeof fetch = fetch,
    signal?: AbortSignal,
  ) =>
    geoItems<District>(`provinces/${provinceId}/districts`, fetchImpl, signal),
};

export type ApplicationConfig = "open" | "closed" | "error";

/**
 * Whether the form is open (server side). 200 `enabled:false` and 404 (no
 * brand on this host) are closed; anything else is error.
 */
export async function fetchDealerApplicationConfig(
  request: PublicWarrantyRequest,
  fetcher: UpstreamFetcher,
): Promise<ApplicationConfig> {
  try {
    const res = await fetcher("public/dealer-applications/config", {
      method: "GET",
      headers: publicUpstreamHeaders(request),
    });
    if (res.status === 404) return "closed";
    if (res.status !== 200) return "error";
    const envelope = JSON.parse(new TextDecoder().decode(res.body)) as {
      data?: { enabled?: unknown };
    };
    return envelope.data?.enabled === true ? "open" : "closed";
  } catch {
    return "error";
  }
}

/**
 * The form's texts in the page language (`?lang=` may differ from the
 * session language the client providers carry), handed to the client
 * island: `{ "landing.dealer_application.title": "...", ... }`.
 */
export function applicationMessages(locale: AppLocale): Record<string, string> {
  const out: Record<string, string> = {};
  for (const key of Object.keys(en.landing ?? {})) {
    if (!key.startsWith("dealer_application.")) continue;
    out[`landing.${key}`] = translate(locale, `landing.${key}`);
  }
  return out;
}
