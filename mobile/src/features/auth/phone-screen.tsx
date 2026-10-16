import { router, Stack, useLocalSearchParams } from 'expo-router';
import { Smartphone } from '@/ui/icons';
import { useTranslation } from 'react-i18next';

import { EmptyState } from '@/ui/empty-state';
import { Screen } from '@/ui/screen';

/**
 * Phone sign-in (WhatsApp code). The mobile API has no phone/OTP sign-in yet
 * (only /v1/mobile/auth/login with e-mail + password); see mobile/README.md
 * "Backend gaps". Until then this screen sends the user back to e-mail.
 */
export function PhoneScreen() {
  const { t } = useTranslation('auth');
  const { phone } = useLocalSearchParams<{ phone: string }>();
  return (
    <>
      <Stack.Screen options={{ title: t('phone_title') }} />
      <Screen>
        <EmptyState
          icon={Smartphone}
          title={phone ?? ''}
          body={t('phone_unavailable')}
          action={{ title: t('change'), onPress: () => router.back() }}
        />
      </Screen>
    </>
  );
}
