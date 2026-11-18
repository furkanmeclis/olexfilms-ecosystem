import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { AccessibilityInfo, ActivityIndicator, Platform, Pressable, StyleSheet, View } from 'react-native';
import { GestureDetector, GestureHandlerRootView, usePanGesture } from 'react-native-gesture-handler';
import Animated, {
  FadeIn,
  FadeOut,
  FadeOutUp,
  LinearTransition,
  SlideInUp,
  useAnimatedStyle,
  useReducedMotion,
  useSharedValue,
  withSpring,
  withTiming,
} from 'react-native-reanimated';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { FullWindowOverlay } from 'react-native-screens';
import { scheduleOnRN } from 'react-native-worklets';
import { useCSSVariable } from 'uniwind';
import { useStore } from 'zustand';

import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { Text } from '@/ui/text';

import type { ToastItem, ToastKind } from './store';
import { toast, toastStore } from './toaster';

/** Tür → ikon ve marka token'ı (web'in rozet renkleriyle aynı: global.css). */
const KIND: Record<ToastKind, { icon?: IconName; token: string; accent: string }> = {
  success: { icon: 'success', token: '--color-success', accent: 'bg-success' },
  error: { icon: 'error', token: '--color-destructive', accent: 'bg-destructive' },
  warning: { icon: 'alert', token: '--color-warning', accent: 'bg-warning' },
  info: { icon: 'info', token: '--color-primary', accent: 'bg-primary' },
  loading: { token: '--color-primary', accent: 'bg-primary' },
};

/** Kaydırma eşikleri: bu kadar yana / yukarı ya da hızlı fırlatma kapatır. */
const SWIPE_X = 80;
const SWIPE_UP = 32;
const FLING = 700;

/**
 * Uygulamanın toast katmanı (TEC-1235). Kök layout'ta bir kez durur; ekranın
 * üstünde, güvenli alanın altında görünür (klavye altta açıldığından üstü
 * kapatmaz). iOS'ta FullWindowOverlay ile native modal / sheet'lerin de üstünde.
 *
 * Android'de RN `<Modal>` ayrı bir pencere açar ve kök katmanı örter; böyle
 * bir modalın içinde `<ToastHost inModal />` kullanılır (yalnızca Android'de
 * çizilir, iOS'ta kök katman zaten üstte).
 */
export function ToastHost({ inModal = false }: { inModal?: boolean }) {
  const toasts = useStore(toastStore, (s) => s.toasts);
  if (inModal && Platform.OS !== 'android') return null;
  if (toasts.length === 0) return null;
  const stack = <ToastStack toasts={toasts} />;
  if (Platform.OS === 'ios' && !inModal) {
    return (
      <FullWindowOverlay>
        <GestureHandlerRootView style={StyleSheet.absoluteFill} pointerEvents="box-none">
          {stack}
        </GestureHandlerRootView>
      </FullWindowOverlay>
    );
  }
  return stack;
}

function ToastStack({ toasts }: { toasts: ToastItem[] }) {
  const insets = useSafeAreaInsets();
  return (
    <View
      pointerEvents="box-none"
      className="absolute inset-x-0 top-0 items-center gap-2 px-3"
      style={{ paddingTop: insets.top + 8, zIndex: 1000, elevation: 1000 }}>
      {toasts.map((item) => (
        <ToastCard key={item.id} item={item} />
      ))}
    </View>
  );
}

