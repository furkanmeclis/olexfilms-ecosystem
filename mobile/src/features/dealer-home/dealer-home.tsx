import { useQueries, useQuery } from '@tanstack/react-query';
import { type Href, router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { useModuleFilter } from '@/core/modules';
import { activeMembership, isCenterWorkplace, useCan } from '@/core/spaces';
import { AdminHomeScreen } from '@/features/admin/admin-home-screen';
import { BrandMark, PlateBadge } from '@/features/customer-space/home-parts';
import { formatDate, formatNumber, fullName, type Service, serviceTone, useLinks, vehicleLabel } from '@/features/services/lib';
import { errorText } from '@/features/services/ui';
import { HomeHeaderLeft, HomeHeaderRight } from '@/features/shell/header-actions';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { KpiCard, KpiGrid, KpiGridSkeleton } from '@/ui/kpi';
import { Skeleton } from '@/ui/skeleton';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import type { IconName } from '@/ui/icons';
import { PullRefresh } from '@/ui/pull-refresh';

import { BOARD_GROUPS, type BoardGroup, boardRows, fitTags, kpiIcon, listFilterOf, statusParam, uniqueCategories } from './board';

function greetingKey(hour: number) {
  if (hour < 12) return 'home:greeting_morning';
  if (hour < 18) return 'home:greeting_day';
  return 'home:greeting_evening';
}

type Action = { key: string; icon: IconName; href: Href; show: boolean };

/**
 * Dealer home: the jobs a dealer starts most, the last 30 days in numbers
 * (reports overview cards) and the board of services to work on.
 */
export function DealerHome() {
  const { t, i18n } = useTranslation('dealer');
  const me = useSession((s) => s.me);
  const org = activeMembership(me);
  const links = useLinks();
  const canWrite = useCan('services.write');
  const canServices = useCan('services.read');
  const canCustomers = useCan('customers.read');
  const canAccounting = useCan('accounting.read');
  const canSell = useCan('product_sales.write');
  const canOrders = useCan('orders.read');

  const overview = useQuery({
    queryKey: ['dealer-home', 'overview', i18n.language],
    queryFn: () => unwrap(api.GET('/v1/reports/overview', { params: { query: { period: '30d' } } })),
  });
  // One request per board group: each brings its own `total` and server order.
  const board = useQueries({
    queries: BOARD_GROUPS.map((g) => ({
      queryKey: ['dealer-home', 'board', g.key],
      queryFn: () =>
        unwrap(
          api.GET('/v1/services', {
            params: { query: { status: statusParam(g), sort: g.sort, limit: g.limit, offset: 0 } },
          }),
        ),
      enabled: canServices,
    })),
  });

  const cards = overview.data?.series.find((s) => s.key === 'cards')?.points ?? overview.data?.series[0]?.points ?? [];

  const moduleOn = useModuleFilter();
  // Scan and measure stay in the tab bar; the home keeps the desk jobs.
  const actions = (
    [
      { key: 'new_service', icon: 'add', href: links.newService, show: canWrite },
      { key: 'customers', icon: 'customers', href: '/bayi/menu/customers' as Href, show: canCustomers },
      // TEC-866: the warranty list is closed to dealers; its shortcut became the
      // services list (warranties open from the service detail).
      { key: 'services', icon: 'service', href: '/bayi/services', show: canServices },
      // Without services (accounting roles): the desk jobs of the finance side.
      { key: 'accounting', icon: 'accounting', href: '/bayi/menu/accounting' as Href, show: !canServices && canAccounting },
      { key: 'quick_sale', icon: 'cart', href: '/bayi/menu/quick_sale/new' as Href, show: !canServices && canSell },
      { key: 'orders', icon: 'order', href: '/bayi/menu/orders' as Href, show: !canServices && canOrders },
    ] satisfies Action[]
  ).filter((a) => moduleOn(a.key) && a.show);

  const refresh = () => {
    void overview.refetch();
    if (canServices) board.forEach((q) => void q.refetch());
  };
  const refreshing = overview.isRefetching || board.some((q) => q.isRefetching);

  return (
    <>
      <Stack.Screen
        options={{
          title: t(greetingKey(new Date().getHours()), { name: me?.user.name ?? '' }),
          headerLeft: () => <HomeHeaderLeft />,
          headerRight: () => <HomeHeaderRight />,
        }}
      />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-5 px-4 pb-12 pt-2"
        refreshControl={<PullRefresh busy={refreshing} onRefresh={refresh} />}>
        {org ? (
          <Text variant="footnote" tone="muted">
            {org.name} · {t(`spaces:org_type.${org.type ?? 'dealer'}`)}
          </Text>
        ) : null}

        {actions.length > 0 ? (
          <View className="flex-row gap-2.5">
            {actions.map((a) => (
              <Pressable
                key={a.key}
                accessibilityRole="button"
                onPress={() => router.push(a.href)}
                className="min-h-[84px] flex-1 justify-between rounded-2xl border border-border bg-card p-3 active:opacity-80"
                style={{ borderCurve: 'continuous' }}>
                <View className="size-8 items-center justify-center rounded-full bg-secondary">
                  <Icon name={a.icon} tone="primary" size={17} />
                </View>
                <Text variant="callout" className="font-semibold" numberOfLines={1}>
                  {t(`home_action.${a.key}`)}
                </Text>
              </Pressable>
            ))}
          </View>
        ) : null}

        {overview.isPending ? (
          <View className="gap-2">
            <Skeleton className="mx-1 h-3 w-20" />
            <KpiGridSkeleton />
          </View>
        ) : cards.length > 0 ? (
          <View className="gap-2">
            <Text variant="caption" tone="muted" className="px-1">
              {t('last_30_days')}
            </Text>
            <KpiGrid>
              {cards.slice(0, 8).map((c) => (
                <KpiCard key={c.key} icon={kpiIcon(c.key)} label={c.label} value={formatNumber(c.value, i18n.language)} />
              ))}
            </KpiGrid>
          </View>
        ) : overview.isError ? (
          <View className="items-center gap-2 rounded-2xl border border-border bg-card p-4">
            <Text variant="callout" tone="muted" className="text-center">
              {errorText(overview.error)}
            </Text>
            <Button title={t('common:retry')} variant="secondary" onPress={() => void overview.refetch()} />
          </View>
        ) : null}

        {canServices ? <Board queries={board} canWrite={canWrite} /> : null}
      </ScrollView>
    </>
  );
}

type Page = { items: Service[]; total: number };
type BoardQuery = { data?: Page; isPending: boolean; error: unknown; refetch: () => unknown };

/** "My board": next up, in progress, completed. */
function Board({ queries, canWrite }: { queries: BoardQuery[]; canWrite: boolean }) {
  const { t } = useTranslation('dealer');
  const links = useLinks();
  const failed = queries.find((q) => q.error);
  const loading = queries.some((q) => q.isPending);
  const empty = !loading && !failed && queries.every((q) => (q.data?.total ?? 0) === 0);

  return (
    <View className="gap-4">
      <Text variant="title" className="px-1">
        {t('board_title')}
      </Text>
      {loading ? (
        <View className="gap-2">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-[84px] rounded-2xl" />
          ))}
        </View>
      ) : failed ? (
        <View className="items-center gap-2 rounded-2xl border border-border bg-card p-4">
          <Text variant="callout" tone="muted" className="text-center">
            {errorText(failed.error)}
          </Text>
          <Button
            title={t('common:retry')}
            variant="secondary"
            onPress={() => queries.forEach((q) => void q.refetch())}
          />
        </View>
      ) : empty ? (
        <View className="items-center gap-2 rounded-2xl border border-border bg-card p-5">
          <Icon name="service" tone="muted-foreground" />
          <Text variant="callout" tone="muted" className="text-center">
            {t('services_empty_body')}
          </Text>
          {canWrite ? <Button title={t('new_service')} onPress={() => router.push(links.newService)} /> : null}
        </View>
      ) : (
        BOARD_GROUPS.map((g, i) => (
          <BoardSection key={g.key} group={g} page={queries[i]?.data} prominent={g.key === 'next'} />
        ))
      )}
    </View>
  );
}

