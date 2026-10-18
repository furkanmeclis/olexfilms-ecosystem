import { useQuery } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { formatDate } from '@/features/services/lib';
import { ListBody } from '@/features/services/ui';
import { Icon } from '@/ui/icon';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { retryPortal } from './lib';

type Claim = components['schemas']['PortalWarrantyClaimStatus'];

function claimTone(s: Claim['status']): StatusTone {
  if (s === 'approved' || s === 'closed') return 'done';
  if (s === 'rejected') return 'problem';
  return 'progress';
}

/** Warranty claims of the customer (GET /v1/portal/warranty-claims): status only, the API omits details and photos. */
export function CustomerClaimsScreen() {
  const { t, i18n } = useTranslation('customer');
  const q = useQuery({
    queryKey: ['portal', 'warranty-claims'],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/warranty-claims')),
  });
  const items = q.data?.items ?? [];
  return (
    <>
      <Stack.Screen options={{ title: t('claims') }} />
      <FlatList
        data={items}
        keyExtractor={(c) => c.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10 pt-2"
        ListEmptyComponent={
          <ListBody
            isPending={q.isPending}
            error={q.error}
            count={items.length}
            onRetry={() => void q.refetch()}
            empty={{ icon: 'warrantyClaim', title: t('claims_empty'), body: t('claims_empty_body') }}
          />
        }
        renderItem={({ item }) => (
          <Pressable
            accessibilityRole="button"
            disabled={!item.warranty_uuid}
            onPress={() => item.warranty_uuid && router.push(`/musteri/warranties/${encodeURIComponent(item.warranty_uuid)}`)}
            className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
            <View className="size-10 items-center justify-center rounded-md bg-secondary">
              <Icon name="warrantyClaim" tone="primary" size={20} />
            </View>
            <View className="flex-1 gap-0.5">
              <Text variant="callout" className="font-semibold" numberOfLines={1}>
                {t('claim_created', { date: formatDate(item.created_at, i18n.language) })}
              </Text>
              <Text variant="footnote" tone="muted" numberOfLines={1}>
                {t('claim_updated', { date: formatDate(item.updated_at, i18n.language) })}
              </Text>
            </View>
            <StatusPill label={t(`claim_status.${item.status}`)} tone={claimTone(item.status)} />
            {item.warranty_uuid ? <Icon name="forward" tone="muted-foreground" size={18} /> : null}
          </Pressable>
        )}
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}
      />
    </>
  );
}
