"use client";

import { CircleCheck, Send } from "lucide-react";
import {
  useEffect,
  useId,
  useMemo,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { AppLocale } from "@/config/i18n";
import { normalizePhone } from "@/features/public-leads/lib/dealer-application";

import {
  fetchLeadFormConfig,
  submitPublicDealerLead,
  type PublicDealerLeadFormConfig,
  type PublicDealerLeadRequest,
} from "../lib/dealer-showcase";

type T = (key: string, params?: Record<string, string | number>) => string;

export type ShowcaseLeadFormProps = {
  code: string;
  locale: AppLocale;
  t: T;
  fetchImpl?: typeof fetch;
};

type Values = {
  name: string;
  phone: string;
  country: string;
  email: string;
  vehicle_brand: string;
  vehicle_model: string;
  interested_services: string[];
  message: string;
  preferred_channel: "phone" | "email" | "whatsapp";
  kvkk_consent: boolean;
  website: string;
};

const EMPTY: Values = {
  name: "",
  phone: "",
  country: "TR",
  email: "",
  vehicle_brand: "",
  vehicle_model: "",
  interested_services: [],
  message: "",
  preferred_channel: "whatsapp",
  kvkk_consent: false,
  website: "",
};

const COUNTRIES = ["TR", "DE", "BG", "FR", "ES", "IT", "AZ", "US"] as const;
const NO_SERVICES: PublicDealerLeadFormConfig["services"] = [];

type Field = keyof Values;
type Errors = Partial<Record<Field, string>>;

/**
 * Client island for the public showcase quote form. The surrounding dealer
 * page stays server-rendered; this component fetches the F5-01c form config
 * and posts through the public BFF route.
 */
export function ShowcaseLeadForm({
  code,
  locale,
  t,
  fetchImpl,
}: ShowcaseLeadFormProps) {
  const doFetch = fetchImpl ?? fetch;
  const formId = useId();
  const [config, setConfig] = useState<PublicDealerLeadFormConfig | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [values, setValues] = useState<Values>(EMPTY);
  const [errors, setErrors] = useState<Errors>({});
  const [submitting, setSubmitting] = useState(false);
  const [done, setDone] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    void fetchLeadFormConfig(code, locale, doFetch).then((result) => {
      if (!active) return;
      if (result.kind === "ok") {
        setConfig(result.config);
        setValues((prev) => ({
          ...prev,
          country: result.config.default_phone_country || prev.country,
        }));
      } else {
        setUnavailable(true);
      }
    });
    return () => {
      active = false;
    };
  }, [code, locale, doFetch]);

  const services = config?.services ?? NO_SERVICES;

  const set = <K extends Field>(field: K, value: Values[K]) => {
    setValues((prev) => ({ ...prev, [field]: value }));
    if (errors[field]) {
      setErrors((prev) => {
        const next = { ...prev };
        delete next[field];
        return next;
      });
    }
  };

  const serviceIds = useMemo(
    () => new Set(services.map((s) => s.uuid)),
    [services],
  );

  const validate = () => {
    const found: Errors = {};
    if (!values.name.trim()) found.name = t("portal.dealer_page.form_required");
    if (!values.phone.trim()) {
      found.phone = t("portal.dealer_page.form_required");
    } else if (!normalizePhone(values.phone, values.country)) {
      found.phone = t("portal.dealer_page.form_phone_invalid");
    }
    if (!values.kvkk_consent) {
      found.kvkk_consent = t("portal.dealer_page.form_kvkk_required");
    }
    if (
      values.email.trim() &&
      !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(values.email)
    ) {
      found.email = t("portal.dealer_page.form_email_invalid");
    }
    return found;
  };

  const onSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!config || submitting) return;
    setNotice(null);
    const found = validate();
    setErrors(found);
    if (Object.values(found).some(Boolean)) return;
    const payload: PublicDealerLeadRequest = {
      name: values.name.trim(),
      phone:
        normalizePhone(values.phone, values.country) ?? values.phone.trim(),
      interested_services: values.interested_services.filter((id) =>
        serviceIds.has(id),
      ),
      preferred_channel: values.preferred_channel,
      kvkk_consent: values.kvkk_consent,
      language: locale,
      form_token: config.form_token,
      website: values.website,
    };
    const email = values.email.trim();
    if (email) payload.email = email;
    const brand = values.vehicle_brand.trim();
    if (brand) payload.vehicle_brand = brand;
    const model = values.vehicle_model.trim();
    if (model) payload.vehicle_model = model;
    const message = values.message.trim();
    if (message) payload.message = message;
    setSubmitting(true);
    const result = await submitPublicDealerLead(code, payload, doFetch);
    setSubmitting(false);
    if (result.kind === "ok") {
      setDone(true);
      return;
    }
    if (result.kind === "validation") {
      const next: Errors = {};
      for (const field of result.fields) {
        if (field === "phone")
          next.phone = t("portal.dealer_page.form_phone_invalid");
        if (field === "kvkk_consent")
          next.kvkk_consent = t("portal.dealer_page.form_kvkk_required");
      }
      setErrors(next);
      setNotice(t("portal.dealer_page.form_validation"));
      return;
    }
    setNotice(
      result.kind === "rate_limited"
        ? result.retryAfter
          ? t("warranty.public.retry_after", { seconds: result.retryAfter })
          : t("portal.dealer_page.form_rate_limited")
        : t("portal.dealer_page.form_failed"),
    );
  };

  if (done) {
    return (
      <section
        data-screen="showcase-lead-success"
        className="border-border bg-card rounded-lg border p-4"
      >
        <CircleCheck className="text-primary mb-2 size-5" aria-hidden />
        <h2 className="font-semibold">
          {t("portal.dealer_page.form_success_title")}
        </h2>
        <p className="text-muted-foreground mt-1 text-sm">
          {t("portal.dealer_page.form_success_body")}
        </p>
      </section>
    );
  }

  if (unavailable) return null;

  return (
    <section data-slot="showcase-lead-form" className="space-y-3">
      <h2 className="text-base font-semibold">
        {t("portal.dealer_page.form_title")}
      </h2>
      {!config ? (
        <p className="text-muted-foreground text-sm">
          {t("portal.dealer_page.form_loading")}
        </p>
      ) : (
        <form className="space-y-3" onSubmit={onSubmit} noValidate>
          <Field
            label={t("portal.dealer_page.form_name")}
            id={`${formId}-name`}
            error={errors.name}
          >
            <Input
              id={`${formId}-name`}
              name="name"
              value={values.name}
              aria-invalid={Boolean(errors.name) || undefined}
              onChange={(e) => set("name", e.target.value)}
            />
          </Field>

          <div className="grid gap-3 sm:grid-cols-[7rem_1fr]">
            <Field
              label={t("portal.dealer_page.form_country")}
              id={`${formId}-country`}
            >
              <select
                id={`${formId}-country`}
                name="country"
                value={values.country}
                className="border-border bg-background focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 text-sm shadow-sm focus-visible:ring-2 focus-visible:outline-none"
                onChange={(e) => set("country", e.target.value)}
              >
                {COUNTRIES.map((country) => (
                  <option key={country} value={country}>
                    {country}
                  </option>
                ))}
              </select>
            </Field>
            <Field
              label={t("portal.dealer_page.form_phone")}
              id={`${formId}-phone`}
              error={errors.phone}
            >
              <Input
                id={`${formId}-phone`}
                name="phone"
                inputMode="tel"
                dir="ltr"
                value={values.phone}
                aria-invalid={Boolean(errors.phone) || undefined}
                onBlur={() => {
                  if (!values.phone.trim()) return;
                  const phone = normalizePhone(values.phone, values.country);
                  setErrors((prev) => ({
                    ...prev,
                    phone: phone
                      ? undefined
                      : t("portal.dealer_page.form_phone_invalid"),
                  }));
                  if (phone) set("phone", phone);
                }}
                onChange={(e) => set("phone", e.target.value)}
              />
            </Field>
          </div>

          <Field
            label={t("portal.dealer_page.form_email")}
            id={`${formId}-email`}
            error={errors.email}
          >
            <Input
              id={`${formId}-email`}
              name="email"
              type="email"
              value={values.email}
              aria-invalid={Boolean(errors.email) || undefined}
              onChange={(e) => set("email", e.target.value)}
            />
          </Field>

          <div className="grid gap-3 sm:grid-cols-2">
            <Field
              label={t("portal.dealer_page.form_vehicle_brand")}
              id={`${formId}-brand`}
            >
              <Input
                id={`${formId}-brand`}
                name="vehicle_brand"
                value={values.vehicle_brand}
                onChange={(e) => set("vehicle_brand", e.target.value)}
              />
            </Field>
            <Field
              label={t("portal.dealer_page.form_vehicle_model")}
              id={`${formId}-model`}
            >
              <Input
                id={`${formId}-model`}
                name="vehicle_model"
                value={values.vehicle_model}
                onChange={(e) => set("vehicle_model", e.target.value)}
              />
            </Field>
          </div>

          {services.length ? (
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium">
                {t("portal.dealer_page.form_services")}
              </legend>
              <div className="grid gap-2 sm:grid-cols-2">
                {services.map((service) => (
                  <label
                    key={service.uuid}
                    className="border-border flex gap-2 rounded-md border p-2 text-sm"
                  >
                    <input
                      type="checkbox"
                      name="interested_services"
                      className="accent-primary mt-0.5 size-4 shrink-0"
                      checked={values.interested_services.includes(
                        service.uuid,
                      )}
                      onChange={(event) => {
                        set(
                          "interested_services",
                          event.target.checked
                            ? [...values.interested_services, service.uuid]
                            : values.interested_services.filter(
                                (id) => id !== service.uuid,
                              ),
                        );
                      }}
                    />
                    <span>{service.title}</span>
                  </label>
                ))}
              </div>
            </fieldset>
          ) : null}

          <Field
            label={t("portal.dealer_page.form_message")}
            id={`${formId}-message`}
          >
            <Textarea
              id={`${formId}-message`}
              name="message"
              rows={4}
              value={values.message}
              onChange={(e) => set("message", e.target.value)}
            />
          </Field>

          <Field
            label={t("portal.dealer_page.form_channel")}
            id={`${formId}-channel`}
          >
            <select
              id={`${formId}-channel`}
              name="preferred_channel"
              value={values.preferred_channel}
              className="border-border bg-background focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 text-sm shadow-sm focus-visible:ring-2 focus-visible:outline-none"
              onChange={(e) =>
                set(
                  "preferred_channel",
                  e.target.value as Values["preferred_channel"],
                )
              }
            >
              <option value="whatsapp">
                {t("portal.dealer_page.form_channel_whatsapp")}
              </option>
              <option value="phone">
                {t("portal.dealer_page.form_channel_phone")}
              </option>
              <option value="email">
                {t("portal.dealer_page.form_channel_email")}
              </option>
            </select>
          </Field>

          <div aria-hidden="true" className="hidden">
            <label htmlFor={`${formId}-website`}>
              {t("portal.dealer_page.form_honeypot")}
            </label>
            <input
              id={`${formId}-website`}
              name="website"
              tabIndex={-1}
              autoComplete="off"
              value={values.website}
              onChange={(e) => set("website", e.target.value)}
            />
          </div>

          <div className="space-y-1">
            <div className="flex items-start gap-2">
              <input
                id={`${formId}-kvkk`}
                type="checkbox"
                name="kvkk_consent"
                className="accent-primary mt-0.5 size-4 shrink-0"
                checked={values.kvkk_consent}
                aria-invalid={Boolean(errors.kvkk_consent) || undefined}
                onChange={(event) => set("kvkk_consent", event.target.checked)}
              />
              <label
                htmlFor={`${formId}-kvkk`}
                className="text-sm leading-snug"
              >
                {t("portal.dealer_page.form_kvkk")}
              </label>
            </div>
            {errors.kvkk_consent ? (
              <p className="text-destructive text-xs">{errors.kvkk_consent}</p>
            ) : null}
            <p className="text-muted-foreground text-xs">{config.kvkk_text}</p>
          </div>

          {notice ? <p className="text-destructive text-sm">{notice}</p> : null}

          <Button type="submit" disabled={!values.kvkk_consent || submitting}>
            <Send className="size-4" aria-hidden />
            {submitting
              ? t("portal.dealer_page.form_submitting")
              : t("portal.dealer_page.form_submit")}
          </Button>
        </form>
      )}
    </section>
  );
}

function Field({
  id,
  label,
  error,
  children,
}: {
  id: string;
  label: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-1">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      {children}
      {error ? <p className="text-destructive text-xs">{error}</p> : null}
    </div>
  );
}
