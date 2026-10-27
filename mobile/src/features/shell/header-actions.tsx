import * as Haptics from 'expo-haptics';
import { router, useSegments } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { switchSpace } from '@/core/space-memory';
import { kindsOf, type SpaceKind } from '@/core/spaces';
import { NotificationBell } from '@/features/notifications/bell';
import { BrandLogo } from '@/ui/brand-logo';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

/** Olexfilms crest for home headers; decorative, the title says where you are. */
export function HeaderBrand() {
  return <BrandLogo variant="mark" height={30} />;
}

/**
 * Home header leading items: crest, then the space switch when the user also
 * works in the warehouse (TEC-1027: workplace ↔ warehouse, same account and
 * organization), else the user's initials.
 */
export function HomeHeaderLeft() {
  const me = useSession((s) => s.me);
  const kinds = kindsOf(me);
  const workplace = kinds.find((k) => k !== 'depo');
  const both = !!workplace && kinds.includes('depo');
  return (
    <View className="flex-row items-center gap-2.5">
      <HeaderBrand />
      {both ? <SpaceSwitch workplace={workplace} /> : <SpaceButton />}
    </View>
  );
}

/** Two-way switch between the workplace and the warehouse; the current one is filled. */
function SpaceSwitch({ workplace }: { workplace: SpaceKind }) {
  const { t } = useTranslation('spaces');
  const me = useSession((s) => s.me);
  const current = (useSegments() as string[])[1] === 'depo' ? 'depo' : workplace;
  const options: { kind: SpaceKind; label: string }[] = [
    { kind: workplace, label: t('workplace') },
    { kind: 'depo', label: t('kind.depo') },
  ];
  return (
    <View className="flex-row rounded-full bg-secondary p-0.5" accessibilityRole="tablist">
      {options.map((o) => {
        const on = o.kind === current;
        return (
          <Pressable
            key={o.kind}
            accessibilityRole="tab"
            accessibilityState={{ selected: on }}
            hitSlop={4}
            onPress={() => {
              if (on) return;
              void Haptics.selectionAsync();
              switchSpace(me, o.kind);
            }}
            className={on ? 'rounded-full bg-primary px-3 py-1' : 'rounded-full px-3 py-1 active:opacity-70'}>
            <Text variant="footnote" tone={on ? 'onPrimary' : 'default'} className="font-semibold">
              {o.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

/** The user's initials (no action: Settings sit in the menu; TEC-1027). */
export function SpaceButton() {
  const user = useSession((s) => s.me?.user);
  const initials = `${user?.name?.[0] ?? ''}${user?.surname?.[0] ?? ''}`.toUpperCase() || '?';
  return (
    <View accessible={false} className="size-8 items-center justify-center rounded-full bg-secondary">
      <Text variant="footnote" className="font-bold">
        {initials}
      </Text>
    </View>
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
