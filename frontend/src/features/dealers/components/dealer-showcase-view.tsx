import {
  CircleAlert,
  Clock3,
  ExternalLink,
  Globe2,
  Images,
  MapPin,
  MessageCircle,
  SearchX,
  Star,
  Store,
  Wrench,
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
  dealerPhotoSrc,
  dealerLocality,
  dealerLogoSrc,
  dealerMapHref,
  dealerPosition,
  googleProfileHref,
  type PublicDealer,
  type PublicDealerResult,
  type PublicDealerShowcase,
} from "@/features/dealers/lib/dealer-showcase";
import { whatsappHref } from "@/features/dealers/lib/dealers";
import { translate } from "@/lib/i18n/messages";
import { cn } from "@/lib/utils";

import { ShowcaseBookingSlot } from "./showcase-booking-slot";
import { ShowcaseLeadForm } from "./showcase-lead-form";

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
      body = <DealerCard dealer={result.dealer} locale={locale} t={t} />;
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

function DealerCard({
  dealer,
  locale,
  t,
}: {
  dealer: PublicDealer;
  locale: AppLocale;
  t: T;
}) {
  const position = dealerPosition(dealer);
  const locality = dealerLocality(dealer);
  const logo = dealerLogoSrc(dealer);
  const showcase = dealer.showcase;
  const wa =
    showcase?.whatsapp_chat_url ??
    whatsappHref(
      dealer.whatsapp,
      t("portal.dealer_page.whatsapp_text", { name: dealer.name }),
    );
  return (
    <section
      data-screen="dealer"
      className="bg-card flex flex-col gap-5 rounded-lg border p-5 shadow-sm"
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
          {showcase ? <Rating showcase={showcase} t={t} /> : null}
        </div>
      </header>

      {showcase?.headline || showcase?.about ? (
        <section className="space-y-2" data-slot="showcase-intro">
          {showcase.headline ? (
            <p className="text-lg font-semibold">{showcase.headline}</p>
          ) : null}
          {showcase.about ? (
            <p className="text-muted-foreground text-sm whitespace-pre-line">
              {showcase.about}
            </p>
          ) : null}
        </section>
      ) : null}

      {showcase ? <Hours showcase={showcase} t={t} /> : null}

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

      {showcase?.services?.length ? (
        <section className="space-y-3" data-slot="showcase-services">
          <h2 className="flex items-center gap-2 text-base font-semibold">
            <Wrench className="size-4" aria-hidden />
            {t("portal.dealer_page.services")}
          </h2>
          <div className="grid gap-3 sm:grid-cols-2">
            {showcase.services.map((service, index) => (
              <article
                key={`${service.kind}-${service.title}-${index}`}
                className="border-border rounded-lg border p-3"
              >
                <h3 className="text-sm font-semibold">{service.title}</h3>
                {service.description ? (
                  <p className="text-muted-foreground mt-1 text-sm">
                    {service.description}
                  </p>
                ) : null}
              </article>
            ))}
          </div>
        </section>
      ) : null}

      {showcase?.photos?.length ? (
        <Gallery dealer={dealer} showcase={showcase} t={t} />
      ) : null}

      {showcase?.social_links && Object.keys(showcase.social_links).length ? (
        <SocialLinks links={showcase.social_links} t={t} />
      ) : null}

      {showcase?.lead_form_enabled ? (
        <ShowcaseLeadForm code={dealer.code} locale={locale} t={t} />
      ) : null}

      <ShowcaseBookingSlot />
    </section>
  );
}

function Rating({ showcase, t }: { showcase: PublicDealerShowcase; t: T }) {
  if (typeof showcase.google_rating !== "number") return null;
  const href = googleProfileHref(showcase);
  const label = t("portal.dealer_page.google_rating", {
    rating: showcase.google_rating.toFixed(1),
    count: showcase.google_review_count ?? 0,
  });
  const content = (
    <>
      <span className="flex text-amber-500" aria-hidden>
        {Array.from({ length: 5 }, (_, i) => (
          <Star
            key={i}
            className={cn(
              "size-3.5",
              i < Math.round(showcase.google_rating ?? 0) && "fill-current",
            )}
          />
        ))}
      </span>
      <span>{label}</span>
      {href ? <ExternalLink className="size-3" aria-hidden /> : null}
    </>
  );
  return href ? (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      data-slot="google-rating"
      className="text-muted-foreground hover:text-foreground flex flex-wrap items-center gap-1 text-xs"
    >
      {content}
    </a>
  ) : (
    <span
      data-slot="google-rating"
      className="text-muted-foreground flex flex-wrap items-center gap-1 text-xs"
    >
      {content}
    </span>
  );
}

