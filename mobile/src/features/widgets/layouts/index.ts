// Widget kayıt defteri: eşitleme ve temizleme native modüle yalnız buradan erişir.
// `expo-widgets` bu dosyada hiç import edilmez; native modülü olmayan derlemede
// kayıt boş kalır ve eşitleme ile temizleme hiçbir şey yapmaz. iOS'ta Metro `index.ios.ts`'i seçer;
// o da gerçek widget'ları korumalı `require` ile kaydeder.
import type { WidgetConfigurationEnum } from 'expo-widgets';

import type { WidgetName } from '../config';

/** `expo-widgets` `Widget` sınıfının eşitlemenin kullandığı kısmı. */
export interface WidgetHandle {
  updateTimeline(entries: { date: Date; props: object }[]): void;
  updateSnapshot(props: object): void;
  /** Yalnız müşteri widget'larında (`vehicle` parametresi) vardır. */
  setConfigurationParameterEnum?(parameterName: 'vehicle', options?: WidgetConfigurationEnum[]): void;
}

export type WidgetRegistry = Partial<Record<WidgetName, WidgetHandle>>;

/** Bu derlemede kullanılabilen widget'lar; native modül yoksa boş. */
export const registry: WidgetRegistry = {};

export function widgetFor(name: WidgetName): WidgetHandle | null {
  return registry[name] ?? null;
}

export function widgetsAvailable(): boolean {
  return Object.keys(registry).length > 0;
}