function BoardSection({ group, page, prominent }: { group: BoardGroup; page?: Page; prominent: boolean }) {
  const { t, i18n } = useTranslation('dealer');
  const links = useLinks();
  const rows = boardRows(page?.items ?? [], group);
  const total = page?.total ?? 0;
  const filter = listFilterOf(group);
  // `status=all` resets a filter left over from an earlier "see all".
  const seeAll = `/bayi/services?status=${filter ?? 'all'}` as Href;

  return (
    <View className="gap-2">
      <View className="flex-row items-center gap-2 px-1">
        <Text variant={prominent ? 'headline' : 'callout'} className={cn(!prominent && 'font-semibold')} numberOfLines={1}>
          {t(`board_${group.key}`)}
        </Text>
        <View className={cn('rounded-full px-2 py-0.5', prominent && total > 0 ? 'bg-primary/10' : 'bg-secondary')}>
          <Text variant="footnote" tone={prominent && total > 0 ? 'primary' : 'muted'} className="font-semibold tabular-nums">
            {formatNumber(total, i18n.language)}
          </Text>
        </View>
        <View className="flex-1" />
        {total > 0 ? (
          <Pressable accessibilityRole="button" hitSlop={8} onPress={() => router.push(seeAll)}>
            <Text variant="footnote" tone="primary" className="font-semibold">
              {t('see_all')}
            </Text>
          </Pressable>
        ) : null}
      </View>
      {rows.length === 0 ? (
        <Text variant="footnote" tone="muted" className="px-1">
          {t(`board_empty_${group.key}`)}
        </Text>
      ) : (
        rows.map((s, i) => (
          <BoardRow
            key={s.uuid}
            s={s}
            first={prominent && i === 0}
            showStatus={s.status === 'ready'}
            date={group.key === 'done' ? (s.completed_at ?? s.created_at) : s.created_at}
            onPress={() => router.push(links.service(s.uuid))}
          />
        ))
      )}
    </View>
  );
}

