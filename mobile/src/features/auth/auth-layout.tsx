import { GlassView } from 'expo-glass-effect';
import { router } from 'expo-router';
import { type ReactNode, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Keyboard, Platform, Pressable, StyleSheet, View } from 'react-native';
import Animated, { FadeIn, FadeOut } from 'react-native-reanimated';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import Svg, { Defs, LinearGradient, RadialGradient, Rect, Stop } from 'react-native-svg';
import { useUniwind } from 'uniwind';

import { BrandLogo } from '@/ui/brand-logo';
import { glassAvailable } from '@/ui/glass-card';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

/** Deep brand green of the sign-in hero; fixed in both themes like the web's auth panel. */
export const HERO_BOTTOM = '#00261a';

/** True while the software keyboard is up (iOS reports it before the animation). */
export function useKeyboardVisible() {
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    const show = Keyboard.addListener(Platform.OS === 'ios' ? 'keyboardWillShow' : 'keyboardDidShow', () => setVisible(true));
    const hide = Keyboard.addListener(Platform.OS === 'ios' ? 'keyboardWillHide' : 'keyboardDidHide', () => setVisible(false));
    return () => {
      show.remove();
      hide.remove();
    };
  }, []);
  return visible;
}

/**
 * Brand panel above the sign-in sheet: green gradient, a gold glow and the
 * crest as a watermark. The headline steps aside while the keyboard is up.
 */
export function AuthHero({ title, subtitle }: { title: string; subtitle: string }) {
  const { t } = useTranslation('auth');
  const insets = useSafeAreaInsets();
  const keyboard = useKeyboardVisible();
  // Percent sizes on an absolute Svg only covered the top padding; draw at the measured size.
  const [size, setSize] = useState({ width: 0, height: 0 });

  return (
    <View
      className="flex-1 overflow-hidden"
      style={{ paddingTop: insets.top + 12, backgroundColor: HERO_BOTTOM }}
      onLayout={(e) => setSize({ width: e.nativeEvent.layout.width, height: e.nativeEvent.layout.height })}>
      <Svg style={StyleSheet.absoluteFill} width={size.width} height={size.height}>
        <Defs>
          <LinearGradient id="hero" x1="0" y1="0" x2="0.6" y2="1">
            <Stop offset="0" stopColor="#0a5a37" />
            <Stop offset="1" stopColor={HERO_BOTTOM} />
          </LinearGradient>
          <RadialGradient id="glow" cx="0.95" cy="0.05" r="0.75">
            <Stop offset="0" stopColor="#f2ac4a" stopOpacity="0.28" />
            <Stop offset="1" stopColor="#f2ac4a" stopOpacity="0" />
          </RadialGradient>
        </Defs>
        <Rect width={size.width} height={size.height} fill="url(#hero)" />
        <Rect width={size.width} height={size.height} fill="url(#glow)" />
      </Svg>
      <View pointerEvents="none" className="absolute -end-16 bottom-4 opacity-10" importantForAccessibility="no-hide-descendants">
        <BrandLogo variant="mark" height={300} />
      </View>

      <View className="flex-1 justify-between px-6 pb-12">
        <BrandLogo variant="logo" tone="dark" height={36} label={t('common:app_name')} />
        {keyboard ? null : (
          <Animated.View entering={FadeIn.duration(200)} exiting={FadeOut.duration(120)} className="gap-3">
            <Text variant="largeTitle" className="text-on-media">
              {title}
            </Text>
            <Text variant="body" className="text-on-media/75">
              {subtitle}
            </Text>
          </Animated.View>
        )}
      </View>
    </View>
  );
}

/** Round back button for the headerless sign-in steps (the swipe-back gesture still works). */
export function BackButton() {
  const { t } = useTranslation();
  const { theme } = useUniwind();
  const back = () => (router.canGoBack() ? router.back() : router.replace('/sign-in'));
  const button = (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('common:back')}
      hitSlop={8}
      onPress={back}
      className="size-11 items-center justify-center rounded-full active:opacity-60">
      <Icon name="back" size={20} weight="semibold" />
    </Pressable>
  );
  if (glassAvailable()) {
    return (
      <GlassView isInteractive glassEffectStyle="regular" colorScheme={theme === 'dark' ? 'dark' : 'light'} style={{ borderRadius: 22 }}>
        {button}
      </GlassView>
    );
  }
  return <View className="rounded-full border border-border bg-card">{button}</View>;
}

type StepProps = {
  title: string;
  subtitle?: ReactNode;
  children: ReactNode;
};

/**
 * A sign-in step after the first screen (code, password, 2FA): back button,
 * title, content. No ScrollView: the content is short and sits above the
 * keyboard, so nothing moves when a field takes focus.
 */
export function AuthStep({ title, subtitle, children }: StepProps) {
  const insets = useSafeAreaInsets();
  return (
    <View className="flex-1 bg-background" style={{ paddingTop: insets.top + 4 }}>
      <View className="flex-row px-4 py-1">
        <BackButton />
      </View>
      <View className="flex-1 gap-7 px-6 pt-5">
        <View className="gap-2">
          <Text variant="largeTitle">{title}</Text>
          {subtitle ? (
            <Text variant="callout" tone="muted">
              {subtitle}
            </Text>
          ) : null}
        </View>
        {children}
      </View>
    </View>
  );
}
