import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useCan } from '@/core/spaces';
import { formatDateTime, formatNumber } from '@/features/services/lib';
import { Sheet } from '@/features/services/sheet';
import { Chips, DetailSkeleton, ErrorState, errorText, InfoRow, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { campaignKeys } from './campaigns-list-screen';
import {
  type Campaign,
  campaignLocales,
  campaignStats,
  campaignTone,
  canCancel,
  canDecide,
  canSubmit,
  type Decision,
  decisionBody,
  decisionReady,
} from './lib';

/**
 * One campaign: status, channels, delivery numbers, contents and the
 * approval timeline. Write users submit a draft or cancel before sending;
 * approvers (opened from the approval queue) approve, reject or ask for
 * changes after checking the audience preview.
 */
export function CampaignDetailScreen() {
  const { uuid, review } = useLocalSearchParams<{ uuid: string; review?: string }>();
  const { t, i18n } = useTranslation('campaigns');
  const qc = useQueryClient();
  const canWrite = useCan('campaigns.write');
  const canApprove = useCan('campaigns.approve');
  const [decisionOpen, setDecisionOpen] = useState(false);
  const [decision, setDecision] = useState<Decision>('approve');
  const [reason, setReason] = useState('');

  const q = useQuery({
    queryKey: campaignKeys.detail(uuid),
    queryFn: () => unwrap(api.GET('/v1/campaigns/{uuid}', { params: { path: { uuid } } })),
  });
  const c = q.data;
  const reviewing = review === '1' && canApprove && !!c && canDecide(c.status);

  const preview = useQuery({
    queryKey: [...campaignKeys.detail(uuid), 'preview'],
    queryFn: () => unwrap(api.POST('/v1/campaigns/{uuid}/preview', { params: { path: { uuid } } })),
    enabled: decisionOpen,
  });

  const written = (next: Campaign) => {
    qc.setQueryData(campaignKeys.detail(uuid), next);
    void qc.invalidateQueries({ queryKey: campaignKeys.all });
  };
  const fail = (e: unknown) => Alert.alert(t('toast.failed'), errorText(e));

  const submit = useMutation({
    mutationFn: () => unwrap(api.POST('/v1/campaigns/{uuid}/submit', { params: { path: { uuid } } })),
    onSuccess: (next) => {
      written(next);
      Alert.alert(t('toast.submitted'));
    },
    onError: fail,
  });
  const cancel = useMutation({
    mutationFn: () => unwrap(api.POST('/v1/campaigns/{uuid}/cancel', { params: { path: { uuid } } })),
    onSuccess: (next) => {
      written(next);
      Alert.alert(t('toast.cancelled'));
    },
    onError: fail,
  });
  const decide = useMutation({
    mutationFn: ({ d, r }: { d: Decision; r: string }) => {
      const opts = { params: { path: { uuid } }, body: decisionBody(d, r) };
      if (d === 'approve') return unwrap(api.POST('/v1/campaigns/{uuid}/approve', opts));
      if (d === 'reject') return unwrap(api.POST('/v1/campaigns/{uuid}/reject', { ...opts, body: { reason: r.trim() } }));
      return unwrap(api.POST('/v1/campaigns/{uuid}/request-changes', { ...opts, body: { reason: r.trim() } }));
    },
    onSuccess: (next) => {
      setDecisionOpen(false);
      written(next);
      Alert.alert(t('approvals.toast_decided'));
      if (router.canGoBack()) router.back();
    },
    onError: fail,
  });

  const confirm = (title: string, body: string, run: () => void, destructive?: boolean) =>
    Alert.alert(title, body, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: title, style: destructive ? 'destructive' : 'default', onPress: run },
    ]);

  const openDecision = (d: Decision) => {
    setDecision(d);
    setReason('');
    setDecisionOpen(true);
  };

  const stats = c ? campaignStats(c) : null;
  const busy = submit.isPending || cancel.isPending || decide.isPending;

  return (
    <>
      <Stack.Screen options={{ title: c?.name ?? t('detail.title'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!c ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <View className="gap-2 rounded-lg border border-border bg-card p-4">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="title" className="flex-1" numberOfLines={3}>
                  {c.name}
                </Text>
                <StatusPill label={t(`status.${c.status}`)} tone={campaignTone(c.status)} />
              </View>
              <Text variant="footnote" tone="muted">
                {[c.organization_name, c.channels.map((ch) => t(`channel.${ch}`)).join(', ')].join(' · ')}
              </Text>
              {reviewing ? (
                <View className="mt-2 gap-2">
                  <Button title={t('actions.approve')} disabled={busy} onPress={() => openDecision('approve')} />
                  <View className="flex-row gap-2">
                    <Button
                      title={t('actions.request_changes')}
                      variant="secondary"
                      disabled={busy}
                      onPress={() => openDecision('changes')}
                      className="flex-1"
                    />
                    <Button
                      title={t('actions.reject')}
                      variant="outline"
                      disabled={busy}
                      onPress={() => openDecision('reject')}
                      className="flex-1"
                    />
                  </View>
                </View>
              ) : null}
              {canWrite && !reviewing && (canSubmit(c.status) || canCancel(c.status)) ? (
                <View className="mt-2 flex-row gap-2">
                  {canSubmit(c.status) ? (
                    <Button
                      title={t('actions.submit_approval')}
                      disabled={busy}
                      loading={submit.isPending}
                      onPress={() => confirm(t('actions.submit_approval'), t('submit_body'), () => submit.mutate())}
                      className="flex-1"
                    />
                  ) : null}
                  {canCancel(c.status) ? (
                    <Button
                      title={t('actions.cancel')}
                      variant="outline"
                      disabled={busy}
                      loading={cancel.isPending}
                      onPress={() => confirm(t('actions.cancel'), t('cancel_body'), () => cancel.mutate(), true)}
                      className="flex-1"
                    />
                  ) : null}
                </View>
              ) : null}
            </View>

            <Section title={t('detail.summary')}>
              <InfoRow label={t('fields.audience_type')} value={t(`audience.${c.audience_filter.audience_type}`)} />
              <InfoRow
                label={t('fields.scheduled_at')}
                value={c.scheduled_at ? formatDateTime(c.scheduled_at, i18n.language) : t('detail.not_scheduled')}
              />
              <InfoRow label={t('detail.started_at')} value={formatDateTime(c.started_at, i18n.language)} />
              <InfoRow label={t('detail.finished_at')} value={formatDateTime(c.finished_at, i18n.language)} />
              <InfoRow label={t('detail.timezone')} value={c.timezone} last />
            </Section>

            {stats ? (
              <Section title={t('detail.delivery')}>
                <InfoRow label={t('stats.total')} value={formatNumber(c.recipients_total, i18n.language)} />
                <InfoRow label={t('stats.sent')} value={formatNumber(c.recipients_sent, i18n.language)} />
                <InfoRow label={t('stats.failed')} value={formatNumber(c.recipients_failed, i18n.language)} />
                <InfoRow label={t('stats.skipped')} value={formatNumber(c.recipients_skipped, i18n.language)} />
                <InfoRow label={t('stats.pending')} value={formatNumber(stats.pending, i18n.language)} />
                <InfoRow
                  label={t('stats.success_rate')}
                  value={stats.successRate === null ? '—' : `${stats.successRate}%`}
                  last
                />
              </Section>
            ) : null}

            {(c.contents ?? []).length ? (
              <Section title={t('detail.contents')}>
                {(c.contents ?? []).map((content, i, all) => (
                  <View key={content.locale} className={`gap-1 px-4 py-3 ${i < all.length - 1 ? 'border-b border-border' : ''}`}>
                    <Text variant="caption" tone="muted">
                      {content.locale}
                    </Text>
                    {content.title ? <Text variant="headline">{content.title}</Text> : null}
                    <Text variant="callout" selectable>
                      {content.body}
                    </Text>
                  </View>
                ))}
              </Section>
            ) : null}

            <Section title={t('timeline.title')}>
              {(c.events ?? []).length ? (
                (c.events ?? []).map((e, i, all) => (
                  <View key={e.uuid} className={`gap-0.5 px-4 py-2.5 ${i < all.length - 1 ? 'border-b border-border' : ''}`}>
                    <Text variant="callout">{t(`event.${e.event_type}`)}</Text>
                    <Text variant="footnote" tone="muted">
                      {formatDateTime(e.created_at, i18n.language)}
                    </Text>
                    {e.reason ? (
                      <Text variant="callout" selectable>
                        {e.reason}
                      </Text>
                    ) : null}
                  </View>
                ))
              ) : (
                <Text variant="callout" tone="muted" className="px-4 py-3">
                  {t('timeline.empty')}
                </Text>
              )}
            </Section>
          </>
        )}
      </ScrollView>

      {decisionOpen && c ? (
        <Sheet
          title={c.name}
          saveTitle={t('approvals.submit')}
          busy={decide.isPending}
          canSave={decisionReady(decision, reason)}
          onClose={() => setDecisionOpen(false)}
          onSave={() => decide.mutate({ d: decision, r: reason })}>
          <Text variant="callout" tone="muted">
            {t('approvals.preview_description')}
          </Text>
          <View className="gap-1 rounded-lg bg-secondary p-3">
            {preview.isPending ? (
              <Text variant="footnote" tone="muted">
                {t('preview.loading')}
              </Text>
            ) : preview.isError ? (
              <Text variant="footnote" tone="danger">
                {t('preview.failed')}
              </Text>
            ) : null}
            <Text variant="callout">{t('preview.total', { count: preview.data?.total ?? c.recipients_total })}</Text>
            <Text variant="callout">
              {`${t('preview.languages')}: ${campaignLocales(
                preview.data?.locales ?? [],
                (c.contents ?? []).map((x) => x.locale),
              ).join(', ')}`}
            </Text>
            <Text variant="callout">{t('preview.excluded', { count: preview.data?.excluded.total ?? 0 })}</Text>
          </View>
          <Chips<Decision>
            value={decision}
            onChange={setDecision}
            options={[
              { value: 'approve', label: t('actions.approve') },
              { value: 'changes', label: t('actions.request_changes') },
              { value: 'reject', label: t('actions.reject') },
            ]}
          />
          <Input
            label={t('approvals.reason')}
            placeholder={t('approvals.reason_placeholder')}
            value={reason}
            onChangeText={setReason}
            multiline
            maxLength={1000}
            className="min-h-24 py-3"
            error={!decisionReady(decision, reason) ? t('validation.reason_required') : undefined}
          />
        </Sheet>
      ) : null}
    </>
  );
}
