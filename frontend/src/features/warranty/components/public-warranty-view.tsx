import {
  Building2,
  CarFront,
  CircleAlert,
  Clock3,
  FileDown,
  SearchX,
  ShieldCheck,
} from "lucide-react";
import type { ReactNode } from "react";

import { AppLogo } from "@/components/brand/ides-logo";
import { Badge } from "@/components/ui/badge";
import { buttonVariants } from "@/components/ui/button";
import {
  LOCALE_NAMES,
  SUPPORTED_LOCALES,
  localeDir,
  type AppLocale,
} from "@/config/i18n";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import {
  hasActiveWarranty,
  publicWarrantyPdfHref,
  sortWarranties,
  statusTone,
  type PdfNotice,
  type PublicService,
  type PublicServiceWarranty,
  type PublicWarrantyResult,
} from "@/features/warranty/lib/public-warranty";
import {
  progressTone,
  warrantyProgress,
} from "@/features/warranty/lib/warranty-list";
import { formatDate, formatNumber } from "@/lib/i18n/format";
import { translate } from "@/lib/i18n/messages";
import { cn } from "@/lib/utils";

export type PublicWarrantyViewProps = {
  result: PublicWarrantyResult;
  locale: AppLocale;
  timeZone: string;
  /** Path of this page without query, for the language links. */
  path: string;
  /** A failed PDF download sent back here (`?pdf=`, TEC-248). */
  pdfNotice?: PdfNotice | null;
  /** Clock of the progress bars (tests). */
  now?: Date;
};

type T = (key: string, params?: Record<string, string | number>) => string;

/**
 * Public warranty page body (TEC-189; one page per service, TEC-1005). The
 * legacy hub's layout (vehicle, applied products with their warranties,
 * dealer, certificate) without the phone mock frame, in the brand tokens.
 * Renders on the server: no session, no client fetch. Mobile first,
 * logical properties only (RTL for ar). Four screens: the service, not
 * found, rate limited (429) and error.
 */