/** Board row: brand logo, car and plate, product categories, customer and date. */
function BoardRow({
  s,
  first,
  showStatus,
  date,
  onPress,
}: {
  s: Service;
  first: boolean;
  showStatus: boolean;
  date: string;
  onPress: () => void;
}) {
  const { t, i18n } = useTranslation('dealer');
  const car = vehicleLabel(s.car_brand?.name, s.car_model?.name, s.model_year) || s.service_no;
  const tags = fitTags(uniqueCategories(s));
  const meta = [fullName(s.customer), formatDate(date, i18n.language)].filter(Boolean).join(' · ');

  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className={cn(
        'flex-row items-center gap-3 rounded-2xl border bg-card px-3.5 py-3 active:opacity-80',
        first ? 'border-primary/40' : 'border-border',
      )}
      style={{ borderCurve: 'continuous' }}>
      <BrandMark uuid={s.car_brand?.uuid} name={s.car_brand?.name} size={40} />
      <View className="min-w-0 flex-1 gap-1.5">
        <View className="flex-row items-center gap-2">
          <Text variant="headline" numberOfLines={1} className="min-w-0 flex-1">
            {car}
          </Text>
          {showStatus ? <StatusPill label={t(`service_status.${s.status}`)} tone={serviceTone(s.status)} /> : null}
        </View>
        {s.plate || tags.shown.length > 0 ? (
          <View className="flex-row flex-wrap items-center gap-1">
            {s.plate ? <PlateBadge plate={s.plate} country={s.plate_country} /> : null}
            {tags.shown.map((name) => (
              <View key={name} className="max-w-full rounded-full bg-secondary px-2 py-0.5">
                <Text variant="footnote" className="text-[12px] leading-[16px]" numberOfLines={1}>
                  {name}
                </Text>
              </View>
            ))}
            {tags.more > 0 ? (
              <View className="rounded-full bg-secondary px-2 py-0.5">
                <Text variant="footnote" tone="muted" className="text-[12px] leading-[16px] tabular-nums">
                  +{formatNumber(tags.more, i18n.language)}
                </Text>
              </View>
            ) : null}
          </View>
        ) : null}
        {meta ? (
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {meta}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
}

/**
 * Home of the `bayi` workplace: the center (which runs its own service point
 * since TEC-1003) gets the network dashboard with service intake on top
 * (TEC-1016); a dealer gets its own home.
 */
export function WorkplaceHome() {
  const center = useSession((s) => isCenterWorkplace(s.me));
  return center ? <AdminHomeScreen /> : <DealerHome />;
}
