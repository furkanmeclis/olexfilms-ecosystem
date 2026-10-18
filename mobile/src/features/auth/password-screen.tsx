import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { GlassCard } from '@/ui/glass-card';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { AuthLayout } from './auth-layout';
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
      {/* The large title below replaces the header title; the header keeps the back button. */}
      <Stack.Screen options={{ title: t('password_title'), headerTitle: '' }} />
      <AuthLayout title={t('password_title')}>
        <GlassCard className="gap-5">
          <View className="flex-row items-center gap-3 rounded-lg bg-secondary px-4 py-3">
            <Icon name="mail" tone="muted-foreground" size={17} />
            <Text variant="callout" className="flex-1" numberOfLines={1}>
              {email}
            </Text>
          </View>
          <Input
            icon="password"
            label={t('password_label')}
            placeholder={t('placeholder_password')}
            value={password}
            onChangeText={(v) => usePendingLogin.getState().set({ password: v })}
            className="bg-background"
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
        </GlassCard>
        <Text variant="footnote" tone="muted" className="text-center">
          {t('forgot_password')}
        </Text>
      </AuthLayout>
    </>
  );
}
