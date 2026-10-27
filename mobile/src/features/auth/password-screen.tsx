import { router } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';
import Animated, { useAnimatedStyle, useReducedMotion, useSharedValue, withSequence, withTiming } from 'react-native-reanimated';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { AuthField } from './auth-field';
import { AuthStep } from './auth-layout';
import { usePendingLogin } from './pending-login';
import { useLogin } from './use-login';

export function PasswordScreen() {
  const { t } = useTranslation('auth');
  const email = usePendingLogin((s) => s.email);
  const password = usePendingLogin((s) => s.password);
  const [visible, setVisible] = useState(false);
  const [error, setError] = useState<string>();
  const [errorKey, setErrorKey] = useState(0);
  const login = useLogin();
  const reduceMotion = useReducedMotion();
  const shake = useSharedValue(0);
  const shakeStyle = useAnimatedStyle(() => ({ transform: [{ translateX: shake.get() }] }));

  useEffect(() => {
    if (!errorKey || reduceMotion) return;
    shake.set(
      withSequence(
        withTiming(-10, { duration: 50 }),
        withTiming(10, { duration: 50 }),
        withTiming(-7, { duration: 50 }),
        withTiming(7, { duration: 50 }),
        withTiming(0, { duration: 50 }),
      ),
    );
  }, [errorKey, reduceMotion, shake]);

  const submit = () => {
    // Empty or blank: no request (useLogin checks it again).
    if (!password.trim() || login.isPending) return;
    login.mutate(undefined, {
      onError: (e) => {
        // An account with authenticator 2FA moves on to the code screen (useLogin).
        if (isApiError(e, 'MFA_REQUIRED')) return;
        setError(errorMessage(isApiError(e) ? e.code : undefined));
        setErrorKey((k) => k + 1);
      },
    });
  };

  return (
    <AuthStep title={t('password_title')}>
      <View className="flex-row items-center gap-3 rounded-2xl border border-border bg-card p-3" style={{ borderCurve: 'continuous' }}>
        <View className="size-11 items-center justify-center rounded-full bg-primary/10">
          <Text variant="headline" tone="primary">
            {email.charAt(0).toLocaleUpperCase()}
          </Text>
        </View>
        <View className="flex-1 gap-0.5">
          <Text variant="footnote" tone="muted">
            {t('password_account')}
          </Text>
          <Text variant="headline" numberOfLines={1}>
            {email}
          </Text>
        </View>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={t('password_change_a11y')}
          hitSlop={8}
          onPress={() => router.back()}
          className="rounded-full px-3 py-2 active:opacity-60">
          <Text variant="callout" tone="primary" className="font-semibold">
            {t('change')}
          </Text>
        </Pressable>
      </View>

      <View className="gap-4">
        <Animated.View style={shakeStyle}>
          <AuthField
            label={t('password_label')}
            placeholder={t('placeholder_password')}
            value={password}
            onChangeText={(v) => {
              usePendingLogin.getState().set({ password: v });
              if (error) setError(undefined);
            }}
            error={error}
            secureTextEntry={!visible}
            autoFocus
            autoCapitalize="none"
            autoCorrect={false}
            autoComplete="current-password"
            textContentType="password"
            returnKeyType="go"
            onSubmitEditing={submit}
            trailing={
              <Pressable
                accessibilityRole="button"
                accessibilityLabel={t(visible ? 'password_hide' : 'password_show')}
                hitSlop={8}
                onPress={() => setVisible((v) => !v)}
                className="min-h-14 justify-center pe-4 ps-2 active:opacity-60">
                <Icon name={visible ? 'hide' : 'show'} tone="muted-foreground" size={20} />
              </Pressable>
            }
          />
        </Animated.View>
        <Button title={t('sign_in')} size="lg" loading={login.isPending} disabled={!password.trim()} onPress={submit} />
      </View>

      <View className="flex-row items-start justify-center gap-2 px-2">
        <Icon name="info" tone="muted-foreground" size={14} />
        <Text variant="footnote" tone="muted" className="shrink">
          {t('forgot_password')}
        </Text>
      </View>
    </AuthStep>
  );
}
