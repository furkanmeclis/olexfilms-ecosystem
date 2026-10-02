import { CircleAlert, Clock3, SearchX, ShieldCheck } from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import {
  LOCALE_NAMES,
  SUPPORTED_LOCALES,
  localeDir,
  type AppLocale,
} from "@/config/i18n";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import {
  statusTone,
  type PublicWarranty,
  type PublicWarrantyResult,
} from "@/features/warranty/lib/public-warranty";
import { formatDate, formatNumber } from "@/lib/i18n/format";
import { translate } from "@/lib/i18n/messages";

export type PublicWarrantyViewProps = {
  result: PublicWarrantyResult;
  locale: AppLocale;
  timeZone: string;
  /** Path of this page without query, for the language links. */
  path: string;
};

/**
 * Public warranty page body (TEC-189). Renders on the server: no session,
 * no client fetch. Mobile first, logical properties only (RTL for ar).
 * Four screens: the warranty, not found, rate limited (429) and error.
 */
export function PublicWarrantyView({
  result,
  locale,
  timeZone,
  path,
}: PublicWarrantyViewProps) {
  const t = (key: string, params?: Record<string, string | number>) =>
    translate(locale, key, params);

  let body: ReactNode;
  switch (result.kind) {
    case "ok":
      body = (
        <WarrantyCard
          warranty={result.warranty}
          locale={locale}
          timeZone={timeZone}
          t={t}
        />
      );
      break;
    case "not_found":
      body = (
        <Notice
          screen="not-found"
          icon={<SearchX className="size-8" aria-hidden />}
          title={t("warranty.public.not_found_title")}
          text={t("warranty.public.not_found_body")}
        />
      );
      break;
    case "rate_limited":
      body = (
        <Notice
          screen="rate-limited"
          icon={<Clock3 className="size-8" aria-hidden />}
          title={t("warranty.public.rate_limited_title")}
          text={t("warranty.public.rate_limited_body")}
          extra={
            result.retryAfter
              ? t("warranty.public.retry_after", { seconds: result.retryAfter })
              : undefined
          }
        />
      );
      break;
    default:
      body = (
        <Notice
          screen="error"
          icon={<CircleAlert className="size-8" aria-hidden />}
          title={t("warranty.public.error_title")}
          text={t("warranty.public.error_body")}
        />
      );
  }

  return (
    <main
      lang={locale}
      dir={localeDir(locale)}
      data-slot="public-warranty"
      className="bg-muted/30 text-foreground min-h-dvh px-4 py-6 sm:py-10"
    >
      <div className="mx-auto flex w-full max-w-md flex-col gap-4">
        <p className="text-muted-foreground text-center text-sm font-medium">
          {t("warranty.public.page_title")}
        </p>
        {body}
        <nav
          aria-label={t("warranty.public.language")}
          className="text-muted-foreground flex flex-wrap justify-center gap-x-3 gap-y-1 pt-2 text-xs"
        >
          {SUPPORTED_LOCALES.map((l) => (
            <a
              key={l}
              href={`${path}?lang=${l}`}
              lang={l}
              hrefLang={l}
              aria-current={l === locale ? "true" : undefined}
              className={
                l === locale
                  ? "text-foreground font-semibold"
                  : "hover:text-foreground underline-offset-2 hover:underline"
              }
            >
              {LOCALE_NAMES[l]}
            </a>
          ))}
        </nav>
      </div>
    </main>
  );
}

const STATUS_KEYS: Record<PublicWarranty["status"], string> = {
  active: "warranty.public.status.active",
  expired: "warranty.public.status.expired",
  void: "warranty.public.status.void",
};

type T = (key: string, params?: Record<string, string | number>) => string;

