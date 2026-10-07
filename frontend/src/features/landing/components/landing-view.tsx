import {
  Layers,
  Mail,
  MapPin,
  Phone,
  QrCode,
  ShieldCheck,
  Store,
  Sun,
} from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
  LOCALE_NAMES,
  SUPPORTED_LOCALES,
  localeDir,
  type AppLocale,
} from "@/config/i18n";
import { routes } from "@/config/routes";
import { translate } from "@/lib/i18n/messages";

import { WarrantyLookupForm } from "./warranty-lookup-form";

/** Contact details of the legacy welcome page (olexfilms Welcome.tsx). */
const CONTACT = {
  phones: [
    { label: "TR", display: "+90 (507) 465 34 34", tel: "+905074653434" },
    { label: "US", display: "+1 (410) 844 5381", tel: "+14108445381" },
  ],
  emails: ["info@olexfilms.com", "bayilik@olexfilms.com"],
  address: "Şenlikköy Mh. Florya Cd. No:14 Bakırköy / İstanbul",
} as const;

export type LandingViewProps = {
  locale: AppLocale;
  /** Set when the language came from `?lang=`; kept on the links. */
  lang?: string;
  /** TEC-320: links `/bayi-basvuru` while the application form is open. */
  dealerApplicationOpen?: boolean;
};

/**
 * Public landing page (TEC-247): the "Protection Solutions" content of the
 * legacy welcome page, a warranty lookup, the dealer finder link and the
 * panel / portal sign-in buttons. Server rendered; only the lookup form is
 * a client island. Logical properties only (RTL for ar), local images.
 */
