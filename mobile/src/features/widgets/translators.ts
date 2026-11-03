// Widget metinleri uygulamanın i18n örneğinden gelir: rakam biçimi (`core/digits`)
// yalnız gerçek örneğin global enterpolasyonuyla uygulanır.
import { i18n } from '@/core/i18n';

import type { TFn } from './format';

/** `lang` ve `namespace` için sabitlenmiş çeviri; dil değişince yeniden alınır. */
export function translator(lang: string, namespace: 'widgets' | 'customer'): TFn {
  const fixed = i18n.getFixedT(lang, namespace);
  return (key, options) => String(fixed(key, options));
}
