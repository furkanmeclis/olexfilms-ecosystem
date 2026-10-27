import { useQuery } from '@tanstack/react-query';
import { Stack, usePathname } from 'expo-router';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import { rememberPath } from '@/core/space-memory';
import { kindsOf } from '@/core/spaces';
import { LARGE_TITLES } from '@/features/shell/large-titles';

/**
 * Signed-in shell. The cached session renders first; /me refreshes it in the
 * background (on launch and when the app returns). Each space is only
 * reachable while the session allows it.
 */
export default function AppLayout() {
  const { t } = useTranslation();
  const me = useSession((s) => s.me);
  const kinds = kindsOf(me);
  const pathname = usePathname();
  // Reopen where the user left off (TEC-1027).
  useEffect(() => rememberPath(me, pathname), [me, pathname]);

  useQuery({
    queryKey: ['me'],
    queryFn: () => useSession.getState().refreshMe(),
    staleTime: 60_000,
    refetchOnWindowFocus: true,
  });

  return (
    <Stack screenOptions={{ headerShown: false, headerBackTitle: t('common:back') }}>
      <Stack.Screen name="index" />
      <Stack.Protected guard={kinds.includes('bayi')}>
        <Stack.Screen name="bayi" />
      </Stack.Protected>
      <Stack.Protected guard={kinds.includes('depo')}>
        <Stack.Screen name="depo" />
      </Stack.Protected>
      <Stack.Protected guard={kinds.includes('admin')}>
        <Stack.Screen name="admin" />
      </Stack.Protected>
      <Stack.Protected guard={kinds.includes('musteri')}>
        <Stack.Screen name="musteri" />
      </Stack.Protected>

      <Stack.Screen
        name="spaces"
        // Sized to its rows (two spaces, maybe a few organizations), no empty half sheet.
        options={{ headerShown: true, presentation: 'formSheet', sheetAllowedDetents: 'fitToContents', sheetGrabberVisible: true }}
      />
      <Stack.Screen name="search" options={{ headerShown: true, presentation: 'modal' }} />
      <Stack.Screen name="intro" options={{ presentation: 'modal' }} />
      <Stack.Screen name="notifications" options={{ headerShown: true, headerLargeTitle: LARGE_TITLES }} />
      <Stack.Screen name="settings" options={{ headerShown: true, headerLargeTitle: LARGE_TITLES }} />
      <Stack.Screen name="soon/[key]" options={{ headerShown: true }} />
      <Stack.Screen
        name="qr-login/[code]"
        options={{ headerShown: true, presentation: 'formSheet', sheetAllowedDetents: [0.6], sheetGrabberVisible: true }}
      />
    </Stack>
  );
}