function ToastCard({ item }: { item: ToastItem }) {
  const { t } = useTranslation('common');
  const reduceMotion = useReducedMotion();
  const kind = KIND[item.kind];
  const tint = useCSSVariable(kind.token);
  const color = typeof tint === 'string' ? tint : undefined;

  const label = [t(`toast.${item.kind}`), item.message, item.description].filter(Boolean).join('. ');

  // iOS'ta canlı bölge yok: her gösterim / yenilemede metni duyur.
  useEffect(() => {
    if (Platform.OS === 'ios') AccessibilityInfo.announceForAccessibility(label);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.version]);

  const x = useSharedValue(0);
  const y = useSharedValue(0);
  const close = () => toast.dismiss(item.id);

  const pan = usePanGesture({
    activeOffsetX: [-10, 10],
    activeOffsetY: [-10, 10],
    onUpdate: (e) => {
      'worklet';
      x.value = e.translationX;
      // Yalnızca yukarı: aşağı çekmek kartı yerinde bırakır.
      y.value = Math.min(0, e.translationY);
    },
    onDeactivate: (e) => {
      'worklet';
      const side = Math.abs(e.translationX) > SWIPE_X || Math.abs(e.velocityX) > FLING;
      const up = e.translationY < -SWIPE_UP || e.velocityY < -FLING;
      if (side || up) {
        if (!reduceMotion) {
          if (side) x.value = withTiming(Math.sign(e.translationX || e.velocityX) * 500, { duration: 160 });
          else y.value = withTiming(-200, { duration: 160 });
        }
        scheduleOnRN(close);
        return;
      }
      x.value = reduceMotion ? 0 : withSpring(0);
      y.value = reduceMotion ? 0 : withSpring(0);
    },
  });

  const drag = useAnimatedStyle(() => ({
    transform: [{ translateX: x.value }, { translateY: y.value }],
    opacity: 1 - Math.min(0.6, Math.abs(x.value) / 300),
  }));

  return (
    <GestureDetector gesture={pan}>
      <Animated.View
        entering={reduceMotion ? FadeIn.duration(120) : SlideInUp.springify().damping(18)}
        exiting={reduceMotion ? FadeOut.duration(120) : FadeOutUp.duration(180)}
        layout={reduceMotion ? undefined : LinearTransition.springify().damping(18)}
        style={[drag, styles.card]}
        className="w-full max-w-[560px] flex-row items-center overflow-hidden rounded-xl border border-border bg-card">
        <View className={cn('w-1 self-stretch', kind.accent)} />
        <Pressable
          onPress={close}
          accessibilityRole="alert"
          accessibilityLiveRegion={item.kind === 'error' ? 'assertive' : 'polite'}
          accessibilityLabel={label}
          accessibilityHint={t('toast.dismiss_hint')}
          className="flex-1 flex-row items-start gap-3 py-3 ps-3 pe-1">
          <View className="pt-px">
            {kind.icon ? (
              <Icon name={kind.icon} color={color} size={20} weight="medium" />
            ) : (
              <ActivityIndicator size="small" color={color} />
            )}
          </View>
          <View className="flex-1 gap-0.5">
            <Text variant="callout" className="font-semibold">
              {item.message}
            </Text>
            {item.description ? (
              <Text variant="footnote" tone="muted">
                {item.description}
              </Text>
            ) : null}
          </View>
        </Pressable>
        {item.action ? (
          <Pressable
            onPress={() => {
              item.action?.onPress();
              close();
            }}
            accessibilityRole="button"
            accessibilityLabel={item.action.label}
            hitSlop={8}
            className="rounded-md px-2 py-1.5 active:opacity-70">
            <Text variant="callout" tone="primary" className="font-semibold">
              {item.action.label}
            </Text>
          </Pressable>
        ) : null}
        {item.kind === 'loading' ? null : (
          <Pressable
            onPress={close}
            accessibilityRole="button"
            accessibilityLabel={t('close')}
            hitSlop={8}
            className="me-2 ms-1 rounded-md p-1.5 active:opacity-70">
            <Icon name="close" tone="muted-foreground" size={16} />
          </Pressable>
        )}
      </Animated.View>
    </GestureDetector>
  );
}

const styles = StyleSheet.create({
  card: { borderCurve: 'continuous', boxShadow: '0 6px 20px rgba(0, 0, 0, 0.14)' },
});
