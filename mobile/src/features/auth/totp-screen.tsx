import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';

import { useLogin } from './use-login';

export function TotpScreen() {
  const { t } = useTranslation('auth');
  const [code, setCode] = useState('');
  const login = useLogin();
  const submit = (c = code) => c.length === 6 && !login.isPending && login.mutate(c);

  return (
    <>
      <Stack.Screen options={{ title: t('totp_title') }} />
      <Screen className="gap-5 pt-4">
        <Input
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
          className="text-center text-[24px] tracking-[8px]"
          error={login.error ? errorMessage(isApiError(login.error) ? login.error.code : undefined) : undefined}
        />
        <Button title={t('sign_in')} size="lg" loading={login.isPending} disabled={code.length !== 6} onPress={() => submit()} />
      </Screen>
    </>
  );
}
