import * as Haptics from 'expo-haptics';
import { router, useSegments } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { switchSpace } from '@/core/space-memory';
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
 * Home header leading items: crest, then the user's avatar. A user who also
 * works in the warehouse switches space from it (TEC-1027): a long press
 * jumps to the other space, a tap opens the space sheet.
 */
export function HomeHeaderLeft() {
  return (
    <View className="flex-row items-center gap-2.5">
      <HeaderBrand />
      <SpaceButton />
    </View>
  );
}

/** Avatar; with two spaces (workplace + warehouse) it carries the space switch and a dot of the open space. */
export function SpaceButton() {
  const { t } = useTranslation('spaces');
  const me = useSession((s) => s.me);
  const kinds = kindsOf(me);
  const workplace = kinds.find((k) => k !== 'depo');
  const both = !!workplace && kinds.includes('depo');
  const inDepo = (useSegments() as string[])[1] === 'depo';
  const many = (me?.organizations?.length ?? 0) > 1;
  const user = me?.user;
  const initials = `${user?.name?.[0] ?? ''}${user?.surname?.[0] ?? ''}`.toUpperCase() || '?';
  const avatar = (
    <View className="size-8 items-center justify-center rounded-full bg-secondary">
      <Text variant="footnote" className="font-bold">
        {initials}
      </Text>
      {both ? (
        <View
          className={inDepo ? 'absolute -bottom-0.5 -end-0.5 size-3 rounded-full border-2 border-background bg-brand' : 'absolute -bottom-0.5 -end-0.5 size-3 rounded-full border-2 border-background bg-primary'}
        />
      ) : null}
    </View>
  );
  if (!both && !many) return <View accessible={false}>{avatar}</View>;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('title_spaces')}
      accessibilityHint={both ? t('switch_hint') : undefined}
      hitSlop={8}
      delayLongPress={350}
      onPress={() => router.push({ pathname: '/spaces', params: { from: inDepo ? 'depo' : (workplace ?? '') } })}
      onLongPress={
        both
          ? () => {
              void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
              switchSpace(me, inDepo ? workplace : 'depo');
            }
          : undefined
      }
      className="active:opacity-70">
      {avatar}
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
