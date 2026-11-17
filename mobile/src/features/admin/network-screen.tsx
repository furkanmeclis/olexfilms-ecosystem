import { useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Linking, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { paths } from '@/core/api/schema';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { PullRefresh } from '@/ui/pull-refresh';

import { RefRow } from '@/features/entity/ref-row';
import { RelatedList } from '@/features/entity/related-list';
import { useCan } from '@/core/spaces';
import { useModuleFilter } from '@/core/modules';
import { fullName, vehicleLabel } from '@/features/services/lib';

import { CardRow, Chips, ErrorState, InfoRow, ListSkeleton, PagedList, PAGE_SIZE, useChildHref, useDebounced, useFormat, usePagedQuery } from './kit';

type OrgType = 'center' | 'distributor' | 'dealer';
type Org =
  paths['/v1/tenant/organizations']['get']['responses'][200]['content']['application/json']['data']['items'][number];
type Filter = 'all' | OrgType;

const FILTERS: Filter[] = ['all', 'distributor', 'dealer'];

export function orgStatusTone(status: string): StatusTone {
  if (status === 'active') return 'done';
  if (status === 'suspended' || status === 'expired') return 'problem';
  if (status === 'pending') return 'progress';
  return 'neutral';
}

/**
 * Dealers and distributors inside the caller's organizations.read scope. The
 * list endpoint returns items without a total, so pages stop on a short page.
 */
export function NetworkScreen() {
  const { t } = useTranslation('admin');
  const href = useChildHref();
  const [filter, setFilter] = useState<Filter>('all');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedQuery<Org>(['network', 'list', filter, query], async (offset) => {
    const data = await unwrap(
      api.GET('/v1/tenant/organizations', {
        params: {
          // TEC-1062: a network list, not a picker; pending (pool) rows stay with their status.
          query: {
            type: filter === 'all' ? undefined : filter,
            q: query || undefined,
            limit: PAGE_SIZE,
            offset,
            include_pending: true,
          },
        },
      }),
    );
    // No total in this response: a full page means there may be more.
    const more = data.items.length === PAGE_SIZE;
    return { items: data.items, limit: PAGE_SIZE, offset, total: offset + data.items.length + (more ? 1 : 0) };
  });

  return (
    <>
      <Stack.Screen
        options={{
          title: t('network.title'),
          headerSearchBarOptions: {
            placeholder: t('network.search'),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(''),
          },
        }}
      />
      <PagedList
        query={list}
        keyOf={(o) => o.uuid}
        header={
          <Chips
            options={FILTERS.map((f) => ({ key: f, label: t(`network.filter.${f}`) }))}
            value={filter}
            onChange={setFilter}
          />
        }
        empty={{ icon: 'building', title: query ? t('network.no_results') : t('network.empty') }}
        renderItem={({ item }) => (
          <CardRow
            title={item.name}
            subtitle={[item.type ? t(`spaces:org_type.${item.type}`) : null, [item.district, item.city].filter(Boolean).join(', ')]
              .filter(Boolean)
              .join(' · ')}
            meta={item.parent ? t('network.parent_of', { name: item.parent.name }) : undefined}
            trailing={<StatusPill label={t(`network.status.${item.status}`, { defaultValue: item.status })} tone={orgStatusTone(item.status)} />}
            onPress={() => router.push(href(item.uuid))}
          />
        )}
      />
    </>
  );
}

export function NetworkDetailScreen() {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const q = useQuery({
    queryKey: ['network', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/tenant/organizations/{uuid}', { params: { path: { uuid: uuid! } } })),
    enabled: !!uuid,
  });
  const o = q.data;

  return (
    <>
      <Stack.Screen options={{ title: o?.name ?? '', headerLargeTitle: false }} />
      <Screen refreshControl={<PullRefresh busy={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : !o ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <View className="gap-2">
              <Text variant="title">{o.name}</Text>
              <View className="flex-row flex-wrap items-center gap-2">
                <StatusPill label={t(`network.status.${o.status}`, { defaultValue: o.status })} tone={orgStatusTone(o.status)} />
                {o.type ? (
                  <Text variant="footnote" tone="muted">
                    {t(`spaces:org_type.${o.type}`)}
                  </Text>
                ) : null}
              </View>
            </View>

            <SectionHeader title={t('network.contact')} />
            <List>
              {o.phone ? (
                <ListRow title={o.phone} subtitle={t('network.phone')} onPress={() => void Linking.openURL(`tel:${o.phone}`)} />
              ) : (
                <InfoRow label={t('network.phone')} value={null} />
              )}
              <InfoRow label={t('network.city')} value={[o.district, o.city].filter(Boolean).join(', ')} />
              <InfoRow label={t('network.address')} value={o.address} last />
            </List>

            <SectionHeader title={t('network.details')} />
            <List>
              {o.parent ? <RefRow kind="organization" label={t('network.parent')} value={o.parent} /> : null}
              {/* plan_code is a free-text code with no readable name anywhere (web edits it as text): not shown. */}
              <InfoRow label={t('network.access_starts')} value={f.date(o.access_starts_at)} />
              <InfoRow label={t('network.access_ends')} value={o.access_ends_at ? f.date(o.access_ends_at) : null} />
              <InfoRow label={t('network.contract_until')} value={o.contract_valid_until ? f.date(o.contract_valid_until) : null} />
              <InfoRow label={t('network.created')} value={f.date(o.created_at)} last />
            </List>

            <NetworkRelated orgUuid={o.uuid} />
          </>
        )}
      </Screen>
    </>
  );
}

/**
 * Reverse lists of a network organization (TEC-1219): its latest orders,
 * services and warranties, each behind its permission and module.
 */
function NetworkRelated({ orgUuid }: { orgUuid: string }) {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const on = useModuleFilter();
  const canOrders = useCan('orders.read') && on('orders');
  const canServices = useCan('services.read');
  const canWarranties = useCan('warranties.read');
  return (
    <>
      <RelatedList
        kind="order"
        title={t('network.related_orders')}
        enabled={canOrders}
        queryKey={['orders', 'list', { organization: orgUuid }]}
        fetch={async (limit) => {
          const r = await unwrap(
            api.GET('/v1/orders', { params: { query: { organization_uuid: [orgUuid], limit, offset: 0 } } }),
          );
          return {
            total: r.total,
            items: r.items.map((x) => ({ uuid: x.uuid, title: x.order_no, subtitle: `${x.status_label} · ${f.date(x.created_at)}` })),
          };
        }}
      />
      <RelatedList
        kind="service"
        title={t('network.related_services')}
        enabled={canServices}
        queryKey={['services', 'list', { organization: orgUuid }]}
        fetch={async (limit) => {
          const r = await unwrap(
            api.GET('/v1/services', { params: { query: { organization_uuid: orgUuid, sort: '-created_at', limit, offset: 0 } } }),
          );
          return {
            total: r.total,
            items: r.items.map((x) => ({
              uuid: x.uuid,
              title: [x.plate, x.service_no].filter(Boolean).join(' · '),
              subtitle: [fullName(x.customer), x.status_label].filter(Boolean).join(' · '),
            })),
          };
        }}
      />
      <RelatedList
        kind="warranty"
        title={t('network.related_warranties')}
        enabled={canWarranties}
        queryKey={['warranties', 'list', { organization: orgUuid }]}
        fetch={async (limit) => {
          const r = await unwrap(
            api.GET('/v1/warranties', { params: { query: { organization_uuid: orgUuid, limit, offset: 0 } } }),
          );
          return {
            total: r.total,
            items: r.items.map((x) => ({
              uuid: x.uuid,
              title: x.public_code,
              subtitle: [x.product.name, x.vehicle.plate ?? vehicleLabel(x.vehicle.brand_name, x.vehicle.model_name, x.vehicle.model_year)]
                .filter(Boolean)
                .join(' · '),
            })),
          };
        }}
      />
    </>
  );
}
