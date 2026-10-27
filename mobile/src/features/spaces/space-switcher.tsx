import * as Haptics from 'expo-haptics';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { isApiError } from '@/core/api/errors';
import { listOrganizations } from '@/core/spaces';
import { BrandLogo } from '@/ui/brand-logo';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

/**
 * Organization switcher, only for accounts with more than one membership
 * (TEC-1016: a single account opens one space, there is no space switch).
 */
export function SpaceSwitcher() {
  const { t } = useTranslation('spaces');
  const me = useSession((s) => s.me);
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
        {error ? (
          <Text variant="footnote" tone="danger" className="text-center">
            {error}
          </Text>
        ) : null}
      </Screen>
    </>
  );
}
