import { PublicQuoteNotFoundNotice } from "@/features/public-leads/components/public-quote-view";
import { PublicPageShell } from "@/features/public-leads/components/public-page-shell";
import { publicPageLocale } from "@/features/public-leads/lib/page-locale";
import { routes } from "@/config/routes";
import { translate } from "@/lib/i18n/messages";

/**
 * 404 of `/teklif/{token}` (TEC-320): unknown, malformed or withdrawn
 * token. not-found.tsx gets no params, so the language comes from the
 * cookie or Accept-Language.
 */
export default async function PublicQuoteNotFound() {
  const { locale } = await publicPageLocale();
  return (
    <PublicPageShell
      locale={locale}
      path={routes.public.root}
      slot="public-quote"
    >
      <PublicQuoteNotFoundNotice
        t={(key, params) => translate(locale, key, params)}
      />
    </PublicPageShell>
  );
}
