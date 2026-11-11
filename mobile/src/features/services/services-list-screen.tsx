import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { activeMembership, isCenterWorkplace, useCan } from '@/core/spaces';
import { readJSON, writeJSON } from '@/core/storage';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { PullRefresh } from '@/ui/pull-refresh';

import {
  formatDate,
  fullName,
  PAGE_SIZE,
  type Service,
  SERVICE_STATUSES,
  type ServiceStatus,
  serviceTone,
  useDebounced,
  useLinks,
  usePagedList,
  vehicleLabel,
} from './lib';
import { Segmented } from '@/features/warehouse/parts';

import { Chips, ListBody, ListFooter, SearchField } from './ui';

type Filter = 'all' | ServiceStatus;
type Scope = 'own' | 'network';
const SCOPE_KEY = 'services.scope';

/**
 * The center sees the whole network's services; "own" narrows the list to the
 * ones its service point performed (web: network-scope toggle, TEC-1003).
 */
function useNetworkScope() {
  const me = useSession((s) => s.me);
  const center = isCenterWorkplace(me);
  const [scope, setScope] = useState<Scope>(() => readJSON<Scope>(SCOPE_KEY) ?? 'own');
  return {
    center,
    scope,
    setScope: (next: Scope) => {
      writeJSON(SCOPE_KEY, next);
      setScope(next);
    },
    organizationUuid: center && scope === 'own' ? activeMembership(me)?.uuid : undefined,
  };
}

/**
 * Service card (TEC-1073): the plate is the title, the second line is the
 * service number and the organization that opened the service (shown when it
 * is not the viewer's own organization, i.e. a dealer seen by the center).
 */
export function ServiceRow({ s, onPress }: { s: Service; onPress: () => void }) {
  const { t, i18n } = useTranslation('dealer');
  const own = useSession((x) => x.me?.active_organization_uuid);
  const opener = s.organization && s.organization.uuid !== own ? s.organization.name : null;
  // Without a plate the number is already the title.
  const second = [s.plate ? s.service_no : null, opener].filter(Boolean).join(' · ');
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="headline" numberOfLines={1} className="flex-1">
          {s.plate || s.service_no}
        </Text>
        <StatusPill label={t(`service_status.${s.status}`)} tone={serviceTone(s.status)} />
      </View>
      {second ? (
        <Text variant="callout" numberOfLines={1}>
          {second}
        </Text>
      ) : null}
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {[vehicleLabel(s.car_brand?.name, s.car_model?.name, s.model_year), fullName(s.customer), formatDate(s.created_at, i18n.language)]
          .filter(Boolean)
          .join(' · ')}
      </Text>
    </Pressable>
  );
}

/** Services of the scope: search, status chips, infinite scroll. */
export function ServicesListScreen() {
  const { t } = useTranslation('dealer');
  const links = useLinks();
  const canWrite = useCan('services.write');
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState<Filter>('all');
  // `?status=` from a home "see all" link (`all` clears it); a changed param re-applies.
  const { status } = useLocalSearchParams<{ status?: string }>();
  const [appliedStatus, setAppliedStatus] = useState<string>();
  if (status !== appliedStatus) {
    setAppliedStatus(status);
    if (status === 'all' || (SERVICE_STATUSES as string[]).includes(status ?? '')) setFilter(status as Filter);
  }
  const query = useDebounced(q.trim());
  const network = useNetworkScope();

  const list = usePagedList(['services', 'list', { q: query, filter, org: network.organizationUuid }], (offset) =>
    unwrap(
      api.GET('/v1/services', {
        params: {
          query: {
            q: query || undefined,
            status: filter === 'all' ? undefined : filter,
            organization_uuid: network.organizationUuid,
            limit: PAGE_SIZE,
            offset,
          },
        },
      }),
    ),
  );

  const chips = [
    { value: 'all' as Filter, label: t('filter_all') },
    ...SERVICE_STATUSES.map((s) => ({ value: s as Filter, label: t(`service_status.${s}`) })),
  ];

  return (
    <>
      <Stack.Screen
        options={{
          title: t('services_title'),
          headerRight: canWrite
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('new_service')}
                  hitSlop={8}
                  onPress={() => router.push(links.newService)}
                  className="size-8 items-center justify-center">
                  <Icon name="add" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <FlatList
        data={list.items}
        keyExtractor={(s) => s.uuid}
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10"
        ListHeaderComponent={
          <View className="gap-3 pb-1 pt-2">
            <SearchField value={q} onChange={setQ} placeholder={t('services_search')} />
            {network.center ? (
              <Segmented
                options={[
                  { value: 'own' as Scope, label: t('scope_own') },
                  { value: 'network' as Scope, label: t('scope_network') },
                ]}
                value={network.scope}
                onChange={network.setScope}
              />
            ) : null}
            <Chips options={chips} value={filter} onChange={setFilter} />
            {list.total !== undefined && list.items.length > 0 ? (
              <Text variant="footnote" tone="muted">
                {t('count_results', { count: list.total })}
              </Text>
            ) : null}
          </View>
        }
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{
              icon: 'service',
              title: query || filter !== 'all' ? t('services_empty_filtered') : t('services_empty'),
              body: query || filter !== 'all' ? undefined : t('services_empty_body'),
              action:
                canWrite && !query && filter === 'all'
                  ? { title: t('new_service'), onPress: () => router.push(links.newService) }
                  : undefined,
            }}
          />
        }
        renderItem={({ item }) => <ServiceRow s={item} onPress={() => router.push(links.service(item.uuid))} />}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
        }}
        ListFooterComponent={<ListFooter loading={list.isFetchingNextPage} />}
        refreshControl={
          <PullRefresh busy={list.isRefetching && !list.isFetchingNextPage} onRefresh={() => void list.refetch()} />
        }
      />
    </>
  );
}
