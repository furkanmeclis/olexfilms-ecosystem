import * as Haptics from 'expo-haptics';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { type StepUpMethod, verifyStepUp } from '@/features/auth/step-up';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { whError } from './format';
import { Card, Segmented } from './parts';

/**
 * Inline identity check before a sensitive action (TEC-961): the password or
 * an authenticator code earns the step-up grant, then `onVerified` retries
 * the action. Passkeys stay on the web panel.
 */
export function StepUpCard({ action, onVerified, onCancel }: { action: string; onVerified: () => Promise<void>; onCancel: () => void }) {
  const { t } = useTranslation('warehouse');
  const [method, setMethod] = useState<StepUpMethod>('password');
  const [secret, setSecret] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  const message = (e: unknown) => {
    if (!isApiError(e)) return whError(e);
    if (e.status === 401) return t(method === 'password' ? 'reclass.step_up_wrong_password' : 'reclass.step_up_wrong_code');
    if (e.status === 429) return t('reclass.step_up_rate_limited');
    if (e.status === 400) return t('reclass.step_up_method_unavailable');
    return whError(e);
  };

  const submit = async () => {
    if (!secret.trim() || busy) return;
    setBusy(true);
    setError(undefined);
    try {
      await verifyStepUp(method, secret.trim());
      setSecret('');
      await onVerified();
    } catch (e) {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="gap-3">
      <View className="gap-1">
        <Text variant="headline">{t('reclass.step_up_title')}</Text>
        <Text variant="footnote" tone="muted">
          {t('reclass.step_up_body')}
        </Text>
      </View>
      <Segmented
        value={method}
        onChange={(m) => {
          setMethod(m);
          setSecret('');
          setError(undefined);
        }}
        options={[
          { value: 'password', label: t('reclass.step_up_password') },
          { value: 'totp', label: t('reclass.step_up_totp') },
        ]}
      />
      {method === 'password' ? (
        <Input
          key="password"
          icon="password"
          label={t('reclass.step_up_password')}
          placeholder={t('reclass.step_up_password_placeholder')}
          value={secret}
          onChangeText={setSecret}
          secureTextEntry
          autoFocus
          autoComplete="current-password"
          textContentType="password"
          returnKeyType="go"
          onSubmitEditing={() => void submit()}
          error={error}
        />
      ) : (
        <Input
          key="totp"
          icon="shield"
          label={t('reclass.step_up_totp')}
          placeholder={t('reclass.step_up_totp_placeholder')}
          hint={t('reclass.step_up_totp_hint')}
          value={secret}
          onChangeText={(v) => setSecret(v.replace(/\s/g, ''))}
          autoFocus
          autoCapitalize="none"
          autoCorrect={false}
          autoComplete="one-time-code"
          textContentType="oneTimeCode"
          returnKeyType="go"
          onSubmitEditing={() => void submit()}
          error={error}
        />
      )}
      <Button title={action} loading={busy} disabled={!secret.trim()} onPress={() => void submit()} />
      <Button title={t('common:cancel')} variant="ghost" disabled={busy} onPress={onCancel} />
      <Text variant="footnote" tone="muted">
        {t('reclass.step_up_passkey_web')}
      </Text>
    </Card>
  );
}
