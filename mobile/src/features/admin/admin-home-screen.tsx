import { useQueryClient } from '@tanstack/react-query';
import { type Href, router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { formatDigits } from '@/core/digits';
import { useModuleFilter } from '@/core/modules';
import { activeMembership, isCenterWorkplace, isSalesWorkplace, useCan } from '@/core/spaces';
import { SalesBoard, ServicesBoard } from '@/features/center-home/boards';
import { type Period, PERIODS, useReport } from '@/features/reports/api';
import { Segmented } from '@/features/warehouse/parts';
import { StatTiles, TimeChart } from '@/features/reports/report-view';
import { APPROVAL_PERMISSIONS } from '@/features/menu/groups';
import { usePendingOrderCount } from '@/features/warehouse/orders-api';
import { HomeHeaderLeft, HomeHeaderRight } from '@/features/shell/header-actions';
import { Icon } from '@/ui/icon';
import { KpiGridSkeleton } from '@/ui/kpi';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { Chips, ErrorState } from './kit';
import type { IconName } from '@/ui/icons';
import { PullRefresh } from '@/ui/pull-refresh';

function greetingKey(hour: number) {
  if (hour < 12) return 'home:greeting_morning';
  if (hour < 18) return 'home:greeting_day';
  return 'home:greeting_evening';
}

type Board = 'sales' | 'services';

/** `permission`: any of them shows the shortcut. */
type Action = { key: string; icon: IconName; href: Href; permission?: string | string[] };

/**
 * Quick actions. The center opens this home inside its workplace (`bayi`,
 * TEC-1016), where service intake comes first and the admin screens live in
 * the menu; a distributor opens it in the `admin` space with its own tabs.
 */
function actionsFor(workplace: boolean, sales = false): Action[] {
  if (workplace) {
    // TEC-1068: sales staff have Orders in the tab bar and measure from the menu.
    return [
      { key: 'new_service', icon: 'add', href: '/bayi/services/new' as Href, permission: 'services.write' },
      { key: 'measure', icon: 'gauge', href: (sales ? '/bayi/menu/measure' : '/bayi/measure') as Href, permission: 'services.write' },
      { key: 'approvals', icon: 'warrantyClaim', href: '/bayi/menu/approvals' as Href, permission: APPROVAL_PERMISSIONS },
      { key: 'network', icon: 'network', href: '/bayi/menu/network' as Href, permission: 'organizations.read' },
      { key: 'orders', icon: 'cart', href: (sales ? '/bayi/orders' : '/bayi/menu/orders') as Href, permission: 'orders.read' },
      { key: 'reports', icon: 'chart', href: '/bayi/menu/reports' as Href },
    ];
  }
  return [
    { key: 'network', icon: 'network', href: '/admin/network', permission: 'organizations.read' },
    { key: 'approvals', icon: 'warrantyClaim', href: '/admin/approvals', permission: APPROVAL_PERMISSIONS },
    { key: 'orders', icon: 'cart', href: '/admin/menu/orders' as Href, permission: 'orders.read' },
    { key: 'reports', icon: 'chart', href: '/admin/menu/reports' as Href },
  ];
}

/**
 * Admin home: network KPIs from the overview report. The center sees the whole
 * network, a distributor its subtree; the API scopes the numbers.
 */
export function AdminHomeScreen() {
  const { t, i18n } = useTranslation('admin');
  const me = useSession((s) => s.me);
  const org = activeMembership(me);
  const [period, setPeriod] = useState<Period>('30d');
  const canServices = useCan('services.read');
  const perms = new Set(me?.permissions ?? []);
  const moduleOn = useModuleFilter();
  // Orders waiting for the center's warehouse, as a badge on the Orders shortcut (TEC-1025).
  const pendingOrders = usePendingOrderCount(isCenterWorkplace(me) && perms.has('orders.read'));
  const actions = actionsFor(isCenterWorkplace(me), isSalesWorkplace(me)).filter(
    (a) => (!a.permission || me?.user.is_super_admin || [a.permission].flat().some((p) => perms.has(p))) && moduleOn(a.key),
  );

  // Center workplace boards: sales for the sales staff (TEC-1068), the
  // network's services for the center (TEC-1073); a switch when both apply.
  const workplace = isCenterWorkplace(me);
  const sales = isSalesWorkplace(me) && moduleOn('orders');
  const boards: Board[] = workplace ? [...(sales ? (['sales'] as const) : []), ...(canServices ? (['services'] as const) : [])] : [];
  const [board, setBoard] = useState<Board | null>(null);
  const shownBoard = board && boards.includes(board) ? board : boards[0];
  const ordersBase = sales ? '/bayi/orders' : '/bayi/menu/orders';

  const overview = useReport('overview', { period });
  const trend = useReport('services.trend', { period, enabled: canServices });
  const cards = overview.data?.series.find((s) => s.key === 'cards');
  const trendSeries = trend.data?.series.find((s) => s.key === 'created');

  const qc = useQueryClient();
  const refresh = () => {
    void overview.refetch();
    if (canServices) void trend.refetch();
    if (boards.length) {
      void qc.invalidateQueries({ queryKey: ['center-home'] });
      void qc.invalidateQueries({ queryKey: ['reports', 'report'] });
    }
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: t(greetingKey(new Date().getHours()), { name: me?.user.name ?? '' }),
          headerLeft: () => <HomeHeaderLeft />,
          headerRight: () => <HomeHeaderRight />,
        }}
      />
      <Screen refreshControl={<PullRefresh busy={overview.isRefetching} onRefresh={refresh} />}>
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
          <KpiGridSkeleton />
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
              <View className="flex-row items-start justify-between">
                <View className="size-9 items-center justify-center rounded-md bg-secondary">
                  <Icon name={a.icon} tone="primary" size={20} />
                </View>
                {a.key === 'orders' && (pendingOrders.data ?? 0) > 0 ? (
                  <View className="min-w-6 items-center rounded-full bg-brand px-2 py-0.5">
                    <Text variant="footnote" className="font-bold tabular-nums" style={{ color: '#002112' }}>
                      {formatDigits(pendingOrders.data ?? 0, i18n.language)}
                    </Text>
                  </View>
                ) : null}
              </View>
              <Text variant="headline">{t(`home.action.${a.key}`)}</Text>
            </Pressable>
          ))}
        </View>

        {boards.length > 1 ? (
          <Segmented
            value={shownBoard}
            onChange={setBoard}
            options={boards.map((b) => ({ value: b, label: t(`home.board_${b}`) }))}
          />
        ) : null}
        {shownBoard === 'sales' ? <SalesBoard period={period} ordersBase={ordersBase} /> : null}
        {shownBoard === 'services' ? <ServicesBoard period={period} /> : null}
      </Screen>
    </>
  );
}
