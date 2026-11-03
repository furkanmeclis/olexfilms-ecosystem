// Widget katmanının paylaştığı sabitler. Saf dosya: React Native/Expo import'u yok.

export const WIDGET_NAMES = [
  'OlexKoruma',
  'OlexGarantiKarti',
  'OlexHizmetDurumu',
  'OlexPanom',
  'OlexHizliIslem',
  'OlexSon30Gun',
  'OlexBekleyenler',
  'OlexCiro',
] as const;

export type WidgetName = (typeof WIDGET_NAMES)[number];

export const CUSTOMER_WIDGETS: readonly WidgetName[] = [
  'OlexKoruma',
  'OlexGarantiKarti',
  'OlexHizmetDurumu',
];

export const DEALER_WIDGETS: readonly WidgetName[] = [
  'OlexPanom',
  'OlexHizliIslem',
  'OlexSon30Gun',
  'OlexBekleyenler',
  'OlexCiro',
];

/** Araç seçiminde widget'a yazılan en fazla araç sayısı. */
export const MAX_VEHICLES = 10;

/** Araç kapağı görselinin yeniden boyutlandırıldığı en büyük genişlik. */
export const HERO_MAX_WIDTH_PX = 480;

/** Paylaşılan alana yazılan Olex işareti (ana ekran) ve beyaz tek renkli sürüm (kilit ekranı). */
export const MARK_FILE = 'olex-mark.png';
export const MARK_MONO_FILE = 'olex-mark-mono.png';

/** İki eşitleme arasındaki en kısa süre. */
export const SYNC_MIN_INTERVAL_MS = 15 * 60_000;

/** Bir widget'ın `props` yükünün en büyük boyutu (UTF-8 bayt). */
export const PROPS_BUDGET_BYTES = 32_768;

const HOUR_MS = 60 * 60_000;

// Sessiz push yok: veri yalnız uygulama açılınca tazelenir. Garanti gibi
// yavaş değişen veriye iki saatte "güncellenemedi" yazmak yanlış alarm olur.
export const STALE_AFTER_MS: Record<WidgetName, number> = {
  OlexKoruma: 7 * 24 * HOUR_MS,
  OlexGarantiKarti: 7 * 24 * HOUR_MS,
  OlexSon30Gun: 24 * HOUR_MS,
  OlexCiro: 24 * HOUR_MS,
  OlexHizmetDurumu: 2 * HOUR_MS,
  OlexPanom: 2 * HOUR_MS,
  OlexHizliIslem: 2 * HOUR_MS,
  OlexBekleyenler: 2 * HOUR_MS,
};
