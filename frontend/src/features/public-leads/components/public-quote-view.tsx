import {
  CalendarClock,
  CircleAlert,
  Clock3,
  FileDown,
  FileText,
  SearchX,
} from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { buttonVariants } from "@/components/ui/button";
import type { AppLocale } from "@/config/i18n";
import { formatCurrency, formatDate, formatNumber } from "@/lib/i18n/format";
import { translate } from "@/lib/i18n/messages";
import { cn } from "@/lib/utils";

import {
  amount,
  isQuoteExpired,
  publicQuotePdfHref,
  type PublicQuote,
  type PublicQuoteLine,
  type PublicQuoteResult,
  type QuotePdfNotice,
} from "../lib/public-quote";
import { PublicNotice, PublicPageShell } from "./public-page-shell";

export type PublicQuoteViewProps = {
  result: PublicQuoteResult;
  locale: AppLocale;
  timeZone: string;
  /** Path of this page without query, for the language links. */
  path: string;
  /** Token of the URL (PDF link). */
  token: string;
  pdfNotice?: QuotePdfNotice | null;
  /** Clock of the validity check (tests). */
  now?: Date;
};

type T = (key: string, params?: Record<string, string | number>) => string;

const PDF_NOTICE_KEYS: Record<QuotePdfNotice, string> = {
  pending: "landing.quote.pdf_pending",
  rate_limited: "landing.quote.pdf_rate_limited",
  unavailable: "landing.quote.pdf_unavailable",
};

/**
 * Public quote page body (TEC-320): read-only quote of `/teklif/{token}`
 * with the issuing organization, lines, totals, validity and the PDF
 * download. Server rendered, no session; it never shows the recipient's
 * phone or e-mail (the API does not return them).
 */
export function PublicQuoteView({
  result,
  locale,
  timeZone,
  path,
  token,
  pdfNotice = null,
  now,
}: PublicQuoteViewProps) {
  const t: T = (key, params) => translate(locale, key, params);

  let body: ReactNode;
  switch (result.kind) {
    case "ok":
      body = (
        <QuoteCard
          quote={result.quote}
          locale={locale}
          timeZone={timeZone}
          token={token}
          pdfNotice={pdfNotice}
          now={now}
          t={t}
        />
      );
      break;
    case "not_found":
      body = <PublicQuoteNotFoundNotice t={t} />;
      break;
    case "rate_limited":
      body = (
        <PublicNotice
          screen="rate-limited"
          icon={<Clock3 className="size-8" aria-hidden />}
          title={t("landing.quote.rate_limited_title")}
          text={t("landing.quote.rate_limited_body")}
          extra={
            result.retryAfter
              ? t("landing.quote.retry_after", { seconds: result.retryAfter })
              : undefined
          }
        />
      );
      break;
    default:
      body = (
        <PublicNotice
          screen="error"
          icon={<CircleAlert className="size-8" aria-hidden />}
          title={t("landing.quote.error_title")}
          text={t("landing.quote.error_body")}
        />
      );
  }

  return (
    <PublicPageShell locale={locale} path={path} slot="public-quote">
      {body}
    </PublicPageShell>
  );
}

export function PublicQuoteNotFoundNotice({ t }: { t: T }) {
  return (
    <PublicNotice
      screen="not-found"
      icon={<SearchX className="size-8" aria-hidden />}
      title={t("landing.quote.not_found_title")}
      text={t("landing.quote.not_found_body")}
    />
  );
}

