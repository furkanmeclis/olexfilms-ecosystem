import * as Haptics from 'expo-haptics';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { isApiError } from '@/core/api/errors';
import { switchSpace } from '@/core/space-memory';
import { kindsOf, listOrganizations, type SpaceKind } from '@/core/spaces';
import { BrandLogo } from '@/ui/brand-logo';
import { cn } from '@/ui/cn';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

const SPACE_COLOR: Partial<Record<SpaceKind, string>> = {
  bayi: 'bg-[#1f6b4a]',
  admin: 'bg-[#a86d0c]',
  depo: 'bg-[#2f5d8a]',
};

function Badge({ kind, label }: { kind: SpaceKind; label: string }) {
  return (
    <View className={cn('size-9 items-center justify-center rounded-md', SPACE_COLOR[kind] ?? 'bg-secondary')}>
      <Text variant="headline" className="text-white">
        {label.slice(0, 1)}
      </Text>
    </View>
  );
}

/**
 * Space sheet (avatar tap; a long press switches directly, TEC-1027): the
 * workplace and the warehouse of the same account, then the organizations
 * when the user has more than one membership.
 */
export function SpaceSwitcher() {
  const { t } = useTranslation('spaces');
  const me = useSession((s) => s.me);
  // The avatar passes the space it was tapped in (the sheet's own route has none).
  const { from } = useLocalSearchParams<{ from?: string }>();
  const kinds = kindsOf(me);
  const workplace = kinds.find((k) => k !== 'depo');
  const spaces: { kind: SpaceKind; label: string }[] =
    workplace && kinds.includes('depo')
      ? [
          { kind: workplace, label: t('workplace') },
          { kind: 'depo', label: t('kind.depo') },
        ]
      : [];
  const current: SpaceKind | undefined = from === 'depo' ? 'depo' : workplace;
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string>();
  const orgs = listOrganizations(me);

  const open = async (slug: string, active: boolean) => {
    setError(undefined);
    void Haptics.selectionAsync();
    try {
      if (!active) {
        setBusy(slug);
        await useSession.getState().switchOrganization(slug);
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
          title: spaces.length ? t('title_spaces') : t('title'),
          headerTitle: ({ children }) => (
            <View className="flex-row items-center gap-2">
              <BrandLogo variant="mark" height={22} />
              <Text variant="headline">{children}</Text>
            </View>
          ),
        }}
      />
      <View className="gap-1 px-4 pb-8 pt-2">
        {spaces.length ? (
          <>
            <SectionHeader title={t('spaces_title')} />
            <List>
              {spaces.map((sp, i) => (
                <ListRow
                  key={sp.kind}
                  title={sp.label}
                  leading={<Badge kind={sp.kind} label={sp.label} />}
                  trailing={sp.kind === current ? <StatusPill label={t('active')} tone="done" /> : undefined}
                  onPress={() => {
                    void Haptics.selectionAsync();
                    // Close the sheet first, so the new space replaces the old one instead of stacking on it.
                    router.back();
                    if (sp.kind !== current) switchSpace(me, sp.kind);
                  }}
                  last={i === spaces.length - 1}
                />
              ))}
            </List>
          </>
        ) : null}
        {orgs.length > 1 ? <SectionHeader title={t('orgs_title')} /> : null}
        {orgs.length > 1 ? (
          <List>
            {orgs.map((o, i) => {
              const active = o.uuid === me?.active_organization_uuid;
              return (
                <ListRow
                  key={o.uuid}
                  title={o.name}
                  subtitle={t(`org_type.${o.type ?? 'dealer'}`)}
                  icon="building"
                  trailing={
                    busy === o.slug ? <ActivityIndicator /> : active ? <StatusPill label={t('active')} tone="done" /> : undefined
                  }
                  onPress={() => void open(o.slug, active)}
                  last={i === orgs.length - 1}
                />
              );
            })}
          </List>
        ) : null}
        {error ? (
          <Text variant="footnote" tone="danger" className="text-center">
            {error}
          </Text>
        ) : null}
      </View>
    </>
  );
}
