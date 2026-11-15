import { useQuery } from '@tanstack/react-query';
import { type Href, router } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { usePricesVisible } from '@/core/use-prices-visible';
import { ErrorState, ListSkeleton, useFormat } from '@/features/admin/kit';
import { ChartCard, RankBars } from '@/features/analytics-kit/charts';
import { ChartEmpty, ChartError, ChartSkeleton } from '@/features/analytics-kit/states';
import type { PeriodState } from '@/features/analytics-kit/period';
import { num, type Row } from '@/features/analytics-kit/table';
import { NEW_ORDER, useCanOrder } from '@/features/catalog/catalog-screen';
import { orderTone } from '@/features/orders/orders-screen';
import { useCatalogReport } from '@/features/reports/api';
import { RankSection, TrendSection, useRowPress } from '@/features/reports/sections';
import { useLinks } from '@/features/services/lib';
import { ServiceRow } from '@/features/services/services-list-screen';
import { Button } from '@/ui/button';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

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

const str = (v: unknown) => (typeof v === 'string' && v ? v : null);

/**
 * Active dealers without an order in the period (orders.inactive_buyers,
 * TEC-1090): never ordered first, then the oldest last order. The card's
 * total is the `inactive_dealers` card; rows are capped by `limit`.
 */
function InactiveBuyers({ period }: { period: PeriodState }) {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const press = useRowPress();
  const SHOWN = 8;
  const r = useCatalogReport('orders.inactive_buyers', period, { limit: SHOWN, granularity: false });
  if (!r.available && !r.loading) return null;
  const data = r.query.data;
  const count = data?.series.find((s) => s.key === 'cards')?.points.find((p) => p.key === 'inactive_dealers')?.value;
  const rows: Row[] = data?.rows ?? [];

  return (
    <ChartCard title={t('home.no_orders')} trailing={count !== undefined ? <Text variant="footnote" tone="muted">{f.number(count)}</Text> : undefined}>
      {r.loading && !data ? (
        <ChartSkeleton height={120} />
      ) : r.query.isError && !data ? (
        <ChartError error={r.query.error} onRetry={() => void r.query.refetch()} />
      ) : rows.length ? (
        <View className="-mx-3.5 -mb-3.5">
          <List>
            {rows.slice(0, SHOWN).map((row, i, shown) => {
              const last = str(row.last_order_at);
              return (
                <ListRow
                  key={str(row.organization_uuid) ?? String(i)}
                  icon="building"
                  title={str(row.organization) ?? '–'}
                  subtitle={[str(row.city), last ? t('home.last_order', { date: f.date(last) }) : t('home.never_ordered')].filter(Boolean).join(' · ')}
                  last={i === shown.length - 1 && (count ?? rows.length) <= SHOWN}
                  onPress={press(row)}
                />
              );
            })}
            {(count ?? 0) > SHOWN ? (
              <ListRow title={t('home.more_dealers', { count: (count ?? 0) - SHOWN })} last onPress={() => router.push('/bayi/menu/network' as Href)} />
            ) : null}
          </List>
        </View>
      ) : (
        <ChartEmpty height={60} text={t('home.no_orders_none')} />
      )}
    </ChartCard>
  );
}

// ------------------------------------------------------------------- sales

/**
 * Sales board of the center sales staff (TEC-1068, TEC-1213): quick order,
 * orders over time against the previous period (orders.trend), the dealers
 * that ordered the most (orders.by_buyer), the active dealers without an
 * order (orders.inactive_buyers) and the latest orders. Amounts only with
 * price visibility (the API leaves them out otherwise, TEC-1051).
 */
export function SalesBoard({ period, ordersBase }: { period: PeriodState; ordersBase: string }) {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const prices = usePricesVisible();
  const canOrder = useCanOrder();

  const trend = useCatalogReport('orders.trend', period);
  const buyers = useCatalogReport('orders.by_buyer', period, { limit: 5 });
  const recent = useQuery({
    queryKey: ['center-home', 'orders', 'recent'],
    queryFn: () => unwrap(api.GET('/v1/orders', { params: { query: { side: 'seller', sort: '-created_at', limit: 5, offset: 0 } } })),
  });

  return (
    <View className="gap-3">
      {canOrder ? <Button title={t('home.quick_order')} icon="add" size="lg" onPress={() => router.push(NEW_ORDER as Href)} /> : null}

      <TrendSection r={trend} defaultMode="bar" />

      <RankSection
        r={buyers}
        title={t('home.top_buyers')}
        toItem={(row) => {
          const count = num(row.order_count) ?? 0;
          const total = prices ? num(row.total) : null;
          const cur = str(row.currency);
          return {
            key: str(row.organization_uuid) ?? String(row.organization),
            label: str(row.organization) ?? '–',
            value: count,
            valueText: t('home.orders_count', { count: f.number(count) }),
            sub: [total !== null && cur ? f.money(total, cur) : null, str(row.city)].filter(Boolean).join(' · ') || null,
          };
        }}
      />

      <InactiveBuyers period={period} />

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
 * Services board of the center (TEC-1073, TEC-1213): the latest services,
 * services over time against the previous period, the period's most used
 * products, brands and models, and services by dealer (services.by_org; the
 * center's own service point is its own row).
 */
export function ServicesBoard({ period }: { period: PeriodState }) {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const links = useLinks();

  const recent = useQuery({
    queryKey: ['center-home', 'services', 'recent'],
    queryFn: () => unwrap(api.GET('/v1/services', { params: { query: { sort: '-created_at', limit: 5, offset: 0 } } })),
  });
  const products = useCatalogReport('services.top_products', period, { limit: 5 });
  const brands = useCatalogReport('services.top_brands', period, { limit: 5 });
  const models = useCatalogReport('services.top_models', period, { limit: 5 });
  const byOrg = useCatalogReport('services.by_org', period, { limit: 5 });

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

      <RankSection
        r={byOrg}
        title={t('home.top_dealers')}
        toItem={(row) => {
          const count = num(row.services_count) ?? 0;
          return {
            key: `${str(row.organization_uuid) ?? row.organization}${row.is_service_point ? ':sp' : ''}`,
            label: str(row.organization) ?? '–',
            value: count,
            valueText: t('home.services_count', { count: f.number(count) }),
            sub: str(row.city),
          };
        }}
      />

      {[
        { key: 'products', title: t('home.top_products'), r: products },
        { key: 'brands', title: t('home.top_brands'), r: brands },
        { key: 'models', title: t('home.top_models'), r: models },
      ].map(({ key, title, r }) => (
        <SeriesRank key={key} title={title} r={r} />
      ))}
    </View>
  );
}

/** A ranking report drawn from its first series (product, brand, model rankings). */
function SeriesRank({ title, r }: { title: string; r: ReturnType<typeof useCatalogReport> }) {
  const { t } = useTranslation('admin');
  if (!r.available && !r.loading) return null;
  const data = r.query.data;
  const s = data?.series.find((x) => x.key !== 'cards' && x.points.length);
  return (
    <ChartCard title={title}>
      {r.loading && !data ? (
        <ChartSkeleton height={100} />
      ) : r.query.isError && !data ? (
        <ChartError error={r.query.error} onRetry={() => void r.query.refetch()} />
      ) : s ? (
        <RankBars items={s.points.map((p) => ({ key: p.key, label: p.label, value: p.value }))} unit={s.unit} />
      ) : (
        <ChartEmpty height={60} text={t('home.empty_period')} />
      )}
    </ChartCard>
  );
}
