import {
  CircleAlert,
  Clock3,
  MapPin,
  MessageCircle,
  SearchX,
  Store,
} from "lucide-react";
import type { ReactNode } from "react";

import { LeafletMap } from "@/components/common/leaflet-map";
import { buttonVariants } from "@/components/ui/button";
import {
  LOCALE_NAMES,
  SUPPORTED_LOCALES,
  localeDir,
  type AppLocale,
} from "@/config/i18n";
import { mapConfig } from "@/config/map";
import {
  dealerLocality,
  dealerLogoSrc,
  dealerMapHref,
  dealerPosition,
  type PublicDealer,
  type PublicDealerResult,
} from "@/features/dealers/lib/dealer-showcase";
import { whatsappHref } from "@/features/dealers/lib/dealers";
import { translate } from "@/lib/i18n/messages";
import { cn } from "@/lib/utils";

/** Dealer finder (TEC-242): "find another dealer" goes here. */
export const DEALER_FINDER_PATH = "/portal/dealers";

export type DealerShowcaseViewProps = {
  result: PublicDealerResult;
  locale: AppLocale;
  /** Path of this page without query, for the language links. */
  path: string;
};

type T = (key: string, params?: Record<string, string | number>) => string;

/**
 * Public dealer showcase body (TEC-250, `/bayi/{code}`). Renders on the
 * server; only the map (TEC-242 LeafletMap) hydrates on the client, and
 * only when the dealer has coordinates. Mobile first, logical properties
 * only (RTL for ar). Four screens: the dealer, not found, rate limited
 * (429) and error. The quote form and Google rating come in F5 (TEC-126).
 */
export function DealerShowcaseView({
  result,
  locale,
  path,
}: DealerShowcaseViewProps) {
  const t: T = (key, params) => translate(locale, key, params);

  let body: ReactNode;
  switch (result.kind) {
    case "ok":
      body = <DealerCard dealer={result.dealer} t={t} />;
      break;
    case "not_found":
      body = (
        <Notice
          screen="not-found"
          icon={<SearchX className="size-8" aria-hidden />}
          title={t("portal.dealer_page.not_found_title")}
          text={t("portal.dealer_page.not_found_body")}
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
          title={t("portal.dealer_page.error_title")}
          text={t("portal.dealer_page.error_body")}
        />
      );
  }

  return (
    <main
      lang={locale}
      dir={localeDir(locale)}
      data-slot="dealer-showcase"
      className="bg-muted/30 text-foreground min-h-dvh px-4 py-6 sm:py-10"
    >
      <div className="mx-auto flex w-full max-w-xl flex-col gap-4">
        {body}
        <a
          href={DEALER_FINDER_PATH}
          data-slot="dealer-finder-link"
          className="text-muted-foreground hover:text-foreground self-center text-sm underline-offset-2 hover:underline"
        >
          {t("portal.dealer_page.find_other")}
        </a>
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

function DealerCard({ dealer, t }: { dealer: PublicDealer; t: T }) {
  const position = dealerPosition(dealer);
  const locality = dealerLocality(dealer);
  const logo = dealerLogoSrc(dealer);
  const wa = whatsappHref(
    dealer.whatsapp,
    t("portal.dealer_page.whatsapp_text", { name: dealer.name }),
  );
  return (
    <section
      data-screen="dealer"
      className="bg-card flex flex-col gap-5 rounded-2xl border p-5 shadow-sm"
    >
      <header className="flex items-center gap-4">
        {logo ? (
          // eslint-disable-next-line @next/next/no-img-element -- public logo URL through the BFF
          <img
            src={logo}
            alt={t("portal.dealer_page.logo_alt", { name: dealer.name })}
            data-slot="dealer-logo"
            loading="lazy"
            className="bg-background size-16 shrink-0 rounded-xl border object-contain p-1"
          />
        ) : (
          <span
            aria-hidden
            className="bg-muted text-muted-foreground flex size-16 shrink-0 items-center justify-center rounded-xl"
          >
            <Store className="size-7" />
          </span>
        )}
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            {t("portal.dealer_page.eyebrow")}
          </span>
          <h1 className="text-xl leading-tight font-semibold break-words">
            {dealer.name}
          </h1>
          {locality ? (
            <span className="text-muted-foreground text-sm">{locality}</span>
          ) : null}
        </div>
      </header>

      {dealer.address ? (
        <div className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            {t("portal.dealer_page.address")}
          </span>
          <address
            data-slot="dealer-address"
            className="flex items-start gap-2 break-words not-italic"
          >
            <MapPin className="mt-0.5 size-4 shrink-0" aria-hidden />
            <span>{dealer.address}</span>
          </address>
        </div>
      ) : null}

      {position ? (
        <div className="flex flex-col gap-2">
          <LeafletMap
            center={position}
            zoom={mapConfig.pointZoom + 3}
            markers={[
              {
                id: dealer.code,
                lat: position.lat,
                lng: position.lng,
                label: dealer.name,
                kind: "place",
                active: true,
              },
            ]}
            ariaLabel={t("portal.dealer_page.map_label")}
            testId="dealer-showcase-map"
          />
          <a
            href={dealerMapHref(position)}
            target="_blank"
            rel="noopener noreferrer"
            data-slot="dealer-map-link"
            className="text-muted-foreground hover:text-foreground self-end text-xs underline-offset-2 hover:underline"
          >
            {t("portal.dealer_page.open_map")}
          </a>
        </div>
      ) : (
        <p
          data-slot="dealer-no-location"
          className="bg-muted/60 text-muted-foreground rounded-xl px-4 py-3 text-sm"
        >
          {t("portal.dealer_page.no_location")}
        </p>
      )}

      {wa ? (
        <a
          href={wa}
          target="_blank"
          rel="noopener noreferrer"
          data-slot="dealer-whatsapp"
          className={cn(
            buttonVariants({ size: "lg" }),
            "w-full bg-emerald-600 text-white hover:bg-emerald-700",
          )}
        >
          <MessageCircle className="size-5" aria-hidden />
          {t("portal.dealer_page.whatsapp")}
        </a>
      ) : null}
    </section>
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
      className="bg-card flex flex-col items-center gap-3 rounded-2xl border p-6 text-center shadow-sm"
    >
      <span className="text-muted-foreground">{icon}</span>
      <h1 className="text-lg font-semibold">{title}</h1>
      <p className="text-muted-foreground text-sm">{text}</p>
      {extra ? <p className="text-muted-foreground text-xs">{extra}</p> : null}
    </section>
  );
}
