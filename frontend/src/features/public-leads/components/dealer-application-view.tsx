import { CircleAlert, Store } from "lucide-react";

import type { AppLocale } from "@/config/i18n";
import { routes } from "@/config/routes";
import { translate } from "@/lib/i18n/messages";

import {
  applicationMessages,
  DEALER_APPLICATION_PATH,
  type ApplicationConfig,
} from "../lib/dealer-application";
import { DealerApplicationForm } from "./dealer-application-form";
import { PublicNotice, PublicPageShell } from "./public-page-shell";

export type DealerApplicationViewProps = {
  /** "closed" never reaches here: the page answers 404. */
  config: Exclude<ApplicationConfig, "closed">;
  locale: AppLocale;
};

/**
 * `/bayi-basvuru` body (TEC-320): intro and the client form while the
 * system setting `leads.dealer_application_enabled` is on; an error card
 * when Go could not tell.
 */
export function DealerApplicationView({
  config,
  locale,
}: DealerApplicationViewProps) {
  const t = (key: string) => translate(locale, key);
  return (
    <PublicPageShell
      locale={locale}
      path={DEALER_APPLICATION_PATH}
      slot="dealer-application"
    >
      {config === "open" ? (
        <section
          data-screen="form"
          className="bg-card flex flex-col gap-5 rounded-2xl border p-5 shadow-sm sm:p-6"
        >
          <header className="flex flex-col gap-1">
            <span className="text-muted-foreground flex items-center gap-1.5 text-xs font-medium tracking-wide uppercase">
              <Store className="size-4" aria-hidden />
              {t("landing.dealer_application.eyebrow")}
            </span>
            <h1 className="text-2xl font-bold">
              {t("landing.dealer_application.title")}
            </h1>
            <p className="text-muted-foreground text-sm">
              {t("landing.dealer_application.description")}
            </p>
          </header>
          <DealerApplicationForm
            locale={locale}
            messages={applicationMessages(locale)}
            homeHref={`${routes.public.root}?lang=${encodeURIComponent(locale)}`}
          />
        </section>
      ) : (
        <PublicNotice
          screen="error"
          icon={<CircleAlert className="size-8" aria-hidden />}
          title={t("landing.dealer_application.error_title")}
          text={t("landing.dealer_application.error_body")}
        />
      )}
    </PublicPageShell>
  );
}
