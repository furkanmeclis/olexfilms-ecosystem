import { GlassView } from 'expo-glass-effect';
import type { ReactNode } from 'react';
import { I18nManager, ScrollView, View } from 'react-native';
import Animated, { Extrapolation, interpolate, type SharedValue, useAnimatedStyle } from 'react-native-reanimated';
import { useUniwind } from 'uniwind';

import { glassAvailable } from '@/ui/glass-card';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { Text } from '@/ui/text';

/** Visual drift between neighbouring pages, in points. */
const PARALLAX = 24;
/** The badge "settles" onto the surface by this much as its page arrives. */
const SETTLE = 10;

function Surface({ size, children }: { size: number; children: ReactNode }) {
  const { theme } = useUniwind();
  const shape = { width: size, height: size, borderRadius: 36, borderCurve: 'continuous' as const };
  if (glassAvailable()) {
    return (
      <GlassView
        glassEffectStyle="regular"
        colorScheme={theme === 'dark' ? 'dark' : 'light'}
        style={[shape, { alignItems: 'center', justifyContent: 'center' }]}>
        {children}
      </GlassView>
    );
  }
  return (
    <View className="items-center justify-center border border-border bg-card" style={shape}>
      {children}
    </View>
  );
}

export type OnboardingPageContent = { icon: IconName; badge: IconName; title: string; body: string };

type Props = OnboardingPageContent & {
  index: number;
  width: number;
  /** Scroll position in pages (0 = first page fully visible). */
  progress: SharedValue<number>;
  reduceMotion: boolean;
};

/** One onboarding page: a large symbol on a glass (or flat card) surface, then title and body. */
export function OnboardingPage({ icon, badge, title, body, index, width, progress, reduceMotion }: Props) {
  const range = [index - 1, index, index + 1];
  const visual = Math.min(width - 112, 240);

  const visualStyle = useAnimatedStyle(() => {
    const opacity = interpolate(progress.value, range, [0, 1, 0], Extrapolation.CLAMP);
    if (reduceMotion) return { opacity };
    return {
      opacity,
      transform: [{ translateX: interpolate(progress.value, range, [PARALLAX, 0, -PARALLAX], Extrapolation.CLAMP) }],
    };
  });

  const badgeStyle = useAnimatedStyle(() => {
    if (reduceMotion) return {};
    return {
      transform: [
        { translateY: interpolate(progress.value, [index - 1, index], [-SETTLE, 0], Extrapolation.CLAMP) },
      ],
    };
  });

  const textStyle = useAnimatedStyle(() => ({
    opacity: interpolate(progress.value, range, [0, 1, 0], Extrapolation.CLAMP),
  }));

  return (
    // Scrolls vertically only when large Dynamic Type overflows a small screen; centred otherwise.
    <ScrollView
      // The pager is pinned LTR (see onboarding-screen); the content follows the language.
      style={{ width, direction: I18nManager.isRTL ? 'rtl' : 'ltr' }}
      contentContainerClassName="grow items-center justify-center gap-12 px-8 py-6"
      showsVerticalScrollIndicator={false}
      alwaysBounceVertical={false}
      accessible
      accessibilityLabel={`${title}. ${body}`}>
      <Animated.View style={visualStyle} importantForAccessibility="no-hide-descendants">
        {/* A soft halo behind the surface gives the glass something to refract. */}
        <View
          className="absolute rounded-full bg-primary opacity-10"
          style={{ width: visual * 1.15, height: visual * 1.15, top: -visual * 0.075, start: -visual * 0.075 }}
        />
        <Surface size={visual}>
          <Icon name={icon} tone="primary" size={Math.round(visual * 0.38)} />
        </Surface>
        <Animated.View
          className="absolute size-16 items-center justify-center rounded-full border-4 border-background bg-primary"
          style={[{ bottom: -10, end: -10 }, badgeStyle]}>
          <Icon name={badge} tone="primary-foreground" size={26} weight="medium" />
        </Animated.View>
      </Animated.View>

      <Animated.View style={textStyle} className="w-full max-w-[360px] items-center gap-3">
        <Text variant="title" className="text-center">
          {title}
        </Text>
        <Text variant="callout" tone="muted" className="text-center">
          {body}
        </Text>
      </Animated.View>
    </ScrollView>
  );
}
