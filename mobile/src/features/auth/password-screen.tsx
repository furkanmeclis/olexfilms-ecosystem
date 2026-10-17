import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { usePendingLogin } from './pending-login';
import { useLogin } from './use-login';

export function PasswordScreen() {
  const { t } = useTranslation('auth');
  const email = usePendingLogin((s) => s.email);
  const password = usePendingLogin((s) => s.password);
  const login = useLogin();
  const error = login.error && !isApiError(login.error, 'MFA_REQUIRED') ? login.error : null;

  return (
    <>
      <Stack.Screen options={{ title: t('password_title') }} />
      <Screen className="gap-5 pt-4">
        <View className="rounded-lg bg-secondary px-4 py-3">
          <Text variant="callout">{email}</Text>
        </View>
        <Input
          label={t('password_label')}
          value={password}
          onChangeText={(v) => usePendingLogin.getState().set({ password: v })}
          secureTextEntry
          autoFocus
          autoComplete="current-password"
          textContentType="password"
          returnKeyType="go"
          onSubmitEditing={() => login.mutate(undefined)}
          error={error ? errorMessage(isApiError(error) ? error.code : undefined) : undefined}
        />
        <Button
          title={t('sign_in')}
          size="lg"
          loading={login.isPending}
          disabled={!password}
          onPress={() => login.mutate(undefined)}
        />
        <Text variant="footnote" tone="muted" className="text-center">
          {t('forgot_password')}
        </Text>
      </Screen>
    </>
  );
}