export function PublicWarrantyView({
  result,
  locale,
  timeZone,
  path,
  pdfNotice = null,
  now,
}: PublicWarrantyViewProps) {
  const t: T = (key, params) => translate(locale, key, params);

  let body: ReactNode;
  switch (result.kind) {
    case "ok":
      body = (
        <ServiceBody
          service={result.service}
          locale={locale}
          timeZone={timeZone}
          t={t}
          pdfNotice={pdfNotice}
          now={now}
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

  const brandName =
    result.kind === "ok" ? result.service.brand.name : undefined;
  return (
    <main
      lang={locale}
      dir={localeDir(locale)}
      data-slot="public-warranty"
      className="bg-muted/30 text-foreground min-h-dvh"
    >
      <header className="bg-primary text-primary-foreground [--brand-glyph:var(--primary-foreground)]">
        <div className="mx-auto flex w-full max-w-2xl flex-col items-center gap-3 px-4 pt-8 pb-14 text-center">
          <AppLogo className="h-10 w-auto" />
          <p className="text-sm font-medium tracking-wide opacity-90">
            {t("warranty.public.page_title")}
          </p>
        </div>
      </header>
      <div className="mx-auto -mt-10 flex w-full max-w-2xl flex-col gap-4 px-4 pb-10">
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
        {brandName ? (
          <p className="text-muted-foreground text-center text-xs">
            © {brandName}
          </p>
        ) : null}
      </div>
    </main>
  );
}

const STATUS_KEYS: Record<PublicServiceWarranty["status"], string> = {
  active: "warranty.public.status.active",
  expired: "warranty.public.status.expired",
  void: "warranty.public.status.void",
};

const PROGRESS_TONE_CLASS = {
  success: "bg-emerald-500",
  warning: "bg-amber-500",
  muted: "bg-muted-foreground/40",
} as const;

/**
 * Elapsed share of start → end. Server rendered (no locale provider), so it
 * is a plain bar; it grows along the inline axis (right-to-left in RTL).
 */
function PublicProgress({
  warranty,
  now,
  t,
}: {
  warranty: PublicServiceWarranty;
  now?: Date;
  t: T;
}) {
  const p = warrantyProgress(warranty.start_at, warranty.end_at, now);
  const tone = progressTone(warranty.status, p);
  return (
    <div
      data-slot="warranty-progress"
      data-percent={p.percent}
      className="flex flex-col gap-1"
    >
      <span className="text-muted-foreground text-xs tabular-nums">
        {t("warranty.progress.elapsed", { percent: p.percent })}
      </span>
      <div
        className="bg-muted h-2 w-full overflow-hidden rounded-full"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={p.percent}
        aria-label={t("warranty.progress.aria")}
      >
        <div
          className={cn("h-full rounded-full", PROGRESS_TONE_CLASS[tone])}
          style={{ inlineSize: `${p.percent}%` }}
        />
      </div>
    </div>
  );
}

function Card({
  icon,
  title,
  children,
  slot,
}: {
  icon: ReactNode;
  title: string;
  children: ReactNode;
  slot?: string;
}) {
  return (
    <section
      data-slot={slot}
      className="bg-card flex flex-col gap-4 rounded-2xl border p-5 shadow-sm"
    >
      <h2 className="text-muted-foreground flex items-center gap-2 text-xs font-semibold tracking-wide uppercase">
        <span className="text-primary">{icon}</span>
        {title}
      </h2>
      {children}
    </section>
  );
}

function partLabel(t: T, part: string): string {
  const key = `services.parts.names.${part}`;
  const label = t(key);
  return label === key ? part : label;
}

function WarrantyItem({
  warranty,
  locale,
  timeZone,
  t,
  now,
}: {
  warranty: PublicServiceWarranty;
  locale: AppLocale;
  timeZone: string;
  t: T;
  now?: Date;
}) {
  const ctx = { locale, timeZone };
  return (
    <li
      data-slot="warranty-item"
      data-warranty-status={warranty.status}
      className="flex flex-col gap-3 rounded-xl border p-4"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-0.5">
          {warranty.category ? (
            <span className="text-muted-foreground text-xs font-medium">
              {warranty.category}
            </span>
          ) : null}
          <h3 className="leading-tight font-semibold break-words">
            {warranty.product.name}
          </h3>
        </div>
        <Badge
          variant={statusTone(warranty.status)}
          data-status={warranty.status}
          className="shrink-0"
        >
          {t(STATUS_KEYS[warranty.status])}
        </Badge>
      </div>
      {warranty.status === "active" ? (
        <div className="bg-muted/60 flex items-baseline justify-between gap-3 rounded-lg px-3 py-2">
          <span className="text-muted-foreground text-sm">
            {t("warranty.public.days_remaining")}
          </span>
          <span
            data-slot="days-remaining"
            className="text-xl font-semibold tabular-nums"
          >
            {formatNumber(warranty.days_remaining, ctx)}
          </span>
        </div>
      ) : null}
      <PublicProgress warranty={warranty} now={now} t={t} />
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        <Field label={t("warranty.public.start")}>
          {formatDate(warranty.start_at, ctx, "medium")}
        </Field>
        <Field label={t("warranty.public.end")}>
          {formatDate(warranty.end_at, ctx, "medium")}
        </Field>
      </dl>
      {warranty.parts.length > 0 ? (
        <div className="flex flex-col gap-1.5">
          <span className="text-muted-foreground text-xs">
            {t("warranty.public.applied_parts")}
          </span>
          <ul className="flex flex-wrap gap-1.5">
            {warranty.parts.map((part) => (
              <li
                key={part}
                className="bg-muted rounded-full px-2.5 py-0.5 text-xs"
              >
                {partLabel(t, part)}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </li>
  );
}

function ServiceBody({
  service,
  locale,
  timeZone,
  t,
  pdfNotice,
  now,
}: {
  service: PublicService;
  locale: AppLocale;
  timeZone: string;
  t: T;
  pdfNotice: PdfNotice | null;
  now?: Date;
}) {
  const ctx = { locale, timeZone };
  const v = service.vehicle;
  const vehicleName = [v.brand_name, v.model_name].filter(Boolean).join(" ");
  const warranties = sortWarranties(service.warranties);
  return (
    <div data-screen="warranty" className="flex flex-col gap-4">
      <Card
        slot="vehicle"
        icon={<CarFront className="size-4" aria-hidden />}
        title={t("warranty.public.vehicle")}
      >
        <div className="flex items-center gap-3">
          <VehicleBrandLogo
            uuid={v.brand_logo_uuid}
            name={v.brand_name}
            height={40}
          />
          <div className="flex min-w-0 flex-col">
            <span className="text-lg font-semibold break-words">
              {vehicleName}
            </span>
            {v.model_year ? (
              <span className="text-muted-foreground text-sm">
                {String(v.model_year)}
              </span>
            ) : null}
          </div>
        </div>
        <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
          <Field label={t("warranty.public.service_no")}>
            <span dir="ltr" className="font-mono" data-slot="service-no">
              {service.service_no}
            </span>
          </Field>
          <Field label={t("warranty.public.service_date")}>
            {formatDate(service.service_date, ctx, "medium")}
          </Field>
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
          {service.package ? (
            <Field label={t("warranty.public.package")}>
              {service.package}
            </Field>
          ) : null}
        </dl>
      </Card>

      <Card
        slot="warranties"
        icon={<ShieldCheck className="size-4" aria-hidden />}
        title={t("warranty.public.warranties")}
      >
        <ul className="flex flex-col gap-3">
          {warranties.map((w) => (
            <WarrantyItem
              key={w.public_code}
              warranty={w}
              locale={locale}
              timeZone={timeZone}
              t={t}
              now={now}
            />
          ))}
        </ul>
      </Card>

      <Card
        slot="dealer"
        icon={<Building2 className="size-4" aria-hidden />}
        title={t("warranty.public.dealer")}
      >
        <p className="font-medium break-words">
          {service.dealer.name}
          {service.dealer.city ? (
            <span className="text-muted-foreground font-normal">
              {" · "}
              {service.dealer.city}
            </span>
          ) : null}
        </p>
      </Card>

      {hasActiveWarranty(service) ? (
        <div className="flex flex-col gap-2">
          <a
            data-slot="pdf-download"
            href={publicWarrantyPdfHref(service.service_no, locale, timeZone)}
            rel="nofollow noreferrer"
            className={cn(buttonVariants({ size: "lg" }), "w-full")}
          >
            <FileDown className="size-4" aria-hidden />
            {t("warranty.public.download_pdf")}
          </a>
          {pdfNotice ? (
            <p
              data-slot="pdf-notice"
              role="alert"
              className="text-destructive text-center text-xs"
            >
              {t(
                pdfNotice === "rate_limited"
                  ? "warranty.public.rate_limited_body"
                  : "warranty.public.error_body",
              )}
            </p>
          ) : null}
        </div>
      ) : null}

      <p className="text-muted-foreground text-center text-xs">
        {t("warranty.public.privacy_note")}
      </p>
    </div>
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
