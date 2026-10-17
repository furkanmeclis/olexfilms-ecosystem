import { useQuery } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import { kindsOf } from '@/core/spaces';
import { LARGE_TITLES } from '@/features/shell/large-titles';

/**
 * Signed-in shell. The cached session renders first; /me refreshes it in the
 * background (on launch and when the app returns). Each space is only
 * reachable while the session allows it.
 */
export default function AppLayout() {
  const { t } = useTranslation();
  const kinds = kindsOf(useSession((s) => s.me));

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
        options={{ headerShown: true, presentation: 'formSheet', sheetAllowedDetents: [0.55, 1], sheetGrabberVisible: true }}
      />
      <Stack.Screen name="search" options={{ headerShown: true, presentation: 'modal' }} />
      <Stack.Screen name="settings" options={{ headerShown: true, headerLargeTitle: LARGE_TITLES }} />
      <Stack.Screen name="soon/[key]" options={{ headerShown: true }} />
      <Stack.Screen
        name="qr-login/[code]"
        options={{ headerShown: true, presentation: 'formSheet', sheetAllowedDetents: [0.6], sheetGrabberVisible: true }}
      />
    </Stack>
  );
}
