import { router } from 'expo-router';
import { View } from 'react-native';

import { Button } from '@/ui/button';
import { Text } from '@/ui/text';

import { usePendingLogin } from './pending-login';

/**
 * Development-only shortcut to the seed-demo accounts (`*@demo.olexfilms.app`).
 * Shown only in dev builds when EXPO_PUBLIC_DEMO_PASSWORD is set in the
 * gitignored `.env.local`, so no credential lives in source. It fills the
 * pending login and opens the password step; signing in is still a tap there.
 */
const DEMO_PASSWORD = process.env.EXPO_PUBLIC_DEMO_PASSWORD;
const DEMO_ACCOUNTS = ['dealer_owner', 'center_staff', 'center_warehouse', 'distributor_owner'] as const;

export function DevDemoAccounts() {
  if (!__DEV__ || !DEMO_PASSWORD) return null;
  return (
    <View className="gap-2">
      <Text variant="footnote" tone="muted">
        Demo (dev)
      </Text>
      <View className="flex-row flex-wrap gap-2">
        {DEMO_ACCOUNTS.map((name) => (
          <Button
            key={name}
            title={name}
            size="md"
            variant="secondary"
            onPress={() => {
              usePendingLogin.getState().set({ mode: 'password', email: `${name}@demo.olexfilms.app`, password: DEMO_PASSWORD });
              router.push('/password');
            }}
          />
        ))}
      </View>
    </View>
  );
}
