import { Image, type ImageSource } from 'expo-image';
import { useTranslation } from 'react-i18next';
import { I18nManager, StyleSheet, View } from 'react-native';
import Animated, { Extrapolation, interpolate, type SharedValue, useAnimatedStyle } from 'react-native-reanimated';
import Svg, { Defs, LinearGradient, Rect, Stop } from 'react-native-svg';

import { BrandLogo } from '@/ui/brand-logo';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { Text } from '@/ui/text';

/**
 * Ground of the onboarding film; fixed in both themes like the brand's site
 * (black studio, green light, gold crest).
 */
export const FILM_BG = '#050807';

/** Share of the screen the photo covers; the text sits on the faded lower edge. */
const PHOTO_SHARE = 0.62;
/** Height kept below the photo for the text and the fixed buttons, unless a page asks for more. */
const RESERVE = 380;
/** How much wider than the screen the photo is drawn, so it can drift while swiping. */
const DRIFT = 0.18;
/** Zoom of a page that is one swipe away; it settles to 1 as the page arrives. */
const ZOOM = 0.08;

export type OnboardingPageContent = {
  key: string;
  image: ImageSource | number;
  eyebrow: string;
  title: string;
  body?: string;
  /** Extra layer drawn over the photo (the warranty card). */
  overlay?: 'warranty';
  /** Rows under the title instead of a paragraph (the last page). */
  features?: { icon: IconName; label: string }[];
  /** Figures under the paragraph (the first page). */
  stats?: { value: string; label: string }[];
  /** Height this page needs below the photo (longer text, feature rows). */
  reserve?: number;
};

type Props = OnboardingPageContent & {
  index: number;
  width: number;
  height: number;
  progress: SharedValue<number>;
  reduceMotion: boolean;
};

function Fade({ height }: { height: number }) {
  return (
    <Svg width="100%" height={height} style={StyleSheet.absoluteFill}>
      <Defs>
        <LinearGradient id="top" x1="0" y1="0" x2="0" y2="1">
          <Stop offset="0" stopColor={FILM_BG} stopOpacity="0.75" />
          <Stop offset="0.22" stopColor={FILM_BG} stopOpacity="0" />
          <Stop offset="0.55" stopColor={FILM_BG} stopOpacity="0" />
          <Stop offset="1" stopColor={FILM_BG} stopOpacity="1" />
        </LinearGradient>
      </Defs>
      <Rect width="100%" height={height} fill="url(#top)" />
    </Svg>
  );
}

/** A sample digital warranty, drawn over the photo: what the customer will carry in the app. */
function WarrantyCard() {
  const { t } = useTranslation('onboarding');
  return (
    <View
      className="w-[300px] gap-4 rounded-[26px] border border-on-media/15 p-5"
      style={{ backgroundColor: 'rgba(5, 8, 7, 0.78)', borderCurve: 'continuous' }}>
      <View className="flex-row items-center justify-between">
        <BrandLogo variant="logo" tone="dark" height={22} />
        <View className="flex-row items-center gap-1 rounded-full bg-brand/15 px-2.5 py-1">
          <Icon name="check" tone="brand" size={12} weight="semibold" />
          <Text variant="caption" className="font-semibold text-brand">
            {t('film_card_verified')}
          </Text>
        </View>
      </View>
      <View className="gap-1">
        <Text variant="caption" className="text-on-media/60">
          {t('film_card_title')}
        </Text>
        <Text variant="title" className="text-on-media">
          {t('film_card_product')}
        </Text>
        <Text variant="footnote" className="text-on-media/70">
          {t('film_card_parts')}
        </Text>
      </View>
      <View className="flex-row items-end justify-between">
        <View className="gap-0.5">
          <Text variant="caption" className="text-on-media/60">
            {t('film_card_term_label')}
          </Text>
          <Text variant="headline" className="text-on-media">
            {t('film_card_term')}
          </Text>
        </View>
        <View className="size-12 items-center justify-center rounded-xl bg-on-media/10">
          <Icon name="qr" tone="on-media" size={28} />
        </View>
      </View>
    </View>
  );
}

export function OnboardingPage({
  image,
  eyebrow,
  title,
  body,
  overlay,
  features,
  stats,
  reserve = RESERVE,
  index,
  width,
  height,
  progress,
  reduceMotion,
}: Props) {
  const photoHeight = Math.round(Math.min(height * PHOTO_SHARE, height - reserve));

  const photoStyle = useAnimatedStyle(() => {
    const d = progress.value - index;
    if (reduceMotion) return { transform: [{ translateX: 0 }, { scale: 1 }] };
    return {
      transform: [
        { translateX: -d * width * DRIFT },
        { scale: interpolate(Math.abs(d), [0, 1], [1, 1 + ZOOM], Extrapolation.CLAMP) },
      ],
    };
  });

  const textStyle = useAnimatedStyle(() => {
    const d = Math.abs(progress.value - index);
    return {
      opacity: interpolate(d, [0, 0.6], [1, 0], Extrapolation.CLAMP),
      transform: [{ translateY: reduceMotion ? 0 : interpolate(d, [0, 1], [0, 24], Extrapolation.CLAMP) }],
    };
  });

  return (
    // The pager pins LTR for its paging math; each page restores the reading direction.
    <View style={{ width, height, direction: I18nManager.isRTL ? 'rtl' : 'ltr' }}>
      <View className="absolute inset-x-0 top-0 overflow-hidden" style={{ height: photoHeight }}>
        <Animated.View
          style={[{ position: 'absolute', top: 0, bottom: 0, width: width * (1 + DRIFT * 2), left: -width * DRIFT }, photoStyle]}>
          <Image source={image} contentFit="cover" style={StyleSheet.absoluteFill} transition={0} accessibilityIgnoresInvertColors />
        </Animated.View>
        <Fade height={photoHeight} />
        {overlay === 'warranty' ? (
          <View className="absolute inset-x-0 items-center" style={{ bottom: photoHeight * 0.18 }}>
            <WarrantyCard />
          </View>
        ) : null}
      </View>

      <Animated.View
        style={[{ position: 'absolute', left: 0, right: 0, top: photoHeight - 72 }, textStyle]}
        className="gap-3 px-7">
        <Text variant="caption" className="font-semibold tracking-[2px] text-brand">
          {eyebrow}
        </Text>
        <Text variant="largeTitle" className="text-on-media">
          {title}
        </Text>
        {body ? (
          <Text variant="body" className="text-on-media/70">
            {body}
          </Text>
        ) : null}
        {stats ? (
          <View className="mt-2 flex-row gap-6">
            {stats.map((s) => (
              <View key={s.label} className="gap-0.5">
                <Text variant="title" className="tabular-nums text-on-media">
                  {s.value}
                </Text>
                <Text variant="footnote" className="text-on-media/60">
                  {s.label}
                </Text>
              </View>
            ))}
          </View>
        ) : null}
        {features ? (
          <View className="mt-1 gap-3">
            {features.map((f) => (
              <View key={f.label} className="flex-row items-center gap-3">
                <View className="size-9 items-center justify-center rounded-full bg-on-media/10">
                  <Icon name={f.icon} tone="brand" size={17} />
                </View>
                <Text variant="callout" className="flex-1 text-on-media/85">
                  {f.label}
                </Text>
              </View>
            ))}
          </View>
        ) : null}
      </Animated.View>
    </View>
  );
}
