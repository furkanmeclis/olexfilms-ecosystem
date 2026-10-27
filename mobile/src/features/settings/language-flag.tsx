import { Image } from 'expo-image';
import { View } from 'react-native';

import type { Language } from '@/core/i18n';

/**
 * Flag of the country a language is shown for (images, not emoji: emoji flags
 * render differently per platform and not at all on some Android builds).
 * Public-domain flags from flagcdn.com, 160 px wide.
 */
const FLAGS: Record<Language, number> = {
  tr: require('../../../assets/images/flags/tr.png'),
  en: require('../../../assets/images/flags/gb.png'),
  bg: require('../../../assets/images/flags/bg.png'),
  de: require('../../../assets/images/flags/de.png'),
  el: require('../../../assets/images/flags/gr.png'),
  uk: require('../../../assets/images/flags/ua.png'),
  ru: require('../../../assets/images/flags/ru.png'),
  fr: require('../../../assets/images/flags/fr.png'),
  es: require('../../../assets/images/flags/es.png'),
  it: require('../../../assets/images/flags/it.png'),
  'zh-CN': require('../../../assets/images/flags/cn.png'),
  az: require('../../../assets/images/flags/az.png'),
  ar: require('../../../assets/images/flags/sa.png'),
};

/** Same-size tile for every flag (their ratios differ), with a hairline so white stripes keep an edge. */
export function LanguageFlag({ code }: { code: Language }) {
  return (
    <View className="h-5 w-7 overflow-hidden rounded-[4px] border border-border" style={{ borderCurve: 'continuous' }}>
      <Image source={FLAGS[code]} contentFit="cover" style={{ width: '100%', height: '100%' }} accessibilityIgnoresInvertColors />
    </View>
  );
}
