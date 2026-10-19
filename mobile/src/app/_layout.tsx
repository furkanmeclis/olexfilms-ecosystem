import '@/global.css';

import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client';
import { useFonts } from 'expo-font';
import { Stack, ThemeProvider } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { StatusBar } from 'expo-status-bar';
import { useEffect } from 'react';
import { I18nextProvider } from 'react-i18next';
import { I18nManager, LogBox } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { LayoutDirection } from 'uniwind';

import { useAppStatus } from '@/core/app-status';
import { useSession } from '@/core/auth/session';
import { i18n } from '@/core/i18n';
import { applyThemePreference } from '@/core/preferences';
import { persistOptions, queryClient } from '@/core/query';
import { startUpdateChecks } from '@/core/updates';
import { UpdateBanner } from '@/features/shell/update-banner';
import { useNavigationTheme } from '@/features/shell/navigation-theme';
import { fontAssets } from '@/ui/fonts';

// Known React Native Animated noise (native driver events after unmount); harmless.
LogBox.ignoreLogs(['Sending `onAnimatedValueUpdate` with no listeners registered']);

// Keep the splash only until the stored session is read and the bundled
// fonts are registered (both local, no network wait).
void SplashScreen.preventAutoHideAsync();
applyThemePreference();

export default function RootLayout() {
  const status = useSession((s) => s.status);
  const updateRequired = useAppStatus((s) => s.updateRequired);
  // A font that fails to load falls back to the system font, never blocks launch.
  const [fontsLoaded, fontError] = useFonts(fontAssets);
  const fontsReady = fontsLoaded || !!fontError;
  const theme = useNavigationTheme(fontsLoaded);

  useEffect(() => {
    void useSession.getState().hydrate();
  }, []);

  // OTA: check on launch and each foreground (throttled); no-op in dev builds and Expo Go.
  useEffect(() => startUpdateChecks(), []);

  useEffect(() => {
    if (status !== 'loading' && fontsReady) void SplashScreen.hideAsync();
  }, [status, fontsReady]);

  if (status === 'loading' || !fontsReady) return null;

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <I18nextProvider i18n={i18n}>
        <PersistQueryClientProvider client={queryClient} persistOptions={persistOptions}>
          <LayoutDirection rtl={I18nManager.isRTL}>
            <ThemeProvider value={theme}>
              <StatusBar style="auto" />
              <Stack screenOptions={{ headerShown: false }}>
                <Stack.Protected guard={!!updateRequired}>
                  <Stack.Screen name="update-required" />
                </Stack.Protected>
                <Stack.Protected guard={!updateRequired && status === 'signedIn'}>
                  <Stack.Screen name="(app)" />
                </Stack.Protected>
                <Stack.Protected guard={!updateRequired && status === 'signedOut'}>
                  <Stack.Screen name="(public)" />
                </Stack.Protected>
              </Stack>
              {!updateRequired && <UpdateBanner />}
            </ThemeProvider>
          </LayoutDirection>
        </PersistQueryClientProvider>
      </I18nextProvider>
    </GestureHandlerRootView>
  );
}
