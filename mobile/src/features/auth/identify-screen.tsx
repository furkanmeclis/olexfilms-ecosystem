import { router } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardAvoidingView, Platform, View } from 'react-native';

import { identify } from '@/core/auth/api';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { BrandLogo } from '@/ui/brand-logo';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { parseIdentifier } from './identifier';
import { usePendingLogin } from './pending-login';

export function IdentifyScreen() {
  const { t } = useTranslation('auth');
  const [value, setValue] = useState(usePendingLogin.getState().email);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const next = async () => {
    const id = parseIdentifier(value);
    if (!id) return setError(t('invalid_identifier'));
    setError(undefined);
    if (id.kind === 'email') {
      usePendingLogin.getState().set({ mode: 'password', email: id.email, password: '' });
      router.push('/password');
      return;
    }
    // A phone gets a WhatsApp code from the backend's identify step.
    setBusy(true);
    try {
      const res = await identify(id.e164);
      const phone = res.phone_e164 ?? id.e164;
      usePendingLogin.getState().set({ mode: 'phone', phone, code: '' });
      router.push({ pathname: '/phone', params: { phone, resendAt: res.resend_at ?? '' } });
    } catch (e) {
      setError(errorMessage(isApiError(e) ? e.code : undefined));
    } finally {
      setBusy(false);
    }
  };

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
      <View className="flex-1 justify-center gap-8 px-6">
        <View className="gap-4">
          <BrandLogo variant="logo" height={56} label={t('common:app_name')} />
          <Text variant="callout" tone="muted">
            {t('identify_subtitle')}
          </Text>
        </View>
        <View className="gap-4">
          <Input
            label={t('identifier_label')}
            placeholder={t('identifier_placeholder')}
            value={value}
            onChangeText={setValue}
            error={error}
            autoCapitalize="none"
            autoCorrect={false}
            autoComplete="username"
            textContentType="username"
            keyboardType="email-address"
            returnKeyType="next"
            onSubmitEditing={() => void next()}
          />
          <Button title={t('common:continue')} size="lg" loading={busy} onPress={() => void next()} />
        </View>
      </View>
    </KeyboardAvoidingView>
  );
}
