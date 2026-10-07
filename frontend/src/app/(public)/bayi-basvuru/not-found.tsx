import { SearchX } from "lucide-react";

import { routes } from "@/config/routes";
import {
  PublicNotice,
  PublicPageShell,
} from "@/features/public-leads/components/public-page-shell";
import { publicPageLocale } from "@/features/public-leads/lib/page-locale";
import { translate } from "@/lib/i18n/messages";

/** 404 of `/bayi-basvuru` while the form is closed (TEC-320). */
export default async function DealerApplicationNotFound() {
  const { locale } = await publicPageLocale();
  const t = (key: string) => translate(locale, key);
  return (
    <PublicPageShell
      locale={locale}
      path={routes.public.dealerApplication}
      slot="dealer-application"
    >
      <PublicNotice
        screen="not-found"
        icon={<SearchX className="size-8" aria-hidden />}
        title={t("landing.dealer_application.not_found_title")}
        text={t("landing.dealer_application.not_found_body")}
      />
    </PublicPageShell>
  );
}
