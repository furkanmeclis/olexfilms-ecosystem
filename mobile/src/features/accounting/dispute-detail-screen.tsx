import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDateTime } from '@/features/services/lib';
import { Chips, DetailSkeleton, ErrorState, errorText, InfoRow, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { accountingKey, useAccountingAccess, useCategoryLabels } from './hooks';
import {
  canResolveDispute,
  DISPUTE_TEXT_MAX,
  disputeTone,
  ENTRY_SOURCE_TYPES,
  formatMoney,
  type Resolution,
  type ResolveErrors,
  type ResolveForm,
  RESOLUTIONS,
  resolveInput,
  validateResolve,
} from './lib';

/**
 * One dispute: the disputed row, the reason and, once resolved, the outcome.
 * The parent the row came from resolves it here (reversal, revision or reject),
 * as on the web; no step-up is involved.
 */
export function DisputeDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('accounting');
  const qc = useQueryClient();
  const access = useAccountingAccess();
  const key = accountingKey(access.orgUuid);
  const categories = useCategoryLabels(access.orgUuid, access.canRead);
  const [form, setForm] = useState<ResolveForm>({ resolution: 'reversal', corrected_amount: '', note: '' });
  const [errors, setErrors] = useState<ResolveErrors>({});

  const q = useQuery({
    queryKey: [...key, 'dispute', uuid],
    queryFn: () => unwrap(api.GET('/v1/accounting/disputes/{uuid}', { params: { path: { uuid } } })),
  });
  const d = q.data;

  const resolve = useMutation({
    mutationFn: () =>
      unwrap(api.POST('/v1/accounting/disputes/{uuid}/resolve', { params: { path: { uuid } }, body: resolveInput(form) })),
    onSuccess: (resolved) => {
      qc.setQueryData([...key, 'dispute', uuid], resolved);
      void qc.invalidateQueries({ queryKey: key });
      Alert.alert(t(`disputes.resolved.${resolved.status}`));
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const submit = () => {
    const errs = validateResolve(form);
    setErrors(errs);
    if (Object.keys(errs).length) return;
    const title = t(`disputes.resolve.submit_${form.resolution}`);
    Alert.alert(title, t('disputes.resolve.description'), [
      { text: t('common:cancel'), style: 'cancel' },
      { text: title, style: form.resolution === 'reject' ? 'destructive' : 'default', onPress: () => resolve.mutate() },
    ]);
  };

  const set = <K extends keyof ResolveForm>(k: K, v: ResolveForm[K]) => setForm((f) => ({ ...f, [k]: v }));
  const resolvable = d ? canResolveDispute(d, access.orgUuid, access.canResolve) : false;
  const source = d ? (ENTRY_SOURCE_TYPES.includes(d.source_type) ? t(`sources.${d.source_type}`) : d.source_type) : '';
  const fieldError = (code?: string) => (code ? t(`validation.${code}`, { max: DISPUTE_TEXT_MAX }) : undefined);

  return (
    <>
      <Stack.Screen options={{ title: t('disputes.detail_title'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!d ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <View className="gap-2 rounded-lg border border-border bg-card p-4">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="title" numberOfLines={2} className="flex-1">
                  {d.organization.name}
                </Text>
                <StatusPill label={t(`disputes.status.${d.status}`)} tone={disputeTone(d.status)} />
              </View>
              <Text variant="footnote" tone="muted">
                {[t('disputes.to', { name: d.counterparty_organization.name }), formatDateTime(d.created_at, i18n.language)].join(' · ')}
              </Text>
            </View>

            <Section title={t('disputes.entry_title')}>
              <InfoRow label={t('fields.date')} value={formatDateTime(d.entry.created_at, i18n.language)} />
              <InfoRow label={t('fields.direction')} value={t(`directions.${d.entry.direction}`)} />
              <InfoRow label={t('fields.category')} value={categories.get(d.entry.category) ?? d.entry.category} />
              <InfoRow label={t('fields.source')} value={source} />
              <InfoRow label={t('fields.amount')} value={formatMoney(d.entry.orig_amount, d.entry.orig_currency, i18n.language)} last />
            </Section>

            <Section title={t('disputes.fields.reason')}>
              <Text variant="callout" selectable className="px-4 py-3">
                {d.reason}
              </Text>
            </Section>

            {d.status !== 'open' ? (
              <Section title={t('disputes.resolution_title')}>
                <InfoRow label={t('fields.status')} value={t(`disputes.status.${d.status}`)} />
                <InfoRow
                  label={t('disputes.fields.corrected_amount')}
                  value={d.corrected_amount ? formatMoney(d.corrected_amount, d.entry.orig_currency, i18n.language) : null}
                />
                <InfoRow label={t('disputes.fields.resolution_note')} value={d.resolution_note} />
                <InfoRow
                  label={t('disputes.fields.resolved_at')}
                  value={d.resolved_at ? formatDateTime(d.resolved_at, i18n.language) : '—'}
                  last
                />
              </Section>
            ) : resolvable ? (
              <Section title={t('disputes.resolve.title')}>
                <View className="gap-4 p-4">
                  <Chips<Resolution>
                    value={form.resolution}
                    onChange={(r) => {
                      set('resolution', r);
                      setErrors({});
                    }}
                    options={RESOLUTIONS.map((r) => ({ value: r, label: t(`disputes.resolve.${r}`) }))}
                  />
                  <Text variant="footnote" tone="muted">
                    {t(`disputes.resolve.${form.resolution}_hint`)}
                  </Text>
                  {form.resolution === 'revision' ? (
                    <Input
                      label={t('disputes.resolve.corrected_amount', { currency: d.entry.orig_currency })}
                      placeholder={t('disputes.resolve.amount_placeholder')}
                      value={form.corrected_amount}
                      onChangeText={(v) => set('corrected_amount', v)}
                      keyboardType="decimal-pad"
                      error={fieldError(errors.corrected_amount)}
                    />
                  ) : null}
                  <Input
                    label={form.resolution === 'reject' ? t('disputes.resolve.reject_note') : t('disputes.resolve.note')}
                    placeholder={t('disputes.resolve.note_placeholder')}
                    value={form.note}
                    onChangeText={(v) => set('note', v)}
                    multiline
                    maxLength={DISPUTE_TEXT_MAX}
                    className="min-h-24 py-3"
                    error={fieldError(errors.note)}
                  />
                  <Button
                    title={t(`disputes.resolve.submit_${form.resolution}`)}
                    variant={form.resolution === 'reject' ? 'destructive' : 'primary'}
                    loading={resolve.isPending}
                    onPress={submit}
                  />
                </View>
              </Section>
            ) : (
              <Text variant="footnote" tone="muted" className="px-1">
                {t('disputes.waiting')}
              </Text>
            )}
          </>
        )}
      </ScrollView>
    </>
  );
}
