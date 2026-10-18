import { router } from 'expo-router';
import type { CountryCode } from 'libphonenumber-js/min';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardAvoidingView, Platform, ScrollView, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { isApiError } from '@/core/api/errors';
import { identify } from '@/core/auth/api';
import { errorMessage } from '@/core/i18n';
import { BrandLogo } from '@/ui/brand-logo';
import { Button } from '@/ui/button';
import { GlassCard } from '@/ui/glass-card';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { CountryPicker } from './country-picker';
import { DevDemoAccounts } from './dev-demo-accounts';
import { usePendingLogin } from './pending-login';
import { DEFAULT_COUNTRY, detectCountry, examplePhone, parsePhone } from './phone-country';

export type SignInMode = 'phone' | 'email';

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Sign-in start: customers sign in by phone (a WhatsApp code), staff and
 * fleet accounts by e-mail and password. Either screen links to the other,
 * since the backend accepts both for every user.
 */
export function SignInScreen({ mode }: { mode: SignInMode }) {
  const { t } = useTranslation('auth');
  const insets = useSafeAreaInsets();
  const phone = mode === 'phone';
  const other: SignInMode = phone ? 'email' : 'phone';

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
      <ScrollView
        keyboardShouldPersistTaps="handled"
        contentContainerClassName="grow justify-center gap-8 px-6"
        contentContainerStyle={{ paddingTop: insets.top + 24, paddingBottom: Math.max(insets.bottom, 16) + 16 }}>
        <View className="gap-5">
          <BrandLogo variant="logo" height={48} label={t('common:app_name')} />
          <View className="gap-2">
            <Text variant="largeTitle">{t(phone ? 'sign_in_phone_title' : 'sign_in_email_title')}</Text>
            <Text variant="callout" tone="muted">
              {t(phone ? 'sign_in_phone_subtitle' : 'sign_in_email_subtitle')}
            </Text>
          </View>
        </View>

        {/* Keyed by mode: switching resets the field, its error and the busy state. */}
        {phone ? <PhoneForm key="phone" /> : <EmailForm key="email" />}

        <View className="items-center">
          <Button
            variant="ghost"
            title={t(phone ? 'sign_in_with_email' : 'sign_in_with_phone')}
            icon={<Icon name={phone ? 'mail' : 'phone'} tone="primary" size={17} />}
            onPress={() => router.setParams({ mode: other })}
          />
        </View>

        {phone ? null : <DevDemoAccounts />}
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

function PhoneForm() {
  const { t } = useTranslation('auth');
  const [country, setCountry] = useState<CountryCode>(DEFAULT_COUNTRY);
  const [value, setValue] = useState('');
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const onChange = (v: string) => {
    setValue(v);
    setError(undefined);
    // A pasted "+44 …" number moves the picker to its own country.
    const detected = detectCountry(v);
    if (detected && detected !== country) setCountry(detected);
  };

  const next = async () => {
    if (busy) return;
    const e164 = parsePhone(value, country);
    if (!e164) return setError(t('invalid_phone'));
    setError(undefined);
    // The backend's identify step sends the WhatsApp code.
    setBusy(true);
    try {
      const res = await identify(e164);
      const phoneE164 = res.phone_e164 ?? e164;
      usePendingLogin.getState().set({ mode: 'phone', phone: phoneE164, code: '' });
      router.push({ pathname: '/phone', params: { phone: phoneE164, resendAt: res.resend_at ?? '' } });
    } catch (e) {
      setError(errorMessage(isApiError(e) ? e.code : undefined));
    } finally {
      setBusy(false);
    }
  };

  return (
    <GlassCard className="gap-5">
      <Input
        icon="phone"
        label={t('phone_label')}
        placeholder={examplePhone(country) ?? t('placeholder_phone')}
        leading={<CountryPicker value={country} onChange={setCountry} />}
        value={value}
        onChangeText={onChange}
        error={error}
        className="bg-background"
        keyboardType="phone-pad"
        textContentType="telephoneNumber"
        autoComplete="tel"
        returnKeyType="next"
        onSubmitEditing={() => void next()}
      />
      <Button title={t('common:continue')} size="lg" loading={busy} disabled={!value.trim()} onPress={() => void next()} />
    </GlassCard>
  );
}

function EmailForm() {
  const { t } = useTranslation('auth');
  const [value, setValue] = useState(usePendingLogin.getState().email);
  const [error, setError] = useState<string>();

  const next = () => {
    const email = value.trim();
    if (!EMAIL.test(email)) return setError(t('invalid_email'));
    setError(undefined);
    usePendingLogin.getState().set({ mode: 'password', email: email.toLowerCase(), password: '' });
    router.push('/password');
  };

  return (
    <GlassCard className="gap-5">
      <Input
        icon="mail"
        label={t('email_label')}
        placeholder={t('placeholder_email')}
        value={value}
        onChangeText={(v) => {
          setValue(v);
          setError(undefined);
        }}
        error={error}
        className="bg-background"
        keyboardType="email-address"
        autoCapitalize="none"
        autoCorrect={false}
        autoComplete="email"
        textContentType="username"
        returnKeyType="next"
        onSubmitEditing={next}
      />
      <Button title={t('common:continue')} size="lg" disabled={!value.trim()} onPress={next} />
    </GlassCard>
  );
}
