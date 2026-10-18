import { GlassView, isGlassEffectAPIAvailable, isLiquidGlassAvailable } from 'expo-glass-effect';
import type { ReactNode } from 'react';
import { Platform, View } from 'react-native';
import { useUniwind } from 'uniwind';

import { cn } from './cn';

let glass: boolean | undefined;
/** Liquid Glass only where the API exists (some iOS 26 betas crash without the runtime check). */
export function glassAvailable() {
  glass ??= Platform.OS === 'ios' && isGlassEffectAPIAvailable() && isLiquidGlassAvailable();
  return glass;
}

/** Rounded surface: Liquid Glass on iOS 26+, a flat bordered card everywhere else. */
export function GlassCard({ children, className }: { children: ReactNode; className?: string }) {
  const { theme } = useUniwind();
  const shape = { borderRadius: 28, borderCurve: 'continuous' as const };
  if (glassAvailable()) {
    return (
      <GlassView
        glassEffectStyle="regular"
        colorScheme={theme === 'dark' ? 'dark' : 'light'}
        style={shape}>
        <View className={cn('p-5', className)}>{children}</View>
      </GlassView>
    );
  }
  return (
    <View className={cn('border border-border bg-card p-5', className)} style={shape}>
      {children}
    </View>
  );
}
