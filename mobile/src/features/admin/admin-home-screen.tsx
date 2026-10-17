import { type Href, router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { activeMembership, useCan } from '@/core/spaces';
import { type Period, PERIODS, useReport } from '@/features/reports/api';
import { StatTiles, TimeChart } from '@/features/reports/report-view';
import { HomeHeaderLeft, SearchButton } from '@/features/shell/header-actions';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import { Chips, ErrorState } from './kit';
import type { IconName } from '@/ui/icons';

function greetingKey(hour: number) {
  if (hour < 12) return 'home:greeting_morning';
  if (hour < 18) return 'home:greeting_day';
  return 'home:greeting_evening';
}

type Action = { key: string; icon: IconName; href: Href; permission?: string };

const ACTIONS: Action[] = [
  { key: 'network', icon: 'network', href: '/admin/network', permission: 'organizations.read' },
  { key: 'approvals', icon: 'warrantyClaim', href: '/admin/approvals' },
  { key: 'orders', icon: 'cart', href: '/admin/menu/orders' as Href, permission: 'orders.read' },
  { key: 'reports', icon: 'chart', href: '/admin/menu/reports' as Href },
];

/**
 * Admin home: network KPIs from the overview report. The center sees the whole
 * network, a distributor its subtree; the API scopes the numbers.
 */
export function AdminHomeScreen() {
  const { t } = useTranslation('admin');
  const me = useSession((s) => s.me);
  const org = activeMembership(me);
  const [period, setPeriod] = useState<Period>('30d');
  const canServices = useCan('services.read');
  const perms = new Set(me?.permissions ?? []);
  const actions = ACTIONS.filter((a) => !a.permission || me?.user.is_super_admin || perms.has(a.permission));

  const overview = useReport('overview', { period });
  const trend = useReport('services.trend', { period, enabled: canServices });
  const cards = overview.data?.series.find((s) => s.key === 'cards');
  const trendSeries = trend.data?.series.find((s) => s.key === 'created');

  const refresh = () => {
    void overview.refetch();
    if (canServices) void trend.refetch();
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: t(greetingKey(new Date().getHours()), { name: me?.user.name ?? '' }),
          headerLeft: () => <HomeHeaderLeft />,
          headerRight: () => <SearchButton />,
        }}
      />
      <Screen refreshControl={<RefreshControl refreshing={overview.isRefetching} onRefresh={refresh} />}>
        {org ? (
          <Text variant="footnote" tone="muted">
            {org.name} · {t(`spaces:org_type.${org.type ?? 'center'}`)} ·{' '}
            {t(org.type === 'distributor' ? 'home.scope_subtree' : 'home.scope_network')}
          </Text>
        ) : null}

        <View className="-mx-4">
          <Chips
            options={PERIODS.map((p) => ({ key: p, label: t(`reports.period.${p}`) }))}
            value={period}
            onChange={setPeriod}
          />
        </View>

        {overview.isPending ? (
          <View className="flex-row flex-wrap gap-2.5">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-20 grow basis-[46%] rounded-lg" />
            ))}
          </View>
        ) : overview.isError && !overview.data ? (
          <ErrorState error={overview.error} onRetry={() => void overview.refetch()} />
        ) : cards && cards.points.length ? (
          <StatTiles series={cards} />
        ) : (
          <Text variant="callout" tone="muted" className="py-4 text-center">
            {t('reports.no_data')}
          </Text>
        )}

        {canServices && trendSeries && !(trend.isError && isApiError(trend.error)) ? (
          <TimeChart series={trendSeries} compact />
        ) : null}

        <SectionHeader title={t('home.quick_actions')} />
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
              <Text variant="headline">{t(`home.action.${a.key}`)}</Text>
            </Pressable>
          ))}
        </View>
      </Screen>
    </>
  );
}
