import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { AuthStep } from './auth-layout';
import { CodeInput } from './code-input';
import { useLogin } from './use-login';

const LENGTH = 6;

export function TotpScreen() {
  const { t } = useTranslation('auth');
  const [code, setCode] = useState('');
  const [error, setError] = useState<string>();
  const [errorKey, setErrorKey] = useState(0);
  const login = useLogin();

  const submit = (c: string) => {
    if (c.length !== LENGTH || login.isPending) return;
    login.mutate(c, {
      onError: (e) => {
        setError(errorMessage(isApiError(e) ? e.code : undefined));
        setErrorKey((k) => k + 1);
        setTimeout(() => setCode(''), 700);
      },
    });
  };

  return (
    <>
      <AuthStep title={t('totp_title')} subtitle={t('totp_hint')}>
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
            ) : null}
          </View>
        </View>
      </AuthStep>
    </>
  );
}
