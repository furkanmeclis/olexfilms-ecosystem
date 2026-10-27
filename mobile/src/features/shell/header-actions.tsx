import { router, useSegments } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { kindsOf } from '@/core/spaces';
import { NotificationBell } from '@/features/notifications/bell';
import { BrandLogo } from '@/ui/brand-logo';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

/** Olexfilms crest for home headers; decorative, the title says where you are. */
export function HeaderBrand() {
  return <BrandLogo variant="mark" height={30} />;
}

/**
 * Home header leading items: crest, then the avatar that opens the account
 * settings. A space opened over the primary one (the warehouse from the
 * center's menu, TEC-1016) shows a back button instead of the avatar.
 */
export function HomeHeaderLeft() {
  const { t } = useTranslation();
  const space = (useSegments() as string[])[1];
  const primary = useSession((s) => kindsOf(s.me)[0]);
  const over = !!space && !!primary && space !== primary;
  return (
    <View className="flex-row items-center gap-2.5">
      {over ? (
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={t('common:back')}
          hitSlop={8}
          onPress={() => (router.canGoBack() ? router.back() : router.replace('/'))}
          className="size-8 items-center justify-center rounded-full bg-secondary active:opacity-70">
          <Icon name="back" tone="primary" size={17} weight="semibold" />
        </Pressable>
      ) : null}
      <HeaderBrand />
      {over ? null : <SpaceButton />}
    </View>
  );
}

/** Avatar in the header: opens the account settings (TEC-1016: no space switcher). */
export function SpaceButton() {
  const { t } = useTranslation('nav');
  const user = useSession((s) => s.me?.user);
  const initials = `${user?.name?.[0] ?? ''}${user?.surname?.[0] ?? ''}`.toUpperCase() || '?';
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('settings')}
      hitSlop={8}
      onPress={() => router.push('/settings')}
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
        <Icon name="search" tone="primary" />
      </View>
    </Pressable>
  );
}

/** Home header trailing items: the notifications bell, then search. */
export function HomeHeaderRight() {
  return (
    <View className="flex-row items-center gap-2">
      <NotificationBell />
      <SearchButton />
    </View>
  );
}
