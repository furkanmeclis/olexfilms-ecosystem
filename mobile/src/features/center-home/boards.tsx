import { useQuery } from '@tanstack/react-query';
import { type Href, router } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { usePricesVisible } from '@/core/use-prices-visible';
import { ErrorState, ListSkeleton, useFormat } from '@/features/admin/kit';
import { NEW_ORDER, useCanOrder } from '@/features/catalog/catalog-screen';
import { orderTone, type Order } from '@/features/orders/orders-screen';
import { type Period, useReport } from '@/features/reports/api';
import { BarList, TimeChart } from '@/features/reports/report-view';
import { useLinks } from '@/features/services/lib';
import { ServiceRow } from '@/features/services/services-list-screen';
import { Button } from '@/ui/button';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { dealersWithoutOrders, periodStart, RANK_MAX_PAGES, RANK_PAGE, type RankRow, topBuyers, topOpeners } from './lib';

type Paged<T> = { items: T[]; total: number };

/**
 * Every row of a period, at most RANK_MAX_PAGES pages of 100; `partial`
 * tells when the period holds more (the ranking then covers the newest rows).
 */
async function readPages<T>(page: (offset: number) => Promise<Paged<T>>): Promise<{ items: T[]; partial: boolean }> {
  const items: T[] = [];
  let total = 0;
  for (let i = 0; i < RANK_MAX_PAGES; i++) {
    const r = await page(i * RANK_PAGE);
    items.push(...r.items);
    total = r.total;
    if (items.length >= r.total || r.items.length === 0) break;
  }
  return { items, partial: items.length < total };
}

function Header({ title }: { title: string }) {
  return (
    <View className="-mx-4">
      <SectionHeader title={title} />
    </View>
  );
}

function Empty({ text }: { text: string }) {
  return (
    <Text variant="footnote" tone="muted" className="px-1">
      {text}
    </Text>
  );
}

/** A ranking of organizations: name, count and (orders) amounts. */
function RankList({ rows, unit, money }: { rows: RankRow[]; unit: 'orders' | 'services'; money: boolean }) {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const max = Math.max(1, ...rows.map((r) => r.count));
  return (
    <View className="gap-3 rounded-lg border border-border bg-card p-3.5">
      {rows.map((r, i) => {
        const amounts = money ? Object.entries(r.totals).map(([cur, v]) => f.money(v, cur)) : [];
        return (
          <View key={r.uuid} className="gap-1">
            <View className="flex-row items-baseline justify-between gap-2">
              <Text variant="footnote" className="flex-1" numberOfLines={1}>
                {`${f.number(i + 1)}. ${r.name}`}
              </Text>
              <Text variant="footnote" className="font-semibold tabular-nums">
                {t(unit === 'orders' ? 'home.orders_count' : 'home.services_count', { count: r.count })}
              </Text>
            </View>
            {amounts.length ? (
              <Text variant="footnote" tone="muted" className="tabular-nums" numberOfLines={1}>
                {amounts.join(' · ')}
              </Text>
            ) : null}
            <View className="h-2 overflow-hidden rounded-full bg-muted">
              <View className="h-2 rounded-full bg-primary" style={{ width: `${(r.count / max) * 100}%` }} />
            </View>
          </View>
        );
      })}
    </View>
  );
}

// ------------------------------------------------------------------- sales

/**
 * Sales board of the center sales staff (TEC-1068): quick order, the order
 * chart of the period (orders.trend), the dealers that ordered the most, the
 * active dealers without an order in the period, the latest orders with
 * their status. Amounts only with price visibility (TEC-1051).
 */
