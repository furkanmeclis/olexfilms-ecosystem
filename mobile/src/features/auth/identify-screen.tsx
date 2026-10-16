import { router } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardAvoidingView, Platform, View } from 'react-native';

import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { parseIdentifier } from './identifier';
import { usePendingLogin } from './pending-login';

export function IdentifyScreen() {
  const { t } = useTranslation('auth');
  const [value, setValue] = useState(usePendingLogin.getState().email);
  const [error, setError] = useState<string>();

  const next = () => {
    const id = parseIdentifier(value);
    if (!id) return setError(t('invalid_identifier'));
    setError(undefined);
    if (id.kind === 'email') {
      usePendingLogin.getState().set({ email: id.email, password: '' });
      router.push('/password');
    } else {
      router.push({ pathname: '/phone', params: { phone: id.e164 } });
    }
  };

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
      <View className="flex-1 justify-center gap-8 px-6">
        <View className="gap-3">
          <View className="size-14 items-center justify-center rounded-xl bg-primary">
            <Text variant="title" tone="onPrimary" className="font-extrabold">
              OF
            </Text>
          </View>
          <Text variant="largeTitle">{t('common:app_name')}</Text>
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
            onSubmitEditing={next}
          />
          <Button title={t('common:continue')} size="lg" onPress={next} />
        </View>
      </View>
    </KeyboardAvoidingView>
  );
}
