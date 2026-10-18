import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardAvoidingView, Platform, ScrollView, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { BrandLogo } from '@/ui/brand-logo';
import { Text } from '@/ui/text';

type Props = {
  title: string;
  subtitle?: ReactNode;
  /**
   * The headerless first step: logo on top, content centred, safe-area padding.
   * Later steps sit under the stack header (its back button) and start at the top.
   */
  brand?: boolean;
  children: ReactNode;
};

/** Shared sign-in page: large title + subtitle, then the form (usually a GlassCard). */
export function AuthLayout({ title, subtitle, brand, children }: Props) {
  const { t } = useTranslation('auth');
  const insets = useSafeAreaInsets();

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
      <ScrollView
        keyboardShouldPersistTaps="handled"
        contentContainerClassName={brand ? 'grow justify-center gap-8 px-6' : 'grow gap-8 px-6'}
        contentContainerStyle={{
          paddingTop: brand ? insets.top + 24 : 16,
          paddingBottom: Math.max(insets.bottom, 16) + 16,
        }}>
        <View className="gap-5">
          {brand ? <BrandLogo variant="logo" height={48} label={t('common:app_name')} /> : null}
          <View className="gap-2">
            <Text variant="largeTitle">{title}</Text>
            {subtitle ? (
              <Text variant="callout" tone="muted">
                {subtitle}
              </Text>
            ) : null}
          </View>
        </View>
        {children}
      </ScrollView>
    </KeyboardAvoidingView>
  );
}
