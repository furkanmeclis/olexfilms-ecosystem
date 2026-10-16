import { Stack } from 'expo-router';
import { Platform } from 'react-native';

/** Stack inside every tab: native large titles that collapse on scroll. */
export function TabStack() {
  return (
    <Stack
      screenOptions={{
        headerLargeTitle: true,
        headerTransparent: Platform.OS === 'ios',
        headerBlurEffect: 'systemChromeMaterial',
        headerShadowVisible: false,
        headerLargeTitleShadowVisible: false,
      }}
    />
  );
}
