import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { formatDate, vehicleLabel } from '@/features/services/lib';
import { ListBody, ListFooter } from '@/features/services/ui';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { portalFiles, useDirectDownload } from './files';
import { PAGE_SIZE, usePortalPages } from './lib';

type Contract = components['schemas']['PortalContract'];

function ContractRow({ c, busy, onOpen }: { c: Contract; busy: boolean; onOpen: () => void }) {
  const { t, i18n } = useTranslation('customer');
  const vehicle = [c.plate, vehicleLabel(c.car_brand_name, c.car_model_name, c.model_year)].filter(Boolean).join(' · ');
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('contract_open')}
      disabled={!c.pdf_ready || busy}
      onPress={onOpen}
      className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
      <View className="flex-row items-center gap-3">
        <View className="size-10 items-center justify-center rounded-md bg-secondary">
          <Icon name="report" tone="primary" size={20} />
        </View>
        <View className="flex-1 gap-0.5">
          <Text variant="headline" numberOfLines={1}>
            {t('contract_no', { no: c.contract_no })}
          </Text>
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {[c.organization?.name, formatDate(c.executed_at ?? c.created_at, i18n.language)].filter(Boolean).join(' · ')}
          </Text>
        </View>
        {busy ? (
          <ActivityIndicator />
        ) : c.pdf_ready ? (
          <Icon name="share" tone="primary" size={20} />
        ) : (
          <Text variant="footnote" tone="muted">
            {t('contract_not_ready')}
          </Text>
        )}
      </View>
      {vehicle ? (
        <Text variant="footnote" tone="muted" numberOfLines={1}>
          {vehicle}
        </Text>
      ) : null}
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {t('contract_service', { no: c.service.service_no })}
      </Text>
    </Pressable>
  );
}

/** Executed intake contracts (GET /v1/portal/contracts) with their PDF (shared through the system sheet). */
export function CustomerContractsScreen() {
  const { t } = useTranslation('customer');
  const list = usePortalPages(['portal', 'contracts'], (offset) =>
    unwrap(api.GET('/v1/portal/contracts', { params: { query: { limit: PAGE_SIZE, offset } } })),
  );
  const download = useDirectDownload();
  const busyUuid = download.isPending ? download.variables?.path : undefined;
  return (
    <>
      <Stack.Screen options={{ title: t('contracts') }} />
      <FlatList
        data={list.items}
        keyExtractor={(c) => c.contract_uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10 pt-2"
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{ icon: 'report', title: t('contracts_empty'), body: t('contracts_empty_body') }}
          />
        }
        renderItem={({ item }) => {
          const path = portalFiles.contractPath(item.contract_uuid);
          return (
            <ContractRow
              c={item}
              busy={busyUuid === path}
              onOpen={() => download.mutate({ path, filename: `contract-${item.contract_no}.pdf` })}
            />
          );
        }}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
        }}
        ListFooterComponent={<ListFooter loading={list.isFetchingNextPage} />}
        refreshControl={
          <RefreshControl refreshing={list.isRefetching && !list.isFetchingNextPage} onRefresh={() => void list.refetch()} />
        }
      />
    </>
  );
}
