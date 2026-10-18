import { router } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, useWindowDimensions, View } from 'react-native';
import Animated, {
  Easing,
  FadeIn,
  scrollTo,
  type SharedValue,
  useAnimatedReaction,
  useAnimatedRef,
  useAnimatedScrollHandler,
  useAnimatedStyle,
  useDerivedValue,
  useReducedMotion,
  useSharedValue,
  withTiming,
} from 'react-native-reanimated';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { scheduleOnRN } from 'react-native-worklets';

import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { OnboardingPage, type OnboardingPageContent } from './onboarding-page';
import { useOnboarding } from './onboarding-state';

const EASE = Easing.out(Easing.cubic);
const DOT = 8;
const DOT_ACTIVE = 22;

type Mode = 'first-run' | 'replay';

/**
 * Customer onboarding, three pages. `first-run` ends in the sign-in choices and
 * is remembered once finished or skipped; `replay` (from Settings) ends in "Done".
 */
export function OnboardingScreen({ mode }: { mode: Mode }) {
  const { t } = useTranslation('onboarding');
  const insets = useSafeAreaInsets();
  const { width } = useWindowDimensions();
  const reduceMotion = useReducedMotion();
  const markSeen = useOnboarding((s) => s.markSeen);

  const pages: OnboardingPageContent[] = [
    { icon: 'vehicle', badge: 'shield', title: t('p1_title'), body: t('p1_body') },
    { icon: 'measure', badge: 'check', title: t('p2_title'), body: t('p2_body') },
    { icon: 'warranty', badge: 'qr', title: t('p3_title'), body: t('p3_body') },
  ];
  const last = pages.length - 1;

  const ref = useAnimatedRef<Animated.ScrollView>();
  const scrollX = useSharedValue(0);
  const fade = useSharedValue(1);
  const progress = useDerivedValue(() => (width > 0 ? scrollX.value / width : 0));
  const current = useDerivedValue(() => Math.min(last, Math.max(0, Math.round(progress.value))));
  const [page, setPage] = useState(0);

  const onScroll = useAnimatedScrollHandler((e) => {
    scrollX.value = e.contentOffset.x;
  });
  useAnimatedReaction(
    () => current.value,
    (now, before) => {
      if (now !== before) scheduleOnRN(setPage, now);
    },
  );

  const pagerStyle = useAnimatedStyle(() => ({ opacity: fade.value }));

  const goTo = (target: number) => {
    const x = target * width;
    if (!reduceMotion) {
      ref.current?.scrollTo({ x, animated: true });
      return;
    }
    // Reduce Motion: no sliding, the pages cross-fade in place.
    fade.set(
      withTiming(0, { duration: 300, easing: EASE }, (finished) => {
        'worklet';
        if (!finished) return;
        scrollTo(ref, x, 0, false);
        fade.set(withTiming(1, { duration: 300, easing: EASE }));
      }),
    );
  };

  const signIn = (signInMode: 'phone' | 'email') => {
    markSeen();
    router.replace({ pathname: '/sign-in', params: { mode: signInMode } });
  };

  const onLast = page === last;

  return (
    // Replay is a page sheet that already sits below the status bar: no top inset there.
    <View
      className="flex-1 bg-background"
      style={{ paddingTop: mode === 'replay' ? 8 : insets.top, paddingBottom: Math.max(insets.bottom, 16) }}>
      <View className="h-11 flex-row items-center justify-end px-4">
        {mode === 'replay' ? (
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('common:close')}
            hitSlop={12}
            onPress={() => router.back()}
            className="size-11 items-center justify-center active:opacity-60">
            <Icon name="close" tone="muted-foreground" size={20} />
          </Pressable>
        ) : (
          <Pressable
            accessibilityRole="button"
            hitSlop={12}
            onPress={() => signIn('phone')}
            className="min-h-11 justify-center px-2 active:opacity-60">
            <Text variant="callout" tone="muted" className="font-semibold">
              {t('skip')}
            </Text>
          </Pressable>
        )}
      </View>

      <Animated.View style={pagerStyle} className="flex-1">
        <Animated.ScrollView
          ref={ref}
          horizontal
          pagingEnabled
          bounces={false}
          showsHorizontalScrollIndicator={false}
          scrollEventThrottle={16}
          onScroll={onScroll}
          // Paging math assumes offset 0 = first page; pinning LTR keeps that true
          // under Arabic, and each page restores RTL for its own content.
          style={{ direction: 'ltr' }}
          className="flex-1">
          {pages.map((p, i) => (
            <OnboardingPage
              key={p.icon}
              {...p}
              index={i}
              width={width}
              progress={progress}
              reduceMotion={reduceMotion}
            />
          ))}
        </Animated.ScrollView>
      </Animated.View>

      <View className="flex-row items-center justify-center gap-2 py-6" style={{ direction: 'ltr' }}>
        {pages.map((p, i) => (
          <Dot key={p.icon} index={i} current={current} reduceMotion={reduceMotion} />
        ))}
      </View>

      {/* Fixed height: switching to the last page swaps the actions without moving the dots. */}
      <View className="h-[116px] gap-2 px-6">
        <Animated.View key={onLast ? 'last' : 'next'} entering={FadeIn.duration(300).easing(EASE)} className="gap-2">
          {!onLast ? (
            <Button size="lg" title={t('next')} onPress={() => goTo(page + 1)} />
          ) : mode === 'replay' ? (
            <Button size="lg" title={t('done')} onPress={() => router.back()} />
          ) : (
            <>
              <Button size="lg" title={t('cta_phone')} onPress={() => signIn('phone')} />
              <Button variant="ghost" title={t('cta_staff')} onPress={() => signIn('email')} />
            </>
          )}
        </Animated.View>
      </View>
    </View>
  );
}

/** Page indicator: the active dot widens; under Reduce Motion it snaps and only the opacity fades. */
function Dot({ index, current, reduceMotion }: { index: number; current: SharedValue<number>; reduceMotion: boolean }) {
  const style = useAnimatedStyle(() => {
    const active = current.value === index;
    const width = active ? DOT_ACTIVE : DOT;
    return {
      width: reduceMotion ? width : withTiming(width, { duration: 300, easing: EASE }),
      opacity: withTiming(active ? 1 : 0.3, { duration: 300, easing: EASE }),
    };
  });
  return <Animated.View style={[{ height: DOT }, style]} className="rounded-full bg-foreground" />;
}
