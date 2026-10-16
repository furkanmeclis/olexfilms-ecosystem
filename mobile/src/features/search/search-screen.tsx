import { Stack } from 'expo-router';
import { Search } from '@/ui/icons';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TextInput, View } from 'react-native';

import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Screen } from '@/ui/screen';

/**
 * Global search: plate, phone, name, barcode, order number in one box.
 * Results need a mobile search endpoint (mobile/README.md "Backend gaps").
 */
export function SearchScreen() {
  const { t } = useTranslation();
  const [q, setQ] = useState('');
  return (
    <>
      <Stack.Screen options={{ title: t('nav:search') }} />
      <Screen className="pt-3">
        <View className="min-h-12 flex-row items-center gap-2 rounded-lg bg-secondary px-3">
          <Icon as={Search} tone="muted-foreground" size={18} />
          <TextInput
            autoFocus
            value={q}
            onChangeText={setQ}
            placeholder={t('common:search_placeholder')}
            placeholderTextColorClassName="accent-muted-foreground"
            returnKeyType="search"
            autoCorrect={false}
            className="flex-1 text-[17px] text-foreground"
          />
        </View>
        <EmptyState icon={Search} title={t('common:coming_soon_title')} body={t('common:coming_soon_body')} />
      </Screen>
    </>
  );
}
