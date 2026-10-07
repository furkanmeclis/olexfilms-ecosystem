"use client";

import { CircleCheck, Send } from "lucide-react";
import Link from "next/link";
import {
  useEffect,
  useId,
  useMemo,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { AppLocale } from "@/config/i18n";
import { countryName } from "@/features/geo/hooks/use-geo";
import type { Country, District, Province } from "@/features/geo/types";
import { cn } from "@/lib/utils";

import {
  EMPTY_APPLICATION,
  LIMITS,
  publicGeo,
  submitDealerApplication,
  toApplicationPayload,
  validateApplication,
  type ApplicationErrors,
  type ApplicationField,
  type DealerApplicationValues,
} from "../lib/dealer-application";

export type DealerApplicationFormProps = {
  locale: AppLocale;
  /** landing.dealer_application.* texts, translated on the server. */
  messages: Record<string, string>;
  /** Landing link of the success screen (keeps `?lang=`). */
  homeHref: string;
  /** Injected in tests. */
  fetchImpl?: typeof fetch;
};

type Load<T> = { items: T[]; loading: boolean; failed: boolean };

const IDLE = { items: [], loading: false, failed: false };

type Notice =
  | { kind: "rate_limited"; retryAfter: number | null }
  | { kind: "closed" }
  | { kind: "failed" };

const SELECT_CLASS =
  "border-border bg-background focus-visible:ring-ring flex h-10 w-full rounded-md border px-3 text-sm shadow-sm focus-visible:ring-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive";

/**
 * Public dealer application form (TEC-320). A client island of the server
 * page; its texts arrive translated in the page language. Country >
 * province > district come from the anonymous geo pickers; the phone is
 * checked with libphonenumber in the selected country. Submit stays
 * disabled until the KVKK consent is ticked. `website` is the hidden
 * honeypot of the API.
 */
export function DealerApplicationForm({
  locale,
  messages,
  homeHref,
  fetchImpl,
}: DealerApplicationFormProps) {
  const doFetch = fetchImpl ?? fetch;
  const t = (key: string, params?: Record<string, string | number>) => {
    let text = messages[key] ?? key;
    for (const [k, v] of Object.entries(params ?? {}))
      text = text.replaceAll(`{{${k}}}`, String(v));
    return text;
  };
  const formId = useId();
  const [values, setValues] =
    useState<DealerApplicationValues>(EMPTY_APPLICATION);
  const [errors, setErrors] = useState<ApplicationErrors>({});
  const [submitting, setSubmitting] = useState(false);
  const [notice, setNotice] = useState<Notice | null>(null);
  const [done, setDone] = useState(false);
  const [countries, setCountries] = useState<Load<Country>>({
    ...IDLE,
    loading: true,
  });
  const [provinces, setProvinces] = useState<Load<Province>>(IDLE);
  const [districts, setDistricts] = useState<Load<District>>(IDLE);

  useEffect(() => {
    const controller = new AbortController();
    publicGeo
      .countries(doFetch, controller.signal)
      .then((items) => setCountries({ items, loading: false, failed: false }))
      .catch(() => {
        if (!controller.signal.aborted)
          setCountries({ items: [], loading: false, failed: true });
      });
    return () => controller.abort();
  }, [doFetch]);

  const country = countries.items.find(
    (c) => String(c.id) === values.country_id,
  );
  const province = provinces.items.find(
    (p) => String(p.id) === values.province_id,
  );

  const countryOptions = useMemo(
    () =>
      countries.items
        .map((c) => ({ value: String(c.id), label: countryName(c, locale) }))
        .sort((a, b) => a.label.localeCompare(b.label, locale)),
    [countries.items, locale],
  );

  const set = <K extends ApplicationField>(
    field: K,
    value: DealerApplicationValues[K],
  ) => {
    setValues((prev) => ({ ...prev, [field]: value }));
    if (errors[field])
      setErrors((prev) => {
        const next = { ...prev };
        delete next[field];
        return next;
      });
  };

  const onCountry = (value: string) => {
    setValues((prev) => ({
      ...prev,
      country_id: value,
      province_id: "",
      district_id: "",
    }));
    setErrors((prev) => ({ ...prev, country_id: undefined, phone: undefined }));
    setDistricts(IDLE);
    const next = countries.items.find((c) => String(c.id) === value);
    if (!next?.has_provinces) {
      setProvinces(IDLE);
      return;
    }
    setProvinces({ items: [], loading: true, failed: false });
    publicGeo
      .provinces(next.iso2, doFetch)
      .then((items) => setProvinces({ items, loading: false, failed: false }))
      .catch(() => setProvinces({ items: [], loading: false, failed: true }));
  };

  const onProvince = (value: string) => {
    setValues((prev) => ({ ...prev, province_id: value, district_id: "" }));
    const next = provinces.items.find((p) => String(p.id) === value);
    if (!next?.has_districts) {
      setDistricts(IDLE);
      return;
    }
    setDistricts({ items: [], loading: true, failed: false });
    publicGeo
      .districts(next.id, doFetch)
      .then((items) => setDistricts({ items, loading: false, failed: false }))
      .catch(() => setDistricts({ items: [], loading: false, failed: true }));
  };

  const onPhoneBlur = () => {
    if (!values.phone.trim()) return;
    const phoneError = validateApplication(values, country?.iso2).phone;
    setErrors((prev) => ({ ...prev, phone: phoneError }));
  };

  const onSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting) return;
    setNotice(null);
    const found = validateApplication(values, country?.iso2);
    setErrors(found);
    if (Object.values(found).some(Boolean)) return;
    setSubmitting(true);
    const result = await submitDealerApplication(
      toApplicationPayload(values, country?.iso2, locale),
      doFetch,
    );
    setSubmitting(false);
    switch (result.kind) {
      case "ok":
        setDone(true);
        return;
      case "validation": {
        const server: ApplicationErrors = {};
        for (const field of result.fields)
          server[field] = {
            key:
              field === "phone"
                ? "landing.dealer_application.errors.phone_invalid"
                : "landing.dealer_application.errors.invalid",
          };
        setErrors(server);
        if (!result.fields.length) setNotice({ kind: "failed" });
        return;
      }
      case "rate_limited":
        setNotice({ kind: "rate_limited", retryAfter: result.retryAfter });
        return;
      case "closed":
        setNotice({ kind: "closed" });
        return;
      default:
        setNotice({ kind: "failed" });
    }
  };

  if (done) {
    return (
      <section
        data-screen="success"
        role="status"
        className="flex flex-col items-center gap-3 py-6 text-center"
      >
        <span className="flex size-14 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600">
          <CircleCheck className="size-8" aria-hidden />
        </span>
        <h2 className="text-xl font-semibold">
          {t("landing.dealer_application.success_title")}
        </h2>
        <p className="text-muted-foreground max-w-md text-sm">
          {t("landing.dealer_application.success_body")}
        </p>
        <Button asChild variant="outline" className="mt-2">
          <Link href={homeHref}>
            {t("landing.dealer_application.back_home")}
          </Link>
        </Button>
      </section>
    );
  }

  const err = (field: ApplicationField) => {
    const e = errors[field];
    return e ? t(e.key, e.params) : undefined;
  };
  const id = (field: string) => `${formId}-${field}`;

  return (
    <form
      noValidate
      onSubmit={onSubmit}
      data-slot="dealer-application-form"
      className="relative flex flex-col gap-4"
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          id={id("company")}
          label={t("landing.dealer_application.company_name")}
          required
          error={err("company_name")}
        >
          <Input
            id={id("company")}
            name="company_name"
            autoComplete="organization"
            maxLength={LIMITS.company_name}
            value={values.company_name}
            aria-invalid={Boolean(errors.company_name) || undefined}
            aria-describedby={
              errors.company_name ? `${id("company")}-error` : undefined
            }
            onChange={(e) => set("company_name", e.target.value)}
            className="h-10"
          />
        </Field>
        <Field
          id={id("contact")}
          label={t("landing.dealer_application.contact_name")}
          required
          error={err("contact_name")}
        >
          <Input
            id={id("contact")}
            name="contact_name"
            autoComplete="name"
            maxLength={LIMITS.contact_name}
            value={values.contact_name}
            aria-invalid={Boolean(errors.contact_name) || undefined}
            aria-describedby={
              errors.contact_name ? `${id("contact")}-error` : undefined
            }
            onChange={(e) => set("contact_name", e.target.value)}
            className="h-10"
          />
        </Field>
      </div>

      <Field
        id={id("country")}
        label={t("landing.dealer_application.country")}
        required
        error={
          err("country_id") ??
          (countries.failed
            ? t("landing.dealer_application.geo_failed")
            : undefined)
        }
      >
        <select
          id={id("country")}
          name="country_id"
          value={values.country_id}
          disabled={countries.loading}
          aria-invalid={Boolean(errors.country_id) || undefined}
          aria-describedby={
            errors.country_id || countries.failed
              ? `${id("country")}-error`
              : undefined
          }
          onChange={(e) => onCountry(e.target.value)}
          className={SELECT_CLASS}
        >
          <option value="">
            {countries.loading
              ? t("landing.dealer_application.geo_loading")
              : t("landing.dealer_application.country_placeholder")}
          </option>
          {countryOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          id={id("province")}
          label={t("landing.dealer_application.province")}
          error={
            provinces.failed
              ? t("landing.dealer_application.geo_failed")
              : undefined
          }
        >
          <select
            id={id("province")}
            name="province_id"
            value={values.province_id}
            disabled={
              !country || provinces.loading || provinces.items.length === 0
            }
            onChange={(e) => onProvince(e.target.value)}
            className={SELECT_CLASS}
          >
            <option value="">
              {provinces.loading
                ? t("landing.dealer_application.geo_loading")
                : t("landing.dealer_application.province_placeholder")}
            </option>
            {provinces.items.map((p) => (
              <option key={p.id} value={String(p.id)}>
                {p.name}
              </option>
            ))}
          </select>
        </Field>
        <Field
          id={id("district")}
          label={t("landing.dealer_application.district")}
          error={
            districts.failed
              ? t("landing.dealer_application.geo_failed")
              : undefined
          }
        >
          <select
            id={id("district")}
            name="district_id"
            value={values.district_id}
            disabled={
              !province || districts.loading || districts.items.length === 0
            }
            onChange={(e) => set("district_id", e.target.value)}
            className={SELECT_CLASS}
          >
            <option value="">
              {districts.loading
                ? t("landing.dealer_application.geo_loading")
                : t("landing.dealer_application.district_placeholder")}
            </option>
            {districts.items.map((d) => (
              <option key={d.id} value={String(d.id)}>
                {d.name}
              </option>
            ))}
          </select>
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          id={id("phone")}
          label={t("landing.dealer_application.phone")}
          required
          hint={t("landing.dealer_application.phone_hint")}
          error={err("phone")}
        >
          <Input
            id={id("phone")}
            name="phone"
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            dir="ltr"
            value={values.phone}
            aria-invalid={Boolean(errors.phone) || undefined}
            aria-describedby={
              errors.phone ? `${id("phone")}-error` : `${id("phone")}-hint`
            }
            onChange={(e) => set("phone", e.target.value)}
            onBlur={onPhoneBlur}
            className="h-10 text-start"
          />
        </Field>
        <Field
          id={id("email")}
          label={`${t("landing.dealer_application.email")} (${t("landing.dealer_application.optional")})`}
          error={err("email")}
        >
          <Input
            id={id("email")}
            name="email"
            type="email"
            autoComplete="email"
            dir="ltr"
            maxLength={LIMITS.email}
            value={values.email}
            aria-invalid={Boolean(errors.email) || undefined}
            aria-describedby={errors.email ? `${id("email")}-error` : undefined}
            onChange={(e) => set("email", e.target.value)}
            className="h-10 text-start"
          />
        </Field>
      </div>

      <Field
        id={id("message")}
        label={`${t("landing.dealer_application.message")} (${t("landing.dealer_application.optional")})`}
        error={err("message")}
      >
        <Textarea
          id={id("message")}
          name="message"
          rows={4}
          maxLength={LIMITS.message}
          placeholder={t("landing.dealer_application.message_placeholder")}
          value={values.message}
          aria-invalid={Boolean(errors.message) || undefined}
          onChange={(e) => set("message", e.target.value)}
        />
      </Field>

      {/* Honeypot: off screen and out of the tab order; people leave it empty. */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute -start-[10000px] top-auto size-px overflow-hidden"
      >
        <label htmlFor={id("website")}>
          {t("landing.dealer_application.honeypot")}
        </label>
        <input
          id={id("website")}
          name="website"
          type="text"
          tabIndex={-1}
          autoComplete="off"
          value={values.website}
          onChange={(e) => set("website", e.target.value)}
        />
      </div>

      <div className="bg-muted/50 flex flex-col gap-3 rounded-xl p-4">
        <p className="text-muted-foreground text-xs">
          {t("landing.dealer_application.kvkk_notice")}
        </p>
        <div className="flex items-start gap-3">
          <Checkbox
            id={id("kvkk")}
            name="kvkk_consent"
            checked={values.kvkk_consent}
            aria-invalid={Boolean(errors.kvkk_consent) || undefined}
            onCheckedChange={(checked) => set("kvkk_consent", checked === true)}
            className="mt-0.5"
          />
          <label htmlFor={id("kvkk")} className="text-sm leading-snug">
            {t("landing.dealer_application.kvkk_consent")}
          </label>
        </div>
        {errors.kvkk_consent ? (
          <p role="alert" className="text-destructive text-sm">
            {err("kvkk_consent")}
          </p>
        ) : null}
      </div>

      {notice ? (
        <p
          role="alert"
          data-notice={notice.kind}
          className="border-destructive/40 bg-destructive/5 text-destructive rounded-lg border px-3 py-2 text-sm"
        >
          {notice.kind === "rate_limited"
            ? [
                t("landing.dealer_application.errors.rate_limited"),
                notice.retryAfter
                  ? t("landing.dealer_application.errors.rate_limited_retry", {
                      minutes: Math.max(1, Math.ceil(notice.retryAfter / 60)),
                    })
                  : "",
              ]
                .filter(Boolean)
                .join(" ")
            : notice.kind === "closed"
              ? t("landing.dealer_application.errors.closed")
              : t("landing.dealer_application.errors.failed")}
        </p>
      ) : null}

      <Button
        type="submit"
        size="lg"
        disabled={!values.kvkk_consent || submitting}
        className="h-11 w-full sm:w-auto sm:self-end"
      >
        <Send className="size-4 rtl:-scale-x-100" aria-hidden />
        {submitting
          ? t("landing.dealer_application.submitting")
          : t("landing.dealer_application.submit")}
      </Button>
    </form>
  );
}

function Field({
  id,
  label,
  required,
  hint,
  error,
  children,
}: {
  id: string;
  label: string;
  required?: boolean;
  hint?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
        {required ? (
          <span className="text-destructive ms-0.5" aria-hidden>
            *
          </span>
        ) : null}
      </label>
      {children}
      {error ? (
        <p id={`${id}-error`} role="alert" className="text-destructive text-sm">
          {error}
        </p>
      ) : hint ? (
        <p id={`${id}-hint`} className={cn("text-muted-foreground text-xs")}>
          {hint}
        </p>
      ) : null}
    </div>
  );
}
