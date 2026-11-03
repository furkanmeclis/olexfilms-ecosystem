// iOS widget kayıt defteri. `index.ts` ile aynı dışa aktarımlar; Metro iOS'ta bunu seçer.
// `require` yalnız native modül varken çalışır: eski dev build'de ve testlerde layout'lar yüklenmez.
// `createWidget` her açılışta layout'u yeniden saklar (`WidgetObject.swift`), düzen değişikliği OTA ile gelir.
import { requireOptionalNativeModule } from 'expo';
import type { Widget } from 'expo-widgets';

import { i18n, isRTL } from '@/core/i18n';

import type { WidgetName } from '../config';
import { CUSTOMER_WIDGETS } from '../config';
import { stripNulls } from '../native-props';
import { signedOutProps, signedOutTitleKey } from '../signed-out';
import { translator } from '../translators';
import type { BaseProps } from '../types';
import type { WidgetHandle, WidgetRegistry } from './index';

export type { WidgetHandle, WidgetRegistry } from './index';

type Layout = { create: (initialProps: BaseProps) => Widget<any, any> };

// `require` tembeldir: import olsa native modül kontrolünden önce çalışırdı.
/* eslint-disable @typescript-eslint/no-require-imports */
const LAYOUTS: Record<WidgetName, () => Layout> = {
  OlexKoruma: () => require('./koruma.widget'),
  OlexGarantiKarti: () => require('./kart.widget'),
  OlexHizmetDurumu: () => require('./hizmet.widget'),
  OlexPanom: () => require('./panom.widget'),
  OlexHizliIslem: () => require('./hizli.widget'),
  OlexSon30Gun: () => require('./son30.widget'),
  OlexBekleyenler: () => require('./bekleyen.widget'),
  OlexCiro: () => require('./ciro.widget'),
};
/* eslint-enable @typescript-eslint/no-require-imports */

/**
 * İlk eşitlemeden önce sistemin çizdiği içerik. Verilmezse layout `props` olmadan
 * çağrılır (`Utils.swift` `parseTimeline`) ve widget hata verir. İşaret henüz yazılmamış olabilir.
 */
function initialProps(name: WidgetName): BaseProps {
  const lang = i18n.language;
  const t = translator(lang, 'widgets');
  return stripNulls(
    signedOutProps(t, CUSTOMER_WIDGETS.includes(name) ? 'customer' : 'dealer', {
      rtl: isRTL(lang),
      markUri: '',
      markMonoUri: '',
      eyebrow: t(signedOutTitleKey(name)),
    }),
  );
}

/** Native'e giden her `props`'tan `null` temizlenir; bkz. `native-props.ts`. */
function handleFor(widget: Widget<any, any>): WidgetHandle {
  return {
    updateTimeline: (entries) =>
      widget.updateTimeline(entries.map((entry) => ({ date: entry.date, props: stripNulls(entry.props) }))),
    updateSnapshot: (props) => widget.updateSnapshot(stripNulls(props)),
    setConfigurationParameterEnum: (name, options) => widget.setConfigurationParameterEnum(name, options),
  };
}

function load(): WidgetRegistry {
  const out: WidgetRegistry = {};
  if (!requireOptionalNativeModule('ExpoWidgets')) return out;
  for (const [name, layout] of Object.entries(LAYOUTS) as [WidgetName, () => Layout][]) {
    try {
      out[name] = handleFor(layout().create(initialProps(name)));
    } catch (e) {
      // Bir widget kaydedilemedi: uygulama ve diğer widget'lar devam eder.
      if (__DEV__) console.warn(`[widgets] ${name} kaydedilemedi`, e);
    }
  }
  return out;
}

/** Bu derlemede kullanılabilen widget'lar; native modül yoksa boş. */
export const registry: WidgetRegistry = load();

export function widgetFor(name: WidgetName): WidgetHandle | null {
  return registry[name] ?? null;
}

export function widgetsAvailable(): boolean {
  return Object.keys(registry).length > 0;
}
