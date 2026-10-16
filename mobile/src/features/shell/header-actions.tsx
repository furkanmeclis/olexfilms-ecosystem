import { router } from 'expo-router';
import { Search } from '@/ui/icons';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

/** Avatar in the header: opens the space switcher. */
export function SpaceButton() {
  const { t } = useTranslation('spaces');
  const user = useSession((s) => s.me?.user);
  const initials = `${user?.name?.[0] ?? ''}${user?.surname?.[0] ?? ''}`.toUpperCase() || '?';
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('title')}
      hitSlop={8}
      onPress={() => router.push('/spaces')}
      className="size-8 items-center justify-center rounded-full bg-secondary active:opacity-70">
      <Text variant="footnote" className="font-bold">
        {initials}
      </Text>
    </Pressable>
  );
}

/** One search box for plates, phones, names and barcodes, on every home screen. */
export function SearchButton() {
  const { t } = useTranslation('nav');
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={t('search')} hitSlop={8} onPress={() => router.push('/search')}>
      <View className="size-8 items-center justify-center">
        <Icon as={Search} tone="primary" />
      </View>
    </Pressable>
  );
}
