import * as Haptics from 'expo-haptics';
import { router } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Platform, Pressable, useWindowDimensions, View } from 'react-native';
import Animated, {
  Easing,
  Extrapolation,
  FadeIn,
  interpolate,
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

import { useOnboarding } from '@/core/onboarding';
import { BrandLogo } from '@/ui/brand-logo';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { FILM_BG, OnboardingPage, type OnboardingPageContent } from './onboarding-page';

const EASE = Easing.out(Easing.cubic);
/** The buttons area keeps one height on every page, so nothing jumps on the last one. */
const ACTIONS_HEIGHT = 116;

/** Stills from the brand films on olexfilms.com (cropped for portrait). */
const IMAGES = {
  crown: require('../../../assets/images/onboarding/01-crown.jpg'),
  shield: require('../../../assets/images/onboarding/02-shield.jpg'),
  gloss: require('../../../assets/images/onboarding/03-gloss.jpg'),
  color: require('../../../assets/images/onboarding/04-color.jpg'),
  dealer: require('../../../assets/images/onboarding/05-dealer.jpg'),
  warranty: require('../../../assets/images/onboarding/06-warranty.jpg'),
  app: require('../../../assets/images/onboarding/07-app.jpg'),
} as const;

type Mode = 'first-run' | 'replay';

/**
 * Customer onboarding: a short brand film of seven pages, each about what the
 * customer gets. `first-run` ends in the sign-in choices and is remembered once
 * finished or skipped; `replay` (from Settings) ends in "Done".
 */
export function OnboardingScreen({ mode }: { mode: Mode }) {
  const { t } = useTranslation('onboarding');
  const insets = useSafeAreaInsets();
  const { width, height } = useWindowDimensions();
  const reduceMotion = useReducedMotion();
  const markSeen = useOnboarding((s) => s.markSeen);

  const pages: OnboardingPageContent[] = [
    {
      key: 'crown',
      image: IMAGES.crown,
      eyebrow: t('film_p1_eyebrow'),
      title: t('film_p1_title'),
      body: t('film_p1_body'),
      stats: [
        { value: '30+', label: t('film_stat_years') },
        { value: '10+', label: t('film_stat_countries') },
        { value: '50+', label: t('film_stat_products') },
      ],
      reserve: 430,
    },
    { key: 'shield', image: IMAGES.shield, eyebrow: t('film_p2_eyebrow'), title: t('film_p2_title'), body: t('film_p2_body') },
    { key: 'gloss', image: IMAGES.gloss, eyebrow: t('film_p3_eyebrow'), title: t('film_p3_title'), body: t('film_p3_body') },
    { key: 'color', image: IMAGES.color, eyebrow: t('film_p4_eyebrow'), title: t('film_p4_title'), body: t('film_p4_body') },
    { key: 'dealer', image: IMAGES.dealer, eyebrow: t('film_p5_eyebrow'), title: t('film_p5_title'), body: t('film_p5_body') },
    {
      key: 'warranty',
      image: IMAGES.warranty,
      eyebrow: t('film_p6_eyebrow'),
      title: t('film_p6_title'),
      body: t('film_p6_body'),
      overlay: 'warranty',
    },
    {
      key: 'app',
      image: IMAGES.app,
      eyebrow: t('film_p7_eyebrow'),
      title: t('film_p7_title'),
      features: [
        { icon: 'warranty', label: t('film_f_warranty') },
        { icon: 'calendar', label: t('film_f_appointment') },
        { icon: 'service', label: t('film_f_history') },
        { icon: 'transfer', label: t('film_f_transfer') },
      ],
      reserve: 520,
    },
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
  const onPageChange = (now: number) => {
    setPage(now);
    void Haptics.selectionAsync();
  };
  useAnimatedReaction(
    () => current.value,
    (now, before) => {
      if (now !== before && before !== null) scheduleOnRN(onPageChange, now);
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
  // Replay on iOS is a page sheet that already sits below the status bar.
  const top = mode === 'replay' && Platform.OS === 'ios' ? 12 : insets.top + 8;

  return (
    <View className="flex-1" style={{ backgroundColor: FILM_BG }}>
      <StatusBar style="light" />
      <Animated.View style={pagerStyle} className="flex-1">
        <Animated.ScrollView
          ref={ref}
          horizontal
          pagingEnabled
          bounces={false}
          showsHorizontalScrollIndicator={false}
          scrollEventThrottle={16}
          onScroll={onScroll}
          // Paging math assumes offset 0 = first page; pinning LTR keeps that true under Arabic.
          style={{ direction: 'ltr' }}
          className="flex-1">
          {pages.map((p, i) => (
            <OnboardingPage {...p} key={p.key} index={i} width={width} height={height} progress={progress} reduceMotion={reduceMotion} />
          ))}
        </Animated.ScrollView>
      </Animated.View>

      {/* Story bars, logo and skip float over the photo. */}
      <View pointerEvents="box-none" className="absolute inset-x-0 gap-3 px-5" style={{ top }}>
        <View className="flex-row gap-1.5" style={{ direction: 'ltr' }}>
          {pages.map((p, i) => (
            <Segment key={p.key} index={i} progress={progress} />
          ))}
        </View>
        <View pointerEvents="box-none" className="h-11 flex-row items-center justify-between">
          <BrandLogo variant="logo" tone="dark" height={26} label={t('common:app_name')} />
          {mode === 'replay' ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('common:close')}
              hitSlop={12}
              onPress={() => router.back()}
              className="size-11 items-center justify-center rounded-full bg-on-media/15 active:opacity-60">
              <Icon name="close" tone="on-media" size={18} />
            </Pressable>
          ) : onLast ? null : (
            <Pressable
              accessibilityRole="button"
              hitSlop={12}
              onPress={() => signIn('phone')}
              className="min-h-9 justify-center rounded-full bg-on-media/15 px-4 active:opacity-60">
              <Text variant="footnote" className="font-semibold text-on-media">
                {t('skip')}
              </Text>
            </Pressable>
          )}
        </View>
      </View>

      <View className="absolute inset-x-0 bottom-0 px-6" style={{ paddingBottom: Math.max(insets.bottom, 16) + 8 }}>
        <View style={{ height: ACTIONS_HEIGHT }} className="justify-end">
          <Animated.View key={onLast ? 'last' : 'next'} entering={FadeIn.duration(300).easing(EASE)} className="gap-2">
            {!onLast ? (
              <FilmButton title={t('next')} onPress={() => goTo(page + 1)} />
            ) : mode === 'replay' ? (
              <FilmButton title={t('done')} onPress={() => router.back()} />
            ) : (
              <>
                <FilmButton title={t('cta_phone')} onPress={() => signIn('phone')} />
                <Pressable
                  accessibilityRole="button"
                  onPress={() => signIn('email')}
                  className="min-h-11 items-center justify-center active:opacity-60">
                  <Text variant="callout" className="font-semibold text-on-media/75">
                    {t('cta_staff')}
                  </Text>
                </Pressable>
              </>
            )}
          </Animated.View>
        </View>
      </View>
    </View>
  );
}

/** White pill on the dark film; the app's primary green would sink into the black. */
function FilmButton({ title, onPress }: { title: string; onPress: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      onPressIn={() => void Haptics.selectionAsync()}
      onPress={onPress}
      className="min-h-14 flex-row items-center justify-center gap-2 rounded-full bg-on-media px-6 active:opacity-85">
      <Text className="text-[17px] font-semibold" style={{ color: FILM_BG }}>
        {title}
      </Text>
    </Pressable>
  );
}

/** One story bar: fills as its page scrolls in, stays full once passed. */
function Segment({ index, progress }: { index: number; progress: SharedValue<number> }) {
  const fill = useAnimatedStyle(() => ({
    width: `${interpolate(progress.value, [index - 1, index], [0, 100], Extrapolation.CLAMP)}%`,
  }));
  return (
    <View className="h-[3px] flex-1 overflow-hidden rounded-full bg-on-media/25">
      <Animated.View style={fill} className="h-full rounded-full bg-on-media" />
    </View>
  );
}