function Hours({ showcase, t }: { showcase: PublicDealerShowcase; t: T }) {
  const dayLabels: Record<
    PublicDealerShowcase["working_hours"][number]["day"],
    string
  > = {
    monday: t("portal.dealer_page.day_monday"),
    tuesday: t("portal.dealer_page.day_tuesday"),
    wednesday: t("portal.dealer_page.day_wednesday"),
    thursday: t("portal.dealer_page.day_thursday"),
    friday: t("portal.dealer_page.day_friday"),
    saturday: t("portal.dealer_page.day_saturday"),
    sunday: t("portal.dealer_page.day_sunday"),
  };
  return (
    <section data-slot="showcase-hours" className="space-y-2 text-sm">
      <h2 className="flex items-center gap-2 text-base font-semibold">
        <Clock3 className="size-4" aria-hidden />
        {t("portal.dealer_page.hours")}
      </h2>
      {showcase.open_now !== null ? (
        <p
          data-slot="showcase-open-now"
          className={cn(
            "w-fit rounded-full px-2 py-1 text-xs font-medium",
            showcase.open_now
              ? "bg-emerald-100 text-emerald-800"
              : "bg-muted text-muted-foreground",
          )}
        >
          {showcase.open_now
            ? t("portal.dealer_page.open_now")
            : t("portal.dealer_page.closed_now")}
        </p>
      ) : null}
      <dl className="grid gap-1">
        {showcase.working_hours.map((day) => (
          <div key={day.day} className="grid grid-cols-[7rem_1fr] gap-3">
            <dt className="text-muted-foreground">{dayLabels[day.day]}</dt>
            <dd>
              {day.windows.length
                ? day.windows.map((w) => `${w.start}-${w.end}`).join(", ")
                : t("portal.dealer_page.closed")}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

function Gallery({
  dealer,
  showcase,
  t,
}: {
  dealer: PublicDealer;
  showcase: PublicDealerShowcase;
  t: T;
}) {
  return (
    <section data-slot="showcase-gallery" className="space-y-3">
      <h2 className="flex items-center gap-2 text-base font-semibold">
        <Images className="size-4" aria-hidden />
        {t("portal.dealer_page.gallery")}
      </h2>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        {showcase.photos.map((photo, index) => {
          const src = dealerPhotoSrc(photo.url);
          const id = `photo-${index + 1}`;
          const alt =
            photo.caption ||
            t("portal.dealer_page.gallery_alt", { name: dealer.name });
          return (
            <a key={photo.url} href={`#${id}`} className="group">
              {/* eslint-disable-next-line @next/next/no-img-element -- public showcase image through BFF */}
              <img
                src={src}
                alt={alt}
                className="aspect-square w-full rounded-md object-cover"
                loading="lazy"
              />
              <span
                id={id}
                className="pointer-events-none fixed inset-0 z-50 hidden bg-black/80 p-4 target:flex target:items-center target:justify-center"
              >
                {/* eslint-disable-next-line @next/next/no-img-element -- lightbox target */}
                <img
                  src={src}
                  alt={alt}
                  className="max-h-[90dvh] max-w-[90vw] rounded-md object-contain"
                />
              </span>
            </a>
          );
        })}
      </div>
    </section>
  );
}

function SocialLinks({ links, t }: { links: Record<string, string>; t: T }) {
  return (
    <section data-slot="showcase-social" className="space-y-2">
      <h2 className="flex items-center gap-2 text-base font-semibold">
        <Globe2 className="size-4" aria-hidden />
        {t("portal.dealer_page.social")}
      </h2>
      <div className="flex flex-wrap gap-2">
        {Object.entries(links).map(([name, href]) => (
          <a
            key={name}
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            className="border-border hover:bg-muted rounded-md border px-3 py-1 text-sm capitalize"
          >
            {name}
          </a>
        ))}
      </div>
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
