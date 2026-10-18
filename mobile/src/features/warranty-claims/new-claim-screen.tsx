import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { formatDate, PAGE_SIZE, useDebounced, usePagedList, type Warranty } from '@/features/services/lib';
import { ListBody, ListFooter, SearchField, Section, errorText } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { useClaimLinks } from './hooks';
import { togglePart } from './lib';

const DESCRIPTION_MAX = 20000;

/** Picks the warranty the claim is opened on: active warranties of the scope. */
function WarrantyPicker({ onPick }: { onPick: (w: Warranty) => void }) {
  const { t, i18n } = useTranslation('claims');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const list = usePagedList(['warranties', 'claim-picker', { q: query }], (offset) =>
    unwrap(
      api.GET('/v1/warranties', { params: { query: { status: 'active', q: query || undefined, limit: PAGE_SIZE, offset } } }),
    ),
  );
  return (
    <FlatList
      data={list.items}
      keyExtractor={(w) => w.uuid}
      contentInsetAdjustmentBehavior="automatic"
      keyboardShouldPersistTaps="handled"
      keyboardDismissMode="on-drag"
      className="flex-1 bg-background"
      contentContainerClassName="gap-2 px-4 pb-10"
      ListHeaderComponent={
        <View className="gap-3 pb-1 pt-2">
          <Text variant="callout" tone="muted">
            {t('pick_warranty')}
          </Text>
          <SearchField value={q} onChange={setQ} placeholder={t('warranty_search')} autoFocus />
        </View>
      }
      ListEmptyComponent={
        <ListBody
          isPending={list.isPending}
          error={list.error}
          count={list.items.length}
          onRetry={() => void list.refetch()}
          empty={{ icon: 'warranty', title: t('warranties_empty') }}
        />
      }
      renderItem={({ item: w }) => (
        <Pressable
          accessibilityRole="button"
          onPress={() => onPick(w)}
          className="gap-0.5 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
          <Text variant="headline" numberOfLines={1}>
            {w.vehicle.plate || w.public_code}
          </Text>
          <Text variant="callout" numberOfLines={1}>
            {w.product.name}
          </Text>
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {[w.public_code, `${formatDate(w.start_at, i18n.language)} – ${formatDate(w.end_at, i18n.language)}`].join(' · ')}
          </Text>
        </Pressable>
      )}
      onEndReachedThreshold={0.5}
      onEndReached={() => {
        if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
      }}
      ListFooterComponent={<ListFooter loading={list.isFetchingNextPage} />}
      refreshControl={
        <RefreshControl refreshing={list.isRefetching && !list.isFetchingNextPage} onRefresh={() => void list.refetch()} />
      }
    />
  );
}

/**
 * Open a claim on a warranty (dealer / distributor): the warranted parts of the
 * service item, a description. The claim is created `open`; its photos and the
 * "submit for review" step are on the detail screen it lands on.
 */
export function NewClaimScreen() {
  const params = useLocalSearchParams<{ warranty_uuid?: string }>();
  const { t } = useTranslation('claims');
  const links = useClaimLinks();
  const qc = useQueryClient();
  const [picked, setPicked] = useState<Warranty | null>(null);
  const [parts, setParts] = useState<string[]>([]);
  const [description, setDescription] = useState('');

  const preset = useQuery({
    queryKey: ['warranties', 'detail', params.warranty_uuid],
    queryFn: () => unwrap(api.GET('/v1/warranties/{uuid}', { params: { path: { uuid: params.warranty_uuid as string } } })),
    enabled: !!params.warranty_uuid && !picked,
  });
  const warranty = picked ?? preset.data ?? null;

  const service = useQuery({
    queryKey: ['services', 'detail', warranty?.service.uuid],
    queryFn: () => unwrap(api.GET('/v1/services/{uuid}', { params: { path: { uuid: warranty?.service.uuid as string } } })),
    enabled: !!warranty,
  });
  const itemUuid = service.data?.warranties?.find((w) => w.uuid === warranty?.uuid)?.service_item_uuid ?? null;
  const item = service.data?.items?.find((i) => i.uuid === itemUuid);
  const available = item?.applied_parts ?? [];

  const create = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST('/v1/warranty-claims', {
          body: {
            warranty_uuid: (warranty as Warranty).uuid,
            description: description.trim(),
            parts: parts.map((part_key) => ({ part_key, service_item_uuid: itemUuid })),
          },
        }),
      ),
    onSuccess: (claim) => {
      void qc.invalidateQueries({ queryKey: ['warranty-claims'] });
      router.replace(links.detail(claim.uuid));
    },
    onError: (e) =>
      Alert.alert(t('action_failed'), isApiError(e, 'WARRANTY_CLAIM_EXISTS') ? t('claim_exists') : errorText(e)),
  });

  const ready = !!warranty && parts.length > 0 && description.trim() !== '';

  if (!warranty) {
    return (
      <>
        <Stack.Screen options={{ title: t('new') }} />
        {params.warranty_uuid && preset.isPending ? null : <WarrantyPicker onPick={setPicked} />}
      </>
    );
  }

  return (
    <>
      <Stack.Screen options={{ title: t('new') }} />
      <ScrollView
        keyboardShouldPersistTaps="handled"
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2">
        <Section title={t('warranty')}>
          <Pressable
            accessibilityRole="button"
            onPress={() => {
              setPicked(null);
              setParts([]);
              router.setParams({ warranty_uuid: undefined });
            }}
            className="flex-row items-center gap-3 px-4 py-3 active:bg-muted">
            <View className="flex-1 gap-0.5">
              <Text variant="body" numberOfLines={1}>
                {warranty.product.name}
              </Text>
              <Text variant="footnote" tone="muted" numberOfLines={1}>
                {[warranty.vehicle.plate, warranty.public_code].filter(Boolean).join(' · ')}
              </Text>
            </View>
            <Text variant="footnote" tone="primary" className="font-semibold">
              {t('change')}
            </Text>
          </Pressable>
        </Section>

        <Section title={t('parts_pick')}>
          {service.isPending ? (
            <Text variant="callout" tone="muted" className="px-4 py-3">
              {t('loading')}
            </Text>
          ) : available.length === 0 ? (
            <Text variant="callout" tone="muted" className="px-4 py-3">
              {service.isError ? errorText(service.error) : t('parts_none')}
            </Text>
          ) : (
            available.map((key, i) => {
              const on = parts.includes(key);
              return (
                <Pressable
                  key={key}
                  accessibilityRole="checkbox"
                  accessibilityState={{ checked: on }}
                  onPress={() => setParts((p) => togglePart(p, key))}
                  className={cn('min-h-11 flex-row items-center gap-3 px-4 py-2.5 active:bg-muted', i < available.length - 1 && 'border-b border-border')}>
                  <Text variant="body" className="flex-1">
                    {key}
                  </Text>
                  {on ? <Icon name="check" tone="primary" size={20} /> : null}
                </Pressable>
              );
            })
          )}
        </Section>

        <Input
          label={t('description')}
          placeholder={t('description_placeholder')}
          value={description}
          onChangeText={setDescription}
          multiline
          maxLength={DESCRIPTION_MAX}
          className="min-h-32 py-3"
        />
        <Text variant="footnote" tone="muted">
          {t('new_hint')}
        </Text>
        <Button title={t('create')} size="lg" disabled={!ready} loading={create.isPending} onPress={() => create.mutate()} />
      </ScrollView>
    </>
  );
}
