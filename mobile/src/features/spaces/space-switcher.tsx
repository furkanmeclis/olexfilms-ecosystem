import * as Haptics from 'expo-haptics';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { isApiError } from '@/core/api/errors';
import { kindsOf, listSpaces, type Space, type SpaceKind, useActiveSpace, useResolvedSpace } from '@/core/spaces';
import { BrandLogo } from '@/ui/brand-logo';
import { cn } from '@/ui/cn';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

const SPACE_COLOR: Record<SpaceKind, string> = {
  bayi: 'bg-[#1f6b4a]',
  depo: 'bg-[#2f5d8a]',
  admin: 'bg-[#a86d0c]',
  musteri: 'bg-[#6a4a7a]',
};

function Badge({ kind }: { kind: SpaceKind }) {
  const { t } = useTranslation('spaces');
  return (
    <View className={cn('size-9 items-center justify-center rounded-md', SPACE_COLOR[kind])}>
      <Text variant="headline" className="text-white">
        {t(`kind.${kind}`).slice(0, 1)}
      </Text>
    </View>
  );
}

/** Switch between the user's spaces in one tap; no new sign-in. */
export function SpaceSwitcher() {
  const { t } = useTranslation('spaces');
  const me = useSession((s) => s.me);
  const current = useResolvedSpace();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string>();
  const spaces = listSpaces(me);

  const open = async (s: Space) => {
    setError(undefined);
    void Haptics.selectionAsync();
    try {
      if (!s.active) {
        setBusy(s.key);
        const next = await useSession.getState().switchOrganization(s.orgSlug);
        const kinds = kindsOf(next);
        useActiveSpace.getState().setKind(kinds.includes(s.kind) ? s.kind : (kinds[0] ?? s.kind));
      } else {
        useActiveSpace.getState().setKind(s.kind);
      }
      // One POP_TO action: an organization switch can drop the current space's
      // stack (Stack.Protected) before a queued POP_TO_TOP runs, leaving it unhandled.
      router.dismissTo('/');
    } catch (e) {
      setError(errorMessage(isApiError(e) ? e.code : undefined));
    } finally {
      setBusy(null);
    }
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: t('title'),
          headerTitle: ({ children }) => (
            <View className="flex-row items-center gap-2">
              <BrandLogo variant="mark" height={22} />
              <Text variant="headline">{children}</Text>
            </View>
          ),
        }}
      />
      <Screen className="pt-2">
        <List>
          {spaces.map((s, i) => (
            <ListRow
              key={s.key}
              title={s.orgName}
              subtitle={`${t(`kind.${s.kind}`)} · ${t(`org_type.${s.orgType}`)}`}
              leading={<Badge kind={s.kind} />}
              trailing={
                busy === s.key ? (
                  <ActivityIndicator />
                ) : s.active && s.kind === current ? (
                  <StatusPill label={t('active')} tone="done" />
                ) : undefined
              }
              onPress={() => void open(s)}
              last={i === spaces.length - 1}
            />
          ))}
        </List>
        {error ? (
          <Text variant="footnote" tone="danger" className="text-center">
            {error}
          </Text>
        ) : null}
      </Screen>
    </>
  );
}
