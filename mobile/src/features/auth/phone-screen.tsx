import * as Haptics from 'expo-haptics';
import { router, useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Pressable, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { identify } from '@/core/auth/api';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { AuthStep } from './auth-layout';
import { CodeInput } from './code-input';
import { usePendingLogin } from './pending-login';
import { formatPhone } from './phone-country';
import { useLogin } from './use-login';

const LENGTH = 6;
/** How long the wrong digits stay red before the boxes clear. */
const CLEAR_AFTER_MS = 700;

function secondsUntil(iso?: string) {
  if (!iso) return 0;
  return Math.max(0, Math.ceil((new Date(iso).getTime() - Date.now()) / 1000));
}

function clock(seconds: number) {
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
}

/** Phone sign-in: the 6-digit code sent over WhatsApp (SMS fallback). */
export function PhoneScreen() {
  const { t } = useTranslation('auth');
  const params = useLocalSearchParams<{ phone: string; resendAt?: string }>();
  const phone = usePendingLogin((s) => s.phone) || params.phone || '';
  const [code, setCode] = useState('');
  const [error, setError] = useState<string>();
  const [errorKey, setErrorKey] = useState(0);
  const [resendAt, setResendAt] = useState(params.resendAt);
  const [wait, setWait] = useState(secondsUntil(params.resendAt));
  const [resending, setResending] = useState(false);
  const [resent, setResent] = useState(false);
  const login = useLogin();

  useEffect(() => {
    const id = setInterval(() => setWait(secondsUntil(resendAt)), 1000);
    return () => clearInterval(id);
  }, [resendAt]);

  useEffect(() => {
    if (!resent) return;
    const id = setTimeout(() => setResent(false), 4000);
    return () => clearTimeout(id);
  }, [resent]);

  const fail = (message: string) => {
    setError(message);
    setErrorKey((k) => k + 1);
    setTimeout(() => setCode(''), CLEAR_AFTER_MS);
  };

  const submit = (c: string) => {
    // Autofill can fire onChangeText twice; a second verify would spend the code again.
    if (c.length !== LENGTH || login.isPending) return;
    usePendingLogin.getState().set({ mode: 'phone', phone, code: c });
    login.mutate(undefined, {
      onError: (e) => {
        // An account with authenticator 2FA moves on to the next screen (useLogin).
        if (isApiError(e, 'MFA_REQUIRED')) return setCode('');
        fail(errorMessage(isApiError(e) ? e.code : undefined));
      },
    });
  };

  const resend = async () => {
    if (!phone || resending || wait > 0) return;
    setError(undefined);
    setResending(true);
    try {
      const res = await identify(phone);
      setResendAt(res.resend_at);
      setWait(secondsUntil(res.resend_at));
      setCode('');
      setResent(true);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    } catch (e) {
      fail(errorMessage(isApiError(e) ? e.code : undefined));
    } finally {
      setResending(false);
    }
  };

  return (
    <>
      <AuthStep title={t('phone_code_title')} subtitle={t('phone_code_subtitle')}>
        <View className="flex-row items-center gap-3 rounded-2xl border border-border bg-card p-3" style={{ borderCurve: 'continuous' }}>
          <View className="size-11 items-center justify-center rounded-full bg-primary/10">
            <Icon name="message" tone="primary" size={20} />
          </View>
          <View className="flex-1 gap-0.5">
            <Text variant="footnote" tone="muted">
              {t('phone_code_sent_to')}
            </Text>
            {/* Phone numbers never mirror in RTL. */}
            <Text variant="headline" className="tabular-nums" style={{ writingDirection: 'ltr', textAlign: 'left' }}>
              {formatPhone(phone)}
            </Text>
          </View>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('phone_change_a11y')}
            hitSlop={8}
            onPress={() => router.back()}
            className="rounded-full px-3 py-2 active:opacity-60">
            <Text variant="callout" tone="primary" className="font-semibold">
              {t('change')}
            </Text>
          </Pressable>
        </View>

        <View className="gap-4">
          <CodeInput
            value={code}
            onChange={(c) => {
              setCode(c);
              if (error) setError(undefined);
            }}
            onComplete={submit}
            length={LENGTH}
            error={!!error}
            errorKey={errorKey}
            disabled={login.isPending}
            accessibilityLabel={t('code_a11y', { length: LENGTH })}
          />
          <View className="min-h-6 flex-row items-center justify-center gap-2" accessibilityLiveRegion="polite">
            {login.isPending ? (
              <>
                <ActivityIndicator size="small" colorClassName="accent-muted-foreground" />
                <Text variant="footnote" tone="muted">
                  {t('code_verifying')}
                </Text>
              </>
            ) : error ? (
              <>
                <Icon name="alert" tone="destructive" size={14} weight="medium" />
                <Text variant="footnote" tone="danger" className="shrink text-center">
                  {error}
                </Text>
              </>
            ) : resent ? (
              <>
                <Icon name="check" tone="primary" size={14} weight="semibold" />
                <Text variant="footnote" tone="primary">
                  {t('phone_resent')}
                </Text>
              </>
            ) : null}
          </View>
        </View>

        <View className="items-center gap-1">
          <Text variant="footnote" tone="muted">
            {t('phone_no_code')}
          </Text>
          {wait > 0 ? (
            <View className="min-h-11 flex-row items-center gap-1.5">
              <Icon name="clock" tone="muted-foreground" size={14} />
              <Text variant="callout" tone="muted" className="tabular-nums">
                {t('phone_resend_wait', { time: clock(wait) })}
              </Text>
            </View>
          ) : (
            <Button variant="ghost" title={t('phone_resend')} loading={resending} onPress={() => void resend()} />
          )}
        </View>
      </AuthStep>
    </>
  );
}
