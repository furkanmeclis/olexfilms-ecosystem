import { type Href, router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { activeMembership } from '@/core/spaces';
import { HomeHeaderLeft, SearchButton } from '@/features/shell/header-actions';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import type { IconName } from '@/ui/icons';

export type QuickAction = { key: string; icon: IconName; href: Href };

function greetingKey(hour: number) {
  if (hour < 12) return 'home:greeting_morning';
  if (hour < 18) return 'home:greeting_day';
  return 'home:greeting_evening';
}

/**
 * Home of every staff space: who and where you are, the next job first, then
 * quick actions. Each space passes its own actions; the layout is shared.
 */
export function HomeScreen({ actions }: { actions: QuickAction[] }) {
  const { t } = useTranslation();
  const me = useSession((s) => s.me);
  const org = activeMembership(me);

  return (
    <>
      <Stack.Screen
        options={{
          title: t(greetingKey(new Date().getHours()), { name: me?.user.name ?? '' }),
          headerLeft: () => <HomeHeaderLeft />,
          headerRight: () => <SearchButton />,
        }}
      />
      <Screen>
        {org ? (
          <Text variant="footnote" tone="muted">
            {org.name} · {t(`spaces:org_type.${org.type ?? 'dealer'}`)}
          </Text>
        ) : null}

        <View className="rounded-lg border border-border bg-card">
          <Text variant="caption" tone="muted" className="px-4 pt-3">
            {t('home:next_up')}
          </Text>
          <EmptyState icon="inbox" title={t('home:nothing_next')} />
        </View>

        <SectionHeader title={t('home:quick_actions')} />
        <View className="flex-row flex-wrap gap-2.5">
          {actions.map((a) => (
            <Pressable
              key={a.key}
              accessibilityRole="button"
              onPress={() => router.push(a.href)}
              className="min-h-24 grow basis-[46%] justify-between rounded-lg border border-border bg-card p-3.5 active:opacity-80">
              <View className="size-9 items-center justify-center rounded-md bg-secondary">
                <Icon name={a.icon} tone="primary" size={20} />
              </View>
              <Text variant="headline">{t(`home:action.${a.key}`)}</Text>
            </Pressable>
          ))}
        </View>
      </Screen>
    </>
  );
}
