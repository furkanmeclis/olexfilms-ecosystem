import { Stack, useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { identify } from '@/core/auth/api';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { usePendingLogin } from './pending-login';
import { useLogin } from './use-login';

function secondsUntil(iso?: string) {
  if (!iso) return 0;
  return Math.max(0, Math.ceil((new Date(iso).getTime() - Date.now()) / 1000));
}

/** Phone sign-in: the 6-digit code sent over WhatsApp (SMS fallback). */
export function PhoneScreen() {
  const { t } = useTranslation('auth');
  const params = useLocalSearchParams<{ phone: string; resendAt?: string }>();
  const phone = usePendingLogin((s) => s.phone) || params.phone || '';
  const [code, setCode] = useState('');
  const [resendAt, setResendAt] = useState(params.resendAt);
  const [wait, setWait] = useState(secondsUntil(params.resendAt));
  const [resendError, setResendError] = useState<string>();
  const login = useLogin();

  useEffect(() => {
    const id = setInterval(() => setWait(secondsUntil(resendAt)), 1000);
    return () => clearInterval(id);
  }, [resendAt]);

  const submit = (c = code) => {
    // Autofill can fire onChangeText twice; a second verify would spend the code again.
    if (c.length !== 6 || login.isPending) return;
    usePendingLogin.getState().set({ mode: 'phone', phone, code: c });
    login.mutate(undefined);
  };

  const resend = async () => {
    setResendError(undefined);
    try {
      const res = await identify(phone);
      setResendAt(res.resend_at);
      setWait(secondsUntil(res.resend_at));
    } catch (e) {
      setResendError(errorMessage(isApiError(e) ? e.code : undefined));
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: t('phone_title') }} />
      <Screen className="gap-5 pt-4">
        <Text variant="callout" tone="muted">
          {t('phone_code_sent', { phone })}
        </Text>
        <Input
          icon="code"
          label={t('phone_code_label')}
          placeholder={t('placeholder_code')}
          value={code}
          onChangeText={(v) => {
            const digits = v.replace(/\D/g, '').slice(0, 6);
            setCode(digits);
            if (digits.length === 6) submit(digits);
          }}
          keyboardType="number-pad"
          textContentType="oneTimeCode"
          autoComplete="one-time-code"
          autoFocus
          className="text-center text-[24px] tracking-[8px]"
          error={login.error && !isApiError(login.error, 'MFA_REQUIRED') ? errorMessage(isApiError(login.error) ? login.error.code : undefined) : resendError}
        />
        <Button title={t('sign_in')} size="lg" loading={login.isPending} disabled={code.length !== 6} onPress={() => submit()} />
        <View className="items-center">
          <Button
            variant="ghost"
            title={wait > 0 ? t('phone_resend_in', { seconds: wait }) : t('phone_resend')}
            disabled={wait > 0}
            onPress={() => void resend()}
          />
        </View>
      </Screen>
    </>
  );
}