export function SalesBoard({ period, ordersBase }: { period: Period; ordersBase: string }) {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const prices = usePricesVisible();
  const canOrder = useCanOrder();
  const from = periodStart(period);

  const trend = useReport('orders.trend', { period });
  const created = trend.data?.series.find((s) => s.key === 'created');

  const sold = useQuery({
    queryKey: ['center-home', 'orders', from],
    queryFn: () =>
      readPages<Order>((offset) =>
        unwrap(
          api.GET('/v1/orders', {
            params: { query: { side: 'seller', created_from: from, sort: '-created_at', limit: RANK_PAGE, offset } },
          }),
        ),
      ),
  });
  const dealers = useQuery({
    queryKey: ['center-home', 'dealers'],
    queryFn: () => unwrap(api.GET('/v1/tenant/organizations', { params: { query: { type: 'dealer', limit: 200 } } })),
    staleTime: 5 * 60_000,
  });
  const recent = useQuery({
    queryKey: ['center-home', 'orders', 'recent'],
    queryFn: () => unwrap(api.GET('/v1/orders', { params: { query: { side: 'seller', sort: '-created_at', limit: 5, offset: 0 } } })),
  });

  const orders = sold.data?.items ?? [];
  const top = topBuyers(orders);
  const idle = dealers.data ? dealersWithoutOrders(dealers.data.items, orders) : [];
  const IDLE_SHOWN = 8;

  return (
    <View className="gap-3">
      {canOrder ? <Button title={t('home.quick_order')} icon="add" size="lg" onPress={() => router.push(NEW_ORDER as Href)} /> : null}

      {created ? <TimeChart series={created} /> : trend.isPending ? <ListSkeleton rows={1} inset={false} /> : null}

      <Header title={t('home.top_buyers')} />
      {sold.isPending ? (
        <ListSkeleton rows={3} inset={false} />
      ) : sold.isError ? (
        <ErrorState error={sold.error} onRetry={() => void sold.refetch()} />
      ) : top.length ? (
        <RankList rows={top} unit="orders" money={prices} />
      ) : (
        <Empty text={t('home.empty_period')} />
      )}
      {sold.data?.partial ? <Empty text={t('home.rank_partial', { count: RANK_PAGE * RANK_MAX_PAGES })} /> : null}

      <Header title={t('home.no_orders')} />
      {sold.isPending || dealers.isPending ? (
        <ListSkeleton rows={3} inset={false} />
      ) : dealers.isError ? (
        <ErrorState error={dealers.error} onRetry={() => void dealers.refetch()} />
      ) : idle.length ? (
        <List>
          {idle.slice(0, IDLE_SHOWN).map((d, i, shown) => (
            <ListRow
              key={d.uuid}
              icon="building"
              title={d.name}
              last={i === shown.length - 1 && idle.length <= IDLE_SHOWN}
              onPress={() => router.push(`/bayi/menu/network/${d.uuid}` as Href)}
            />
          ))}
          {idle.length > IDLE_SHOWN ? (
            <ListRow title={t('home.more_dealers', { count: idle.length - IDLE_SHOWN })} last onPress={() => router.push('/bayi/menu/network' as Href)} />
          ) : null}
        </List>
      ) : (
        <Empty text={t('home.no_orders_none')} />
      )}

      <Header title={t('home.recent_orders')} />
      {recent.isPending ? (
        <ListSkeleton rows={3} inset={false} />
      ) : recent.isError ? (
        <ErrorState error={recent.error} onRetry={() => void recent.refetch()} />
      ) : recent.data.items.length ? (
        <List>
          {recent.data.items.map((o, i, all) => (
            <Pressable
              key={o.uuid}
              accessibilityRole="button"
              onPress={() => router.push(`${ordersBase}/${o.uuid}` as Href)}
              className={`flex-row items-center gap-3 px-4 py-3 active:bg-muted ${i < all.length - 1 ? 'border-b border-border' : ''}`}>
              <View className="min-w-0 flex-1 gap-0.5">
                <Text variant="body" numberOfLines={1}>
                  {o.buyer.name}
                </Text>
                <Text variant="footnote" tone="muted" numberOfLines={1}>
                  {[o.order_no, prices && o.total !== null && o.total !== undefined ? f.money(o.total, o.currency) : null, f.date(o.created_at)]
                    .filter(Boolean)
                    .join(' · ')}
                </Text>
              </View>
              <StatusPill label={o.status_label} tone={orderTone(o.status)} />
            </Pressable>
          ))}
        </List>
      ) : (
        <Empty text={t('orders.empty')} />
      )}
      <Button title={t('home.see_all_orders')} variant="ghost" onPress={() => router.push(ordersBase as Href)} />
    </View>
  );
}

// ---------------------------------------------------------------- services

/**
 * Services board of the center (TEC-1073): the latest services (plate first,
 * service number and the dealer that opened it), and the period's most used
 * products, brands, models and the dealers that opened the most services.
 */
export function ServicesBoard({ period }: { period: Period }) {
  const { t } = useTranslation('admin');
  const links = useLinks();
  const from = periodStart(period);

  const recent = useQuery({
    queryKey: ['center-home', 'services', 'recent'],
    queryFn: () => unwrap(api.GET('/v1/services', { params: { query: { sort: '-created_at', limit: 5, offset: 0 } } })),
  });
  const products = useReport('services.top_products', { period, limit: 5 });
  const brands = useReport('services.top_brands', { period, limit: 5 });
  const models = useReport('services.top_models', { period, limit: 5 });
  const opened = useQuery({
    queryKey: ['center-home', 'services', from],
    queryFn: () =>
      readPages((offset) =>
        unwrap(api.GET('/v1/services', { params: { query: { created_from: from, sort: '-created_at', limit: RANK_PAGE, offset } } })),
      ),
  });
  const openers = topOpeners(opened.data?.items ?? []);

  const rankings = [
    { key: 'products', title: t('home.top_products'), q: products },
    { key: 'brands', title: t('home.top_brands'), q: brands },
    { key: 'models', title: t('home.top_models'), q: models },
  ];

  return (
    <View className="gap-3">
      <Header title={t('home.recent_services')} />
      {recent.isPending ? (
        <ListSkeleton rows={3} inset={false} />
      ) : recent.isError ? (
        <ErrorState error={recent.error} onRetry={() => void recent.refetch()} />
      ) : recent.data.items.length ? (
        <View className="gap-2.5">
          {recent.data.items.map((s) => (
            <ServiceRow key={s.uuid} s={s} onPress={() => router.push(links.service(s.uuid))} />
          ))}
        </View>
      ) : (
        <Empty text={t('home.empty_period')} />
      )}
      <Button title={t('home.see_all_services')} variant="ghost" onPress={() => router.push('/bayi/services?status=all' as Href)} />

      {rankings.map(({ key, title, q }) => {
        const series = q.data?.series[0];
        return (
          <View key={key} className="gap-3">
            <Header title={title} />
            {q.isPending ? (
              <ListSkeleton rows={2} inset={false} />
            ) : q.isError ? (
              <ErrorState error={q.error} onRetry={() => void q.refetch()} />
            ) : series && series.points.length ? (
              <BarList series={series} hideLabel />
            ) : (
              <Empty text={t('home.empty_period')} />
            )}
          </View>
        );
      })}

      <Header title={t('home.top_dealers')} />
      {opened.isPending ? (
        <ListSkeleton rows={3} inset={false} />
      ) : opened.isError ? (
        <ErrorState error={opened.error} onRetry={() => void opened.refetch()} />
      ) : openers.length ? (
        <RankList rows={openers} unit="services" money={false} />
      ) : (
        <Empty text={t('home.empty_period')} />
      )}
      {opened.data?.partial ? <Empty text={t('home.rank_partial', { count: RANK_PAGE * RANK_MAX_PAGES })} /> : null}
    </View>
  );
}
