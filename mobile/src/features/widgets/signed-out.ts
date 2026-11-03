// Giriş kapalı widget içeriği. Saf dosya: çeviri, dil ve görsel yolları parametre gelir.
import type { WidgetName } from './config.ts';
import type { TFn } from './format.ts';
import { links } from './links.ts';
import { WIDGET_THEME } from './theme.ts';
import type { BaseProps } from './types.ts';

export type SignedOutExtras = {
  rtl: boolean;
  markUri: string;
  markMonoUri: string;
  /** Kartın üst başlığı; widget'ın kendi adı. */
  eyebrow: string;
};

const TITLE_KEYS: Record<WidgetName, string> = {
  OlexKoruma: 'koruma.eyebrow',
  OlexGarantiKarti: 'card.title',
  OlexHizmetDurumu: 'service.title',
  OlexPanom: 'panom.title',
  OlexHizliIslem: 'quick.title',
  OlexSon30Gun: 'kpi.title',
  OlexBekleyenler: 'pending.title',
  OlexCiro: 'revenue.title',
};

/** Widget'ın `widgets` namespace'indeki başlık anahtarı. */
export const signedOutTitleKey = (name: WidgetName) => TITLE_KEYS[name];

/**
 * Çıkış sonrası her widget'a yazılan içerik: yalnız ortak alanlar ve giriş kartı.
 * Araç, hizmet, sayı ya da kişi verisi taşımaz.
 */
export function signedOutProps(t: TFn, audience: 'customer' | 'dealer', extras: SignedOutExtras): BaseProps {
  return {
    state: 'signed_out',
    rtl: extras.rtl,
    theme: WIDGET_THEME,
    markUri: extras.markUri,
    markMonoUri: extras.markMonoUri,
    url: links.home(),
    updatedLabel: '',
    staleLabel: null,
    card: {
      eyebrow: extras.eyebrow,
      title: t(audience === 'customer' ? 'state.customer_signed_out_title' : 'state.dealer_signed_out_title'),
      cta: t('state.sign_in'),
    },
  };
}
