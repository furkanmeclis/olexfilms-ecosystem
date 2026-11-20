import { useMutation } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { CodeInput } from '@/features/auth/code-input';
import { Button } from '@/ui/button';
import { confirmAction } from '@/ui/confirm';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { deleteErrorKey, keepsCodeStep } from './lib';

const CODE_LENGTH = 6;

/**
 * Profile > Delete my account (TEC-1257; App Store 5.1.1(v), KVKK). A code
 * goes to the account's phone (POST /v1/portal/me/delete/code); the code
 * confirms the anonymization (POST /v1/portal/me/delete). The server ends
 * every session, so the app signs out locally.
 */
export function DeleteAccountScreen() {
  const { t, i18n } = useTranslation('legal');
  const [sent, setSent] = useState(false);
  const [code, setCode] = useState('');
  const [error, setError] = useState<string>();
  const [errorKey, setErrorKey] = useState(0);

  const fail = (e: unknown) => {
    const apiCode = isApiError(e) ? e.code : undefined;
    setError(t(deleteErrorKey(apiCode)));
    setErrorKey((k) => k + 1);
    if (!keepsCodeStep(apiCode)) setCode('');
  };

  const request = useMutation({
    mutationFn: () => unwrap(api.POST('/v1/portal/me/delete/code', { body: { locale: i18n.language } })),
    onSuccess: () => {
      setSent(true);
      setCode('');
      setError(undefined);
    },
    onError: fail,
  });

  const remove = useMutation({
    mutationFn: (value: string) => unwrap(api.POST('/v1/portal/me/delete', { body: { code: value } })),
    onSuccess: () => {
      toast.success(t('done'));
      // The server already revoked the tokens; a remote logout would only fail.
      void useSession.getState().signOut({ remote: false });
    },
    onError: fail,
  });

  const confirm = (value: string) => {
    if (value.length !== CODE_LENGTH || remove.isPending) return;
    confirmAction({
      title: t('confirm_title'),
      body: t('confirm_body'),
      action: t('confirm'),
      destructive: true,
      onConfirm: () => remove.mutate(value),
    });
  };

  return (
    <>
      <Stack.Screen options={{ title: t('delete_title'), headerLargeTitle: false }} />
      <Screen>
        <Text variant="body">{t('delete_warning')}</Text>
        {error ? (
          <Text variant="callout" tone="danger" accessibilityLiveRegion="polite">
            {error}
          </Text>
        ) : null}
        {!sent ? (
          <Button
            variant="destructive"
            size="lg"
            title={t('send_code')}
            loading={request.isPending}
            onPress={() => request.mutate()}
          />
        ) : (
          <View className="gap-4">
            <Text variant="callout" tone="muted">
              {t('code_sent')}
            </Text>
            <CodeInput
              value={code}
              onChange={(v) => {
                setCode(v);
                setError(undefined);
              }}
              onComplete={confirm}
              length={CODE_LENGTH}
              error={!!error}
              errorKey={errorKey}
              disabled={remove.isPending}
              accessibilityLabel={t('code_label')}
            />
            <Button
              variant="destructive"
              size="lg"
              title={t('confirm')}
              loading={remove.isPending}
              disabled={code.length !== CODE_LENGTH}
              onPress={() => confirm(code)}
            />
            <Button
              variant="ghost"
              title={t('resend')}
              loading={request.isPending}
              disabled={remove.isPending}
              onPress={() => request.mutate()}
            />
          </View>
        )}
      </Screen>
    </>
  );
}
