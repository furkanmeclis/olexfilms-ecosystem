import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { paths } from '@/core/api/schema';
import { useCan } from '@/core/spaces';
import { CardRow, Chips, PagedList, PAGE_SIZE, useChildHref, useDebounced, useFormat, usePagedQuery } from '@/features/admin/kit';
import { EmptyState } from '@/ui/empty-state';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';

type Measurement =
  paths['/v1/measurements']['get']['responses'][200]['content']['application/json']['data']['items'][number];
type Status = 'all' | 'accepted' | 'vin_pending';

/**
 * Measurements of the whole read scope (center: the network, distributor: its
 * dealers) with the dealer on every row. Read only; a row opens the same
 * measurement detail the dealer sees (features/measure).
 */
export function AdminMeasurementsScreen() {
  const { t } = useTranslation('governance');
  const { t: tm } = useTranslation('measure');
  const f = useFormat();
  const href = useChildHref();
  const canRead = useCan('measurements.read');
  const [q, setQ] = useState('');
  const [status, setStatus] = useState<Status>('all');
  const term = useDebounced(q.trim());

  const list = usePagedQuery<Measurement>(
    ['measurements', 'admin-list', status, term],
    (offset) =>
      unwrap(
        api.GET('/v1/measurements', {
          params: {
            query: { limit: PAGE_SIZE, offset, q: term || undefined, status: status === 'all' ? undefined : status },
          },
        }),
      ),
    canRead,
  );

  const header = (
    <View>
      <Chips
        options={[
          { key: 'all', label: t('measurements.all') },
          { key: 'accepted', label: tm('status.accepted') },
          { key: 'vin_pending', label: tm('status.vin_pending') },
        ]}
        value={status}
        onChange={setStatus}
      />
      <View className="px-4 pb-1">
        <Input
          label={t('measurements.search')}
          placeholder={t('measurements.search_placeholder')}
          value={q}
          onChangeText={setQ}
          autoCapitalize="characters"
          autoCorrect={false}
          returnKeyType="search"
        />
      </View>
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: t('measurements.title') }} />
      {canRead ? (
        <PagedList
          query={list}
          keyOf={(m) => m.uuid}
          header={header}
          empty={{ icon: 'measure', title: t('measurements.empty') }}
          renderItem={({ item }) => (
            <CardRow
              title={item.plate || item.vin || tm('no_vehicle')}
              subtitle={[item.organization.name, item.plate && item.vin ? item.vin : null, item.service ? tm('service_no', { no: item.service.service_no }) : null]
                .filter(Boolean)
                .join(' · ')}
              meta={`${f.dateTime(item.measured_at ?? item.created_at)}${item.device_serial ? ` · ${item.device_serial}` : ''}`}
              trailing={<StatusPill label={tm(`status.${item.status}`)} tone={item.status === 'accepted' ? 'done' : 'progress'} />}
              onPress={() => router.push(href(item.uuid))}
            />
          )}
        />
      ) : (
        <EmptyState icon="blocked" title={t('no_access')} />
      )}
    </>
  );
}
