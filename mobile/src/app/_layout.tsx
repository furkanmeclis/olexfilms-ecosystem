import '@/global.css';

import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client';
import { Stack, ThemeProvider } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { StatusBar } from 'expo-status-bar';
import { useEffect } from 'react';
import { I18nextProvider } from 'react-i18next';
import { I18nManager } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { LayoutDirection } from 'uniwind';

import { useAppStatus } from '@/core/app-status';
import { useSession } from '@/core/auth/session';
import { i18n } from '@/core/i18n';
import { applyThemePreference } from '@/core/preferences';
import { persistOptions, queryClient } from '@/core/query';
import { useNavigationTheme } from '@/features/shell/navigation-theme';

// Keep the splash only until the stored session is read (no network wait).
void SplashScreen.preventAutoHideAsync();
applyThemePreference();

export default function RootLayout() {
  const status = useSession((s) => s.status);
  const updateRequired = useAppStatus((s) => s.updateRequired);
  const theme = useNavigationTheme();

  useEffect(() => {
    void useSession.getState().hydrate();
  }, []);

  useEffect(() => {
    if (status !== 'loading') void SplashScreen.hideAsync();
  }, [status]);

  if (status === 'loading') return null;

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
            </ThemeProvider>
          </LayoutDirection>
        </PersistQueryClientProvider>
      </I18nextProvider>
    </GestureHandlerRootView>
  );
}
