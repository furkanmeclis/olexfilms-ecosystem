import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";

import {
  LOCALE_NAMES,
  SUPPORTED_LOCALES,
  localeDir,
  type AppLocale,
} from "@/config/i18n";
import { routes } from "@/config/routes";
import { translate } from "@/lib/i18n/messages";

export type PublicPageShellProps = {
  locale: AppLocale;
  /** Path of this page without query, for the language links. */
  path: string;
  /** data-slot of <main> (tests, analytics). */
  slot: string;
  children: ReactNode;
};

/**
 * Frame of the public lead pages (TEC-320) in the landing (TEC-247) look:
 * the dark green header with the logo, a centered content column and the
 * language footer. Server rendered, logical properties only (RTL for ar).
 */
export function PublicPageShell({
  locale,
  path,
  slot,
  children,
}: PublicPageShellProps) {
  const t = (key: string) => translate(locale, key);
  return (
    <main
      lang={locale}
      dir={localeDir(locale)}
      data-slot={slot}
      className="bg-muted/30 text-foreground flex min-h-dvh flex-col"
    >
      <div className="bg-gradient-to-br from-[#00140c] via-[#003d25] to-[#00140c] text-white">
        <header className="mx-auto flex w-full max-w-3xl items-center justify-between gap-3 px-4 py-4 sm:px-6">
          <Link
            href={`${routes.public.root}?lang=${encodeURIComponent(locale)}`}
            aria-label="Olex Films"
          >
            <Image
              src="/images/olex-logo-yatay.svg"
              alt="Olex Films"
              width={120}
              height={48}
              priority
              unoptimized
              className="h-9 w-auto"
            />
          </Link>
        </header>
      </div>

      <div className="mx-auto flex w-full max-w-3xl flex-1 flex-col gap-4 px-4 py-6 sm:px-6 sm:py-10">
        {children}
      </div>

      <footer className="mt-auto border-t bg-[#00140c] text-emerald-100/80">
        <nav
          aria-label={t("landing.language")}
          className="mx-auto flex w-full max-w-3xl flex-wrap justify-center gap-x-3 gap-y-1 px-4 py-6 text-xs sm:px-6"
        >
          {SUPPORTED_LOCALES.map((l) => (
            <a
              key={l}
              href={`${path}?lang=${encodeURIComponent(l)}`}
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
      </footer>
    </main>
  );
}

/** Centered message card (not found, rate limited, error). */
export function PublicNotice({
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
      className="bg-card mx-auto flex w-full max-w-md flex-col items-center gap-3 rounded-2xl border p-6 text-center shadow-sm"
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
