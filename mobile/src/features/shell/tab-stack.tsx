import { Stack } from 'expo-router';
import { Platform } from 'react-native';

import { LARGE_TITLES } from './large-titles';

/** Stack inside every tab: native large titles that collapse on scroll. */
export function TabStack() {
  return (
    <Stack
      screenOptions={{
        headerLargeTitle: LARGE_TITLES,
        headerTransparent: Platform.OS === 'ios',
        headerShadowVisible: false,
        headerLargeTitleShadowVisible: false,
      }}
    />
  );
}
