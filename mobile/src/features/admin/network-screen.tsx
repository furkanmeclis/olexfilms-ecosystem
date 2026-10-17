import { useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Linking, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { paths } from '@/core/api/schema';
import { Building2 } from '@/ui/icons';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

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
          query: { type: filter === 'all' ? undefined : filter, q: query || undefined, limit: PAGE_SIZE, offset },
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
        empty={{ icon: Building2, title: query ? t('network.no_results') : t('network.empty') }}
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
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
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
                    {t(`spaces:org_type.${o.type}`)} · {o.slug}
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
              <InfoRow label={t('network.parent')} value={o.parent?.name} />
              <InfoRow label={t('network.plan')} value={o.plan_code} />
              <InfoRow label={t('network.access_starts')} value={f.date(o.access_starts_at)} />
              <InfoRow label={t('network.access_ends')} value={o.access_ends_at ? f.date(o.access_ends_at) : null} />
              <InfoRow label={t('network.contract_until')} value={o.contract_valid_until ? f.date(o.contract_valid_until) : null} />
              <InfoRow label={t('network.created')} value={f.date(o.created_at)} last />
            </List>
          </>
        )}
      </Screen>
    </>
  );
}
