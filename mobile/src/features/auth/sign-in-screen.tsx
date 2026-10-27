import { router } from 'expo-router';
import type { CountryCode } from 'libphonenumber-js/min';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardAvoidingView, Platform, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { isApiError } from '@/core/api/errors';
import { identify } from '@/core/auth/api';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Text } from '@/ui/text';

import { AuthField as Field } from './auth-field';
import { AuthHero, useKeyboardVisible } from './auth-layout';
import { CountryPicker } from './country-picker';
import { DevDemoAccounts } from './dev-demo-accounts';
import { usePendingLogin } from './pending-login';
import { DEFAULT_COUNTRY, detectCountry, examplePhone, parsePhone } from './phone-country';

export type SignInMode = 'phone' | 'email';

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Sign-in start: customers sign in by phone (a WhatsApp code), staff and
 * fleet accounts by e-mail and password. Either screen links to the other,
 * since the backend accepts both for every user. The brand panel fills the
 * top; the form sits in a sheet that rises with the keyboard.
 */
export function SignInScreen({ mode }: { mode: SignInMode }) {
  const { t } = useTranslation('auth');
  const insets = useSafeAreaInsets();
  const keyboard = useKeyboardVisible();
  const phone = mode === 'phone';
  const other: SignInMode = phone ? 'email' : 'phone';

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      className="flex-1 bg-background">
      <AuthHero title={t('welcome_title')} subtitle={t('welcome_subtitle')} />

      <View
        className="-mt-8 gap-6 rounded-t-[32px] bg-background px-6 pt-7"
        // Over the keyboard the home-indicator inset is not needed.
        style={{ paddingBottom: keyboard ? 16 : Math.max(insets.bottom, 16) + 8, borderCurve: 'continuous' }}>
        <View className="gap-1">
          <Text variant="title">{t(phone ? 'phone_title' : 'sign_in_email_title')}</Text>
          <Text variant="footnote" tone="muted">
            {t(phone ? 'sign_in_phone_subtitle' : 'sign_in_email_subtitle')}
          </Text>
        </View>

        {/* Keyed by mode: switching resets the field, its error and the busy state. */}
        {phone ? <PhoneForm key="phone" /> : <EmailForm key="email" />}

        <View className="flex-row items-center gap-3">
          <View className="h-px flex-1 bg-border" />
          <Text variant="footnote" tone="muted">
            {t('or')}
          </Text>
          <View className="h-px flex-1 bg-border" />
        </View>

        <Button
          variant="outline"
          size="lg"
          title={t(phone ? 'sign_in_email_title' : 'phone_title')}
          icon={phone ? 'mail' : 'phone'}
          onPress={() => router.setParams({ mode: other })}
        />

        {phone ? null : <DevDemoAccounts />}
      </View>
    </KeyboardAvoidingView>
  );
}

function PhoneForm() {
  const { t } = useTranslation('auth');
  const [country, setCountry] = useState<CountryCode>(DEFAULT_COUNTRY);
  const [value, setValue] = useState('');
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const placeholder = useMemo(() => examplePhone(country) ?? t('placeholder_phone'), [country, t]);

  const onChange = (v: string) => {
    setValue(v);
    setError(undefined);
    // A pasted "+44 …" number moves the picker to its own country.
    const detected = detectCountry(v);
    if (detected && detected !== country) setCountry(detected);
  };

  // A new country makes an "invalid number" message about the old one stale.
  const onCountry = (code: CountryCode) => {
    setCountry(code);
    setError(undefined);
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
    <View className="gap-4">
      <Field
        label={t('phone_label')}
        placeholder={placeholder}
        leading={<CountryPicker bare value={country} onChange={onCountry} />}
        value={value}
        onChangeText={onChange}
        error={error}
        // Digits and "+" never mirror; 'left' is the start edge (RN swaps it in RTL).
        style={{ writingDirection: 'ltr', textAlign: 'left' }}
        keyboardType="phone-pad"
        textContentType="telephoneNumber"
        autoComplete="tel"
        // No returnKeyType: on the number pad RN adds its own accessory bar with an untranslated "Next".
      />
      <Button title={t('common:continue')} size="lg" loading={busy} disabled={!value.trim()} onPress={() => void next()} />
    </View>
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
    <View className="gap-4">
      <Field
        label={t('email_label')}
        placeholder={t('placeholder_email')}
        value={value}
        onChangeText={(v) => {
          setValue(v);
          setError(undefined);
        }}
        error={error}
        keyboardType="email-address"
        autoCapitalize="none"
        autoCorrect={false}
        autoComplete="email"
        textContentType="username"
        returnKeyType="next"
        onSubmitEditing={next}
      />
      <Button title={t('common:continue')} size="lg" disabled={!value.trim()} onPress={next} />
    </View>
  );
}
