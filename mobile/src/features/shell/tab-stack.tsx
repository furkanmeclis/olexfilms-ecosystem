import { Stack } from 'expo-router';
import type { ReactNode } from 'react';
import { Platform } from 'react-native';

import { LARGE_TITLES } from './large-titles';
import { ModuleGate } from './module-gate';

/**
 * Stack inside every tab: native large titles that collapse on scroll.
 * `children` set per-screen options up front (e.g. a headerless home), so the
 * native header never mounts and reserves no space. `module` gates every screen
 * of a stack that belongs to one module (the customer's appointments tab).
 */
export function TabStack({ children, module }: { children?: ReactNode; module?: string } = {}) {
  return (
    <Stack
      // A screen of a switched-off module never opens (TEC-1023 / module flags).
      screenLayout={({ route, children: screen }) => (
        <ModuleGate routeName={route.name} module={module}>
          {screen}
        </ModuleGate>
      )}
      screenOptions={{
        headerLargeTitle: LARGE_TITLES,
        headerTransparent: Platform.OS === 'ios',
        headerShadowVisible: false,
        headerLargeTitleShadowVisible: false,
      }}>
      {children}
    </Stack>
  );
}
