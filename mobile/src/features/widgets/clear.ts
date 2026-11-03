// Çıkış ve sahip değişiminde widget içeriğini siler. Hiçbir adım oturumu kapatmayı engellemez.
import { i18n, isRTL } from '@/core/i18n';

import { CUSTOMER_WIDGETS, WIDGET_NAMES } from './config';
import { widgetFor, widgetsAvailable } from './layouts';
import { signedOutProps, signedOutTitleKey } from './signed-out';
import { removeSyncRecord, syncGate } from './sync-state';
import { translator } from './translators';

/**
 * Sekiz widget'a giriş kartı yazar, müşteri widget'larının araç seçeneklerini
 * sıfırlar ve paylaşılan alandaki görselleri siler. Nesli artırmaz: sahip
 * değişiminde çalışan eşitleme `alive` ile bileti sorar; her yazımdan önce
 * bakılır, böylece geç kalan temizlik daha yeni bir eşitlemenin yazımını ezmez.
 */
export async function wipeWidgets(alive: () => boolean = () => true): Promise<void> {
  if (!widgetsAvailable()) return;
  let images: typeof import('./images') | null = null;
  let markUri = '';
  let markMonoUri = '';
  try {
    images = await import('./images');
    if (!alive()) return;
    const marks = await images.storeMarks();
    markUri = marks.markUri ?? '';
    markMonoUri = marks.markMonoUri ?? '';
  } catch {
    // İşaretsiz çizilir.
  }

  // Giriş kapalı metin o anki dille üretilir.
  if (!alive()) return;
  const lang = i18n.language;
  const t = translator(lang, 'widgets');
  for (const name of WIDGET_NAMES) {
    if (!alive()) return;
    try {
      const audience = CUSTOMER_WIDGETS.includes(name) ? 'customer' : 'dealer';
      widgetFor(name)?.updateSnapshot(
        signedOutProps(t, audience, { rtl: isRTL(lang), markUri, markMonoUri, eyebrow: t(signedOutTitleKey(name)) }),
      );
    } catch {
      // Bir widget yazılamadı: diğerleri devam eder.
    }
  }
  for (const name of CUSTOMER_WIDGETS) {
    if (!alive()) return;
    try {
      widgetFor(name)?.setConfigurationParameterEnum?.('vehicle', undefined);
    } catch {
      // Seçenekler bir sonraki eşitlemede yeniden yazılır.
    }
  }
  if (!alive()) return;
  try {
    images?.purgeAll();
  } catch {
    // Kalan dosyalar bir sonraki `reconcile`da silinir.
  }
}

/** Çıkış: uçuştaki eşitlemeler yazamaz, widget'lar giriş kartına döner, kayıt silinir. Asla reddetmez. */
export async function clearWidgets(): Promise<void> {
  try {
    syncGate.invalidate();
  } catch {
    // Kapı bellek içidir; atmaz.
  }
  // Temizlik sürerken yeni bir oturum eşitleme başlatırsa, geç kalan temizlik onun yazımlarını ezmez.
  const startedAt = syncGate.latest();
  try {
    await wipeWidgets(() => syncGate.latest() === startedAt);
  } catch {
    // Sessiz.
  }
  try {
    removeSyncRecord();
  } catch {
    // Sessiz.
  }
}