function QuoteCard({
  quote,
  locale,
  timeZone,
  token,
  pdfNotice,
  now,
  t,
}: {
  quote: PublicQuote;
  locale: AppLocale;
  timeZone: string;
  token: string;
  pdfNotice: QuotePdfNotice | null;
  now?: Date;
  t: T;
}) {
  const ctx = { locale, timeZone };
  const money = (value: string) =>
    formatCurrency(amount(value), quote.currency, ctx);
  const expired = isQuoteExpired(quote, now);
  const lines = [...quote.lines].sort((a, b) => a.sort_order - b.sort_order);
  const hasDiscount = (amount(quote.discount_total) ?? 0) !== 0;
  const hasTax = (amount(quote.tax_total) ?? 0) !== 0;

  return (
    <section
      data-screen="quote"
      className="bg-card flex flex-col gap-5 rounded-2xl border p-5 shadow-sm sm:p-6"
    >
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-muted-foreground flex items-center gap-1.5 text-xs font-medium tracking-wide uppercase">
            <FileText className="size-4" aria-hidden />
            {t("landing.quote.eyebrow")}
          </span>
          <h1
            data-slot="organization-name"
            className="text-xl leading-tight font-semibold break-words"
          >
            {quote.organization_name}
          </h1>
          <span className="text-muted-foreground text-sm">
            {t("landing.quote.number")}:{" "}
            <span dir="ltr" className="font-mono">
              {quote.display_no}
            </span>
          </span>
        </div>
        {expired ? (
          <Badge
            variant="warning"
            data-slot="quote-expired"
            className="shrink-0"
          >
            {t("landing.quote.expired")}
          </Badge>
        ) : null}
      </header>

      <div
        data-slot="valid-until"
        className="bg-muted/60 flex items-center gap-2 rounded-xl px-4 py-3 text-sm"
      >
        <CalendarClock
          className="text-muted-foreground size-4 shrink-0"
          aria-hidden
        />
        <span className="text-muted-foreground">
          {t("landing.quote.valid_until")}:
        </span>
        <span className="font-medium">
          {quote.valid_until
            ? formatDate(quote.valid_until, ctx, "long")
            : t("landing.quote.no_expiry")}
        </span>
      </div>

      <div className="flex flex-col gap-3">
        <h2 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
          {t("landing.quote.lines_title")}
        </h2>
        {/* Document preview (AGENTS.md §6 exception): line cards, no table. */}
        <ul
          data-slot="quote-lines"
          className="flex flex-col divide-y rounded-xl border"
        >
          {lines.map((line, i) => (
            <QuoteLineItem
              key={`${line.sort_order}-${i}`}
              line={line}
              money={money}
              locale={locale}
              timeZone={timeZone}
              t={t}
            />
          ))}
        </ul>
      </div>

      <dl data-slot="quote-totals" className="flex flex-col gap-2 text-sm">
        <TotalRow label={t("landing.quote.subtotal")}>
          {money(quote.subtotal)}
        </TotalRow>
        {hasDiscount ? (
          <TotalRow label={t("landing.quote.discount_total")}>
            −{money(quote.discount_total)}
          </TotalRow>
        ) : null}
        {hasTax ? (
          <TotalRow label={t("landing.quote.tax_total")}>
            {money(quote.tax_total)}
          </TotalRow>
        ) : null}
        <div className="flex items-baseline justify-between gap-3 border-t pt-3">
          <dt className="font-semibold">{t("landing.quote.grand_total")}</dt>
          <dd
            data-slot="grand-total"
            className="text-xl font-semibold tabular-nums"
          >
            {money(quote.grand_total)}
          </dd>
        </div>
      </dl>

      <div className="flex flex-col gap-2 border-t pt-4">
        <a
          data-slot="pdf-download"
          href={publicQuotePdfHref(token, locale)}
          rel="nofollow noreferrer"
          className={cn(buttonVariants({ variant: "outline" }), "w-full")}
        >
          <FileDown className="size-4" aria-hidden />
          {t("landing.quote.download_pdf")}
        </a>
        {pdfNotice ? (
          <p
            data-slot="pdf-notice"
            data-notice={pdfNotice}
            role={pdfNotice === "pending" ? "status" : "alert"}
            className={cn(
              "text-center text-xs",
              pdfNotice === "pending"
                ? "text-muted-foreground"
                : "text-destructive",
            )}
          >
            {t(PDF_NOTICE_KEYS[pdfNotice])}
          </p>
        ) : null}
      </div>

      <p className="text-muted-foreground text-xs">
        {t("landing.quote.privacy_note")}
      </p>
    </section>
  );
}

function QuoteLineItem({
  line,
  money,
  locale,
  timeZone,
  t,
}: {
  line: PublicQuoteLine;
  money: (value: string) => string;
  locale: AppLocale;
  timeZone: string;
  t: T;
}) {
  const discount = amount(line.discount_amount) ?? 0;
  return (
    <li data-slot="quote-line" className="flex flex-col gap-2 p-3 sm:p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="font-medium break-words">{line.description}</span>
          <span className="text-muted-foreground text-xs">
            {t(`landing.quote.line_type.${line.line_type}`)}
          </span>
        </div>
        <span className="shrink-0 font-semibold tabular-nums">
          {money(line.line_total)}
        </span>
      </div>
      <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs tabular-nums">
        <span>
          {t("landing.quote.quantity")}:{" "}
          {formatNumber(amount(line.quantity), { locale, timeZone })}
        </span>
        <span>
          {t("landing.quote.unit_price")}: {money(line.unit_price)}
        </span>
        {discount !== 0 ? (
          <span>
            {t("landing.quote.discount")}: −{money(line.discount_amount)}
          </span>
        ) : null}
      </div>
    </li>
  );
}

function TotalRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="tabular-nums">{children}</dd>
    </div>
  );
}