export function LandingView({
  locale,
  lang,
  dealerApplicationOpen = false,
}: LandingViewProps) {
  const t = (key: string, params?: Record<string, string | number>) =>
    translate(locale, key, params);

  const solutions: {
    key: string;
    title: string;
    body: string;
    icon: ReactNode;
  }[] = [
    {
      key: "ppf",
      title: t("landing.solutions.ppf.title"),
      body: t("landing.solutions.ppf.body"),
      icon: <ShieldCheck className="size-6" aria-hidden />,
    },
    {
      key: "window_film",
      title: t("landing.solutions.window_film.title"),
      body: t("landing.solutions.window_film.body"),
      icon: <Sun className="size-6" aria-hidden />,
    },
    {
      key: "digital_warranty",
      title: t("landing.solutions.digital_warranty.title"),
      body: t("landing.solutions.digital_warranty.body"),
      icon: <QrCode className="size-6" aria-hidden />,
    },
  ];

  return (
    <main
      lang={locale}
      dir={localeDir(locale)}
      data-slot="landing"
      className="bg-background text-foreground flex min-h-dvh flex-col"
    >
      <div className="relative overflow-hidden bg-gradient-to-br from-[#00140c] via-[#003d25] to-[#00140c] text-white">
        <header className="mx-auto flex w-full max-w-6xl items-center justify-between gap-3 px-4 py-4 sm:px-6">
          <Link href={routes.public.root} aria-label="Olex Films">
            <Image
              src="/images/olex-logo-yatay.svg"
              alt="Olex Films"
              width={120}
              height={48}
              priority
              unoptimized
              className="h-9 w-auto sm:h-10"
            />
          </Link>
          <nav
            aria-label={t("landing.nav.label")}
            className="flex items-center gap-2"
          >
            <Button
              asChild
              variant="ghost"
              size="sm"
              className="text-white hover:bg-white/10 hover:text-white"
            >
              <Link href={routes.portal.login} data-slot="portal-login">
                {t("landing.nav.portal_login")}
              </Link>
            </Button>
            <Button
              asChild
              size="sm"
              className="bg-[#f2ac4a] text-[#00140c] hover:bg-[#f5bd6d]"
            >
              <Link href={routes.guest.login} data-slot="panel-login">
                {t("landing.nav.panel_login")}
              </Link>
            </Button>
          </nav>
        </header>

        <section className="mx-auto grid w-full max-w-6xl gap-10 px-4 pt-10 pb-16 sm:px-6 lg:grid-cols-2 lg:items-center lg:pt-16 lg:pb-24">
          <div className="flex flex-col gap-5 text-center lg:text-start">
            <span className="mx-auto inline-flex w-fit items-center gap-2 rounded-full border border-white/20 bg-white/5 px-3 py-1 text-xs font-medium tracking-wide text-[#f2ac4a] uppercase lg:mx-0">
              <Layers className="size-3.5" aria-hidden />
              {t("landing.hero.eyebrow")}
            </span>
            <h1 className="text-3xl leading-tight font-bold sm:text-5xl">
              {t("landing.hero.title_part1")}{" "}
              <span className="bg-gradient-to-r from-[#e6b800] via-[#ffd700] to-[#e6b800] bg-clip-text text-transparent">
                {t("landing.hero.title_part2")}
              </span>
            </h1>
            <p className="text-base text-emerald-100/80 sm:text-lg">
              {t("landing.hero.description")}
            </p>
          </div>

          <div
            id="garanti"
            className="bg-card text-card-foreground flex flex-col gap-4 rounded-2xl border p-5 shadow-xl sm:p-6"
          >
            <div className="flex flex-col gap-1">
              <h2 className="text-lg font-semibold">
                {t("landing.lookup.title")}
              </h2>
              <p className="text-muted-foreground text-sm">
                {t("landing.lookup.description")}
              </p>
            </div>
            <WarrantyLookupForm
              lang={lang}
              labels={{
                label: t("landing.lookup.label"),
                placeholder: t("landing.lookup.placeholder"),
                submit: t("landing.lookup.submit"),
                errorRequired: t("landing.lookup.error_required"),
              }}
            />
          </div>
        </section>
      </div>

      <section
        aria-labelledby="landing-solutions"
        className="mx-auto w-full max-w-6xl px-4 py-14 sm:px-6"
      >
        <div className="mx-auto mb-8 flex max-w-2xl flex-col gap-2 text-center">
          <h2 id="landing-solutions" className="text-2xl font-bold sm:text-3xl">
            {t("landing.solutions.title")}
          </h2>
          <p className="text-muted-foreground">
            {t("landing.solutions.description")}
          </p>
        </div>
        <ul className="grid gap-4 md:grid-cols-3">
          {solutions.map((s) => (
            <li
              key={s.key}
              data-solution={s.key}
              className="bg-card flex flex-col gap-3 rounded-2xl border p-5 shadow-sm"
            >
              <span className="flex size-11 items-center justify-center rounded-xl bg-[#003d25]/10 text-[#003d25] dark:bg-[#f2ac4a]/10 dark:text-[#f2ac4a]">
                {s.icon}
              </span>
              <h3 className="text-lg font-semibold">{s.title}</h3>
              <p className="text-muted-foreground text-sm">{s.body}</p>
            </li>
          ))}
        </ul>
      </section>

      <section className="bg-muted/40 border-y">
        <div className="mx-auto flex w-full max-w-6xl flex-col items-center gap-4 px-4 py-12 text-center sm:px-6 md:flex-row md:justify-between md:text-start">
          <div className="flex flex-col gap-1">
            <h2 className="flex items-center justify-center gap-2 text-xl font-bold md:justify-start">
              <Store className="size-5" aria-hidden />
              {t("landing.dealers.title")}
            </h2>
            <p className="text-muted-foreground">
              {t("landing.dealers.description")}
            </p>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Button asChild size="lg" variant="outline">
              <Link href={routes.portal.dealers} data-slot="find-dealer">
                {t("landing.dealers.cta")}
              </Link>
            </Button>
            {dealerApplicationOpen ? (
              <Button asChild size="lg">
                <Link
                  href={
                    lang
                      ? `${routes.public.dealerApplication}?lang=${encodeURIComponent(lang)}`
                      : routes.public.dealerApplication
                  }
                  data-slot="dealer-application"
                >
                  {t("landing.dealers.apply_cta")}
                </Link>
              </Button>
            ) : null}
          </div>
        </div>
      </section>

      <section
        id="iletisim"
        aria-labelledby="landing-contact"
        className="mx-auto w-full max-w-6xl px-4 py-14 sm:px-6"
      >
        <div className="mx-auto mb-8 flex max-w-2xl flex-col gap-2 text-center">
          <h2 id="landing-contact" className="text-2xl font-bold sm:text-3xl">
            {t("landing.contact.title")}
          </h2>
          <p className="text-muted-foreground">
            {t("landing.contact.description")}
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <ContactCard
            icon={<Phone className="size-5" aria-hidden />}
            title={t("landing.contact.phones_title")}
          >
            {CONTACT.phones.map((p) => (
              <a
                key={p.tel}
                href={`tel:${p.tel}`}
                dir="ltr"
                className="hover:text-foreground"
              >
                {p.label}: {p.display}
              </a>
            ))}
          </ContactCard>
          <ContactCard
            icon={<Mail className="size-5" aria-hidden />}
            title={t("landing.contact.emails_title")}
          >
            {CONTACT.emails.map((e) => (
              <a
                key={e}
                href={`mailto:${e}`}
                dir="ltr"
                className="hover:text-foreground"
              >
                {e}
              </a>
            ))}
          </ContactCard>
          <ContactCard
            icon={<MapPin className="size-5" aria-hidden />}
            title={t("landing.contact.address_title")}
          >
            <span>{CONTACT.address}</span>
          </ContactCard>
        </div>
      </section>

      <footer className="mt-auto border-t bg-[#00140c] text-emerald-100/80">
        <div className="mx-auto flex w-full max-w-6xl flex-col items-center gap-4 px-4 py-8 sm:px-6 md:flex-row md:justify-between">
          <p className="text-sm">
            {t("landing.footer.copyright", { year: new Date().getFullYear() })}
          </p>
          <nav
            aria-label={t("landing.language")}
            className="flex flex-wrap justify-center gap-x-3 gap-y-1 text-xs"
          >
            {SUPPORTED_LOCALES.map((l) => (
              <a
                key={l}
                href={`/?lang=${l}`}
                lang={l}
                hrefLang={l}
                aria-current={l === locale ? "true" : undefined}
                className={
                  l === locale
                    ? "font-semibold text-white"
                    : "underline-offset-2 hover:text-white hover:underline"
                }
              >
                {LOCALE_NAMES[l]}
              </a>
            ))}
          </nav>
        </div>
      </footer>
    </main>
  );
}

function ContactCard({
  icon,
  title,
  children,
}: {
  icon: ReactNode;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="bg-card flex flex-col items-center gap-3 rounded-2xl border p-5 text-center shadow-sm">
      <span className="text-muted-foreground">{icon}</span>
      <h3 className="font-semibold">{title}</h3>
      <div className="text-muted-foreground flex flex-col gap-1 text-sm">
        {children}
      </div>
    </div>
  );
}
