import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDate, formatDateTime, PAGE_SIZE, usePagedList } from '@/features/services/lib';
import { Sheet } from '@/features/services/sheet';
import { DetailSkeleton, ErrorState, errorText, InfoRow, ListFooter, ListSkeleton, Section } from '@/features/services/ui';
import { EmptyState } from '@/ui/empty-state';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { accountingKey, useAccountingAccess, useCategoryLabels, useOpenDisputeEntries } from './hooks';
import {
  balanceKind,
  balanceTone,
  counterpartyKind,
  DISPUTE_TEXT_MAX,
  type FinanceEntry,
  formatMoney,
  isEntryDisputable,
} from './lib';

/**
 * One cari account: balance, entry count, last entry and its ledger rows
 * (newest first, paged). A child organization disputes a row its parent posted
 * (K24); collections, payments and the statement export stay on the web.
 */
export function CariDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('accounting');
  const qc = useQueryClient();
  const access = useAccountingAccess();
  const key = accountingKey(access.orgUuid);
  const categories = useCategoryLabels(access.orgUuid, access.canRead);
  const openDisputes = useOpenDisputeEntries(access.orgUuid, access.canDispute);
  const [disputing, setDisputing] = useState<FinanceEntry | null>(null);
  const [reason, setReason] = useState('');

  const cari = useQuery({
    queryKey: [...key, 'cari-detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/accounting/cari/{uuid}', { params: { path: { uuid } } })),
  });
  const entries = usePagedList([...key, 'entries', { cari: uuid }], (offset) =>
    unwrap(api.GET('/v1/accounting/entries', { params: { query: { cari_uuid: uuid, limit: PAGE_SIZE, offset } } })),
  );

  const open = useMutation({
    mutationFn: (v: { entry_uuid: string; reason: string }) => unwrap(api.POST('/v1/accounting/disputes', { body: v })),
    onSuccess: () => {
      setDisputing(null);
      setReason('');
      void qc.invalidateQueries({ queryKey: key });
      Alert.alert(t('disputes.opened'));
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const c = cari.data;
  const side = c ? balanceKind(c.balance) : 'settled';
  const categoryLabel = (e: FinanceEntry) => categories.get(e.category) ?? e.category;

  const header = !c ? (
    cari.isPending ? (
      <DetailSkeleton />
    ) : (
      <ErrorState error={cari.error} onRetry={() => void cari.refetch()} />
    )
  ) : (
    <View className="gap-4 pb-1 pt-2">
      <View className="gap-2 rounded-lg border border-border bg-card p-4">
        <View className="flex-row items-center justify-between gap-3">
          <Text variant="footnote" tone="muted" className="flex-1">
            {t('fields.balance')}
          </Text>
          <StatusPill label={t(`balance.${side}`)} tone={balanceTone(side)} />
        </View>
        <Text variant="title" className="tabular-nums">
          {formatMoney(Math.abs(Number(c.balance)), c.currency, i18n.language)}
        </Text>
        <Text variant="footnote" tone="muted">
          {t(`counterparty.${counterpartyKind(c)}`)}
        </Text>
      </View>
      <Section>
        <InfoRow label={t('fields.entry_count')} value={c.entry_count} />
        <InfoRow label={t('fields.last_entry')} value={c.last_entry_at ? formatDateTime(c.last_entry_at, i18n.language) : '—'} />
        <InfoRow label={t('fields.status')} value={c.active ? t('status.active') : t('status.inactive')} last />
      </Section>
      <Text variant="caption" tone="muted" className="px-1">
        {t('cari.entries')}
      </Text>
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: c?.counterparty.name ?? t('cari.detail_title'), headerLargeTitle: false }} />
      <FlatList
        data={c ? entries.items : []}
        keyExtractor={(e) => e.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-12"
        ListHeaderComponent={header}
        ListEmptyComponent={
          !c ? undefined : entries.isPending ? (
            <ListSkeleton rows={3} />
          ) : entries.error ? (
            <ErrorState error={entries.error} onRetry={() => void entries.refetch()} />
          ) : (
            <EmptyState icon="accounting" title={t('entries.empty')} />
          )
        }
        renderItem={({ item: e }) => {
          const disputed = openDisputes.has(e.uuid);
          const disputable = !disputed && access.canDispute && isEntryDisputable(e, access.parentUuid);
          const foreign = e.orig_currency !== e.currency;
          return (
            <View className="gap-1 rounded-lg border border-border bg-card px-4 py-3">
              <View className="flex-row items-start justify-between gap-3">
                <Text variant="callout" numberOfLines={2} className="flex-1 font-semibold">
                  {categoryLabel(e)}
                </Text>
                <Text variant="callout" tone={Number(e.amount) < 0 ? 'danger' : 'default'} className="tabular-nums">
                  {formatMoney(e.amount, e.currency, i18n.language)}
                </Text>
              </View>
              {foreign ? (
                <Text variant="footnote" tone="muted" className="text-right tabular-nums">
                  {formatMoney(e.orig_amount, e.orig_currency, i18n.language)} ·{' '}
                  {t('entries.rate', { rate: Number(e.rate), date: formatDate(e.rate_date, i18n.language) })}
                </Text>
              ) : null}
              <Text variant="footnote" tone="muted" numberOfLines={2}>
                {[formatDateTime(e.created_at, i18n.language), t(`directions.${e.direction}`), e.description]
                  .filter(Boolean)
                  .join(' · ')}
              </Text>
              {e.reversal_of_uuid || e.voided || disputed || disputable ? (
                <View className="mt-1 flex-row flex-wrap items-center gap-2">
                  {e.reversal_of_uuid ? <StatusPill label={t('entries.reversal')} tone="progress" /> : null}
                  {e.voided ? <StatusPill label={t('entries.voided')} tone="problem" /> : null}
                  {disputed ? <StatusPill label={t('disputes.status.open')} tone="progress" /> : null}
                  {disputable ? (
                    <Pressable
                      accessibilityRole="button"
                      hitSlop={8}
                      onPress={() => {
                        setReason('');
                        setDisputing(e);
                      }}>
                      <Text variant="footnote" tone="primary" className="font-semibold">
                        {t('disputes.open_action')}
                      </Text>
                    </Pressable>
                  ) : null}
                </View>
              ) : null}
            </View>
          );
        }}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (entries.hasNextPage && !entries.isFetchingNextPage) void entries.fetchNextPage();
        }}
        ListFooterComponent={<ListFooter loading={entries.isFetchingNextPage} />}
        refreshControl={
          <RefreshControl
            refreshing={(cari.isRefetching || entries.isRefetching) && !entries.isFetchingNextPage}
            onRefresh={() => {
              void cari.refetch();
              void entries.refetch();
            }}
          />
        }
      />

      {disputing ? (
        <Sheet
          title={t('disputes.open_title')}
          saveTitle={t('disputes.submit')}
          busy={open.isPending}
          canSave={reason.trim().length > 0}
          onClose={() => setDisputing(null)}
          onSave={() => open.mutate({ entry_uuid: disputing.uuid, reason: reason.trim() })}>
          <Text variant="callout" tone="muted">
            {t('disputes.open_description')}
          </Text>
          <View className="rounded-md bg-secondary p-3">
            <Text variant="footnote">
              {[formatDateTime(disputing.created_at, i18n.language), categoryLabel(disputing), formatMoney(disputing.amount, disputing.currency, i18n.language)].join(
                ' · ',
              )}
            </Text>
          </View>
          <Input
            label={t('disputes.fields.reason')}
            placeholder={t('disputes.reason_hint')}
            value={reason}
            onChangeText={setReason}
            multiline
            maxLength={DISPUTE_TEXT_MAX}
            className="min-h-24 py-3"
            autoFocus
          />
        </Sheet>
      ) : null}
    </>
  );
}
