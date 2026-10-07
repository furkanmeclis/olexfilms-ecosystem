import type { Metadata } from "next";

import { localeDir } from "@/config/i18n";
import {
  UnsubscribeCard,
  type UnsubscribeTexts,
} from "@/features/campaigns/components/unsubscribe-card";
import { publicPageLocale } from "@/features/public-leads/lib/page-locale";
import { translate } from "@/lib/i18n/messages";

/**
 * Unsubscribe page of the campaign e-mail link (TEC-407):
 * `/abonelik-iptal/{token}`. No session; the button posts the token to Go
 * `POST /v1/public/campaigns/unsubscribe` through the BFF, which writes the
 * marketing opt-out. Language: `?lang=`, else cookie, else Accept-Language.
 */
export const dynamic = "force-dynamic";

type PageProps = {
  params: Promise<{ token: string }>;
  searchParams: Promise<{ lang?: string | string[] }>;
};

const KEYS: (keyof UnsubscribeTexts)[] = [
  "title",
  "body",
  "confirm",
  "done_title",
  "done_body",
  "invalid_title",
  "invalid_body",
  "error",
  "home",
];

export async function generateMetadata({
  searchParams,
}: PageProps): Promise<Metadata> {
  const { lang } = await searchParams;
  const { locale } = await publicPageLocale(lang);
  return {
    title: translate(locale, "common.campaign_unsubscribe.page_title"),
    robots: { index: false, follow: false, nocache: true },
    referrer: "no-referrer",
  };
}

export default async function CampaignUnsubscribePage({
  params,
  searchParams,
}: PageProps) {
  const { token } = await params;
  const { lang } = await searchParams;
  const { locale } = await publicPageLocale(lang);
  const texts = Object.fromEntries(
    KEYS.map((k) => [k, translate(locale, `common.campaign_unsubscribe.${k}`)]),
  ) as UnsubscribeTexts;
  return (
    <UnsubscribeCard
      token={token}
      texts={texts}
      lang={locale}
      dir={localeDir(locale)}
    />
  );
}