function WarrantyCard({
  warranty,
  locale,
  timeZone,
  t,
}: {
  warranty: PublicWarranty;
  locale: AppLocale;
  timeZone: string;
  t: T;
}) {
  const ctx = { locale, timeZone };
  const v = warranty.vehicle;
  const vehicleName = [v.brand_name, v.model_name].filter(Boolean).join(" ");
  return (
    <section
      data-screen="warranty"
      className="bg-card flex flex-col gap-5 rounded-2xl border p-5 shadow-sm"
    >
      <header className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-muted-foreground flex items-center gap-1.5 text-xs font-medium tracking-wide uppercase">
            <ShieldCheck className="size-4" aria-hidden />
            {warranty.brand.name}
          </span>
          <h1 className="text-xl leading-tight font-semibold break-words">
            {warranty.product.name}
          </h1>
        </div>
        <Badge
          variant={statusTone(warranty.status)}
          data-status={warranty.status}
          className="shrink-0 px-2.5 py-1 text-sm"
        >
          {t(STATUS_KEYS[warranty.status])}
        </Badge>
      </header>

      {warranty.status === "active" ? (
        <div className="bg-muted/60 flex items-baseline justify-between gap-3 rounded-xl px-4 py-3">
          <span className="text-muted-foreground text-sm">
            {t("warranty.public.days_remaining")}
          </span>
          <span
            data-slot="days-remaining"
            className="text-2xl font-semibold tabular-nums"
          >
            {formatNumber(warranty.days_remaining, ctx)}
          </span>
        </div>
      ) : null}

      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
        <Field label={t("warranty.public.start")}>
          {formatDate(warranty.start_at, ctx, "medium")}
        </Field>
        <Field label={t("warranty.public.end")}>
          {formatDate(warranty.end_at, ctx, "medium")}
        </Field>
      </dl>

      <div className="flex flex-col gap-3 border-t pt-4">
        <h2 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
          {t("warranty.public.vehicle")}
        </h2>
        <div className="flex items-center gap-3">
          <VehicleBrandLogo
            uuid={v.brand_logo_uuid}
            name={v.brand_name}
            height={36}
          />
          <span className="font-medium break-words">{vehicleName}</span>
        </div>
        <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
          {v.plate_masked ? (
            <Field label={t("warranty.public.plate")}>
              <span dir="ltr" className="font-mono tracking-wider">
                {v.plate_masked}
              </span>
            </Field>
          ) : null}
          {v.vin_last4 ? (
            <Field label={t("warranty.public.vin")}>
              <span dir="ltr" className="font-mono tracking-wider">
                {v.vin_last4}
              </span>
            </Field>
          ) : null}
          {v.model_year ? (
            <Field label={t("warranty.public.model_year")}>
              {String(v.model_year)}
            </Field>
          ) : null}
        </dl>
      </div>

      <div className="flex flex-col gap-1 border-t pt-4 text-sm">
        <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
          {t("warranty.public.dealer")}
        </span>
        <span className="font-medium break-words">
          {warranty.dealer.name}
          {warranty.dealer.city ? (
            <span className="text-muted-foreground font-normal">
              {" · "}
              {warranty.dealer.city}
            </span>
          ) : null}
        </span>
      </div>

      <p className="text-muted-foreground text-xs">
        {t("warranty.public.privacy_note")}
      </p>
    </section>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="font-medium break-words">{children}</dd>
    </div>
  );
}

function Notice({
  screen,
  icon,
  title,
  text,
  extra,
}: {
  screen: string;
  icon: ReactNode;
  title: string;
  text: string;
  extra?: string;
}) {
  return (
    <section
      data-screen={screen}
      role={screen === "error" ? "alert" : undefined}
      className="bg-card flex flex-col items-center gap-3 rounded-2xl border p-6 text-center shadow-sm"
    >
      <span className="bg-muted text-muted-foreground flex size-14 items-center justify-center rounded-full">
        {icon}
      </span>
      <h1 className="text-lg font-semibold">{title}</h1>
      <p className="text-muted-foreground text-sm">{text}</p>
      {extra ? <p className="text-sm font-medium">{extra}</p> : null}
    </section>
  );
}
