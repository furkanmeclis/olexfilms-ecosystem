import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { GlassCard } from '@/ui/glass-card';
import { Input } from '@/ui/input';

import { AuthLayout } from './auth-layout';
import { useLogin } from './use-login';

export function TotpScreen() {
  const { t } = useTranslation('auth');
  const [code, setCode] = useState('');
  const login = useLogin();
  const submit = (c = code) => c.length === 6 && !login.isPending && login.mutate(c);

  return (
    <>
      {/* The large title below replaces the header title; the header keeps the back button. */}
      <Stack.Screen options={{ title: t('totp_title'), headerTitle: '' }} />
      <AuthLayout title={t('totp_title')}>
        <GlassCard className="gap-5">
          <Input
            icon="shield"
            label={t('totp_label')}
            placeholder={t('placeholder_code')}
            hint={t('totp_hint')}
            value={code}
            onChangeText={(v) => {
              const digits = v.replace(/\D/g, '').slice(0, 6);
              setCode(digits);
              // Submits by itself on the sixth digit (also when iOS autofills the code).
              if (digits.length === 6) submit(digits);
            }}
            keyboardType="number-pad"
            textContentType="oneTimeCode"
            autoComplete="one-time-code"
            autoFocus
            className="bg-background text-center text-[24px] tracking-[8px]"
            error={login.error ? errorMessage(isApiError(login.error) ? login.error.code : undefined) : undefined}
          />
          <Button title={t('sign_in')} size="lg" loading={login.isPending} disabled={code.length !== 6} onPress={() => submit()} />
        </GlassCard>
      </AuthLayout>
    </>
  );
}
