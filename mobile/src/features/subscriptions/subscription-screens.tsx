import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { useCan } from '@/core/spaces';
import { useChildHref, useFormat } from '@/features/admin/kit';
import { FleetList, FleetRow } from '@/features/customer-space/fleet-screens';
import { PAGE_SIZE, useDebounced } from '@/features/services/lib';
import { Sheet } from '@/features/services/sheet';
import { Chips, DetailSkeleton, ErrorState, errorText, InfoRow, SearchField, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  CANCEL_REQUEST_STATUSES,
  type CancelQueueItem,
  type CancelRequestStatus,
  cancelRequestTone,
  canRequestCancel,
  decisionBody,
  type Subscription,
  SUBSCRIPTION_STATUSES,
  type SubscriptionStatus,
  subscriptionTone,
  validReason,
} from './lib';

const detailKey = (uuid: string) => ['service-subscriptions', 'detail', uuid] as const;

type StatusFilter = 'all' | SubscriptionStatus;

/** Subscriptions in scope: search, status filter; the cancellation queue for approvers. */
export function SubscriptionsListScreen() {
  const { t } = useTranslation('subscriptions');
  const fmt = useFormat();
  const child = useChildHref();
  const canApprove = useCan('service_subscriptions.cancel_approve');
  const canAssign = useCan('service_subscriptions.assign');
  const [status, setStatus] = useState<StatusFilter>('all');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  return (
    <FleetList<Subscription>
      title={t('title')}
      queryKey={['service-subscriptions', 'list', { status, q: query }]}
      fetchPage={(offset) =>
        unwrap(
          api.GET('/v1/service-subscriptions', {
            params: { query: { status: status === 'all' ? undefined : status, q: query || undefined, limit: PAGE_SIZE, offset } },
          }),
        )
      }
      headerRight={
        canApprove
          ? () => (
              <Pressable
                accessibilityRole="button"
                accessibilityLabel={t('cancel_requests')}
                hitSlop={10}
                onPress={() => router.push(child('cancel-requests'))}>
                <Icon name="inbox" tone="primary" />
              </Pressable>
            )
          : undefined
      }
      header={
        <View className="gap-3 pb-1">
          <SearchField value={q} onChange={setQ} placeholder={t('search')} />
          <Chips<StatusFilter>
            value={status}
            onChange={setStatus}
            options={[
              { value: 'all', label: t('filter_all') },
              ...SUBSCRIPTION_STATUSES.map((s) => ({ value: s, label: t(`status.${s}`) })),
            ]}
          />
          {canAssign ? (
            <Text variant="footnote" tone="muted">
              {t('assign_web')}
            </Text>
          ) : null}
        </View>
      }
      empty={{ icon: 'accounting', title: query || status !== 'all' ? t('empty_filtered') : t('empty'), body: t('empty_body') }}
      renderItem={(s) => (
        <FleetRow
          title={s.item_name ?? t('subscription')}
          subtitle={s.organization_name}
          meta={[`${fmt.date(s.starts_on)} – ${fmt.date(s.ends_on)}`, `${fmt.money(s.price, s.currency)} · ${t(`recurrence.${s.recurrence}`)}`].join(' · ')}
          pill={<StatusPill label={t(`status.${s.status}`)} tone={subscriptionTone(s.status)} />}
          onPress={() => router.push(child(s.uuid))}
        />
      )}
    />
  );
}

/** One subscription: terms, price, contract state and the early cancellation request of the subscriber. */
export function SubscriptionDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('subscriptions');
  const fmt = useFormat();
  const qc = useQueryClient();
  const activeOrg = useSession((s) => s.me?.active_organization_uuid);
  const canCancel = useCan('service_subscriptions.cancel_request');
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');

  const q = useQuery({
    queryKey: detailKey(uuid),
    queryFn: () => unwrap(api.GET('/v1/service-subscriptions/{uuid}', { params: { path: { uuid } } })),
  });
  const sub = q.data;

  const request = useMutation({
    mutationFn: () =>
      unwrap(api.POST('/v1/service-subscriptions/{uuid}/cancel-request', { params: { path: { uuid } }, body: { reason: reason.trim() } })),
    onSuccess: (r) => {
      setOpen(false);
      setReason('');
      void qc.invalidateQueries({ queryKey: ['service-subscriptions'] });
      Alert.alert(t('cancel_sent'), t('cancel_sent_body', { fee: fmt.money(r.cancellation_fee, r.currency) }));
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  return (
    <>
      <Stack.Screen options={{ title: sub?.item_name ?? t('subscription'), headerLargeTitle: false }} />
      <ScrollView
        keyboardShouldPersistTaps="handled"
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!sub ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <View className="gap-2 rounded-lg border border-border bg-card p-4">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="title" className="flex-1" numberOfLines={2}>
                  {sub.item_name ?? t('subscription')}
                </Text>
                <StatusPill label={t(`status.${sub.status}`)} tone={subscriptionTone(sub.status)} />
              </View>
              {sub.organization_name ? (
                <Text variant="callout" tone="muted">
                  {sub.organization_name}
                </Text>
              ) : null}
              {canRequestCancel(sub, activeOrg, canCancel) ? (
                <Button title={t('cancel_request')} variant="outline" onPress={() => setOpen(true)} className="mt-2" />
              ) : null}
            </View>

            <Section title={t('terms')}>
              {sub.item_category ? <InfoRow label={t('category')} value={t(`category_value.${sub.item_category}`)} /> : null}
              <InfoRow label={t('starts_on')} value={fmt.date(sub.starts_on)} />
              <InfoRow label={t('ends_on')} value={fmt.date(sub.ends_on)} />
              <InfoRow label={t('recurrence_label')} value={t(`recurrence.${sub.recurrence}`)} />
              <InfoRow label={t('price')} value={fmt.money(sub.price, sub.currency)} />
              <InfoRow label={t('cancellation_fee')} value={fmt.money(sub.cancellation_fee, sub.currency)} />
              <InfoRow label={t('created_at')} value={fmt.date(sub.created_at)} last />
            </Section>

            {sub.contract ? (
              <Section title={t('contract')}>
                <InfoRow label={t('contract_no')} value={String(sub.contract.contract_no)} mono />
                <InfoRow label={t('contract_status')} value={t(`contract_status_value.${sub.contract.status}`)} last />
              </Section>
            ) : null}
            {sub.status === 'scheduled' ? (
              <Text variant="footnote" tone="muted">
                {t('scheduled_hint')}
              </Text>
            ) : null}
          </>
        )}
      </ScrollView>

      {open && sub ? (
        <Sheet
          title={t('cancel_request')}
          saveTitle={t('cancel_request_send')}
          busy={request.isPending}
          canSave={validReason(reason)}
          onClose={() => setOpen(false)}
          onSave={() => request.mutate()}>
          <Text variant="callout" tone="muted">
            {t('cancel_request_body', { fee: fmt.money(sub.cancellation_fee, sub.currency) })}
          </Text>
          <Input
            label={t('reason')}
            placeholder={t('reason_placeholder')}
            value={reason}
            onChangeText={setReason}
            multiline
            maxLength={5000}
            className="min-h-24 py-3"
            autoFocus
          />
        </Sheet>
      ) : null}
    </>
  );
}

type Decision = { item: CancelQueueItem; approve: boolean };

/** Center cancellation queue: approve or reject the early cancellation requests (cancel_approve). */
export function CancelRequestsScreen() {
  const { t } = useTranslation('subscriptions');
  const fmt = useFormat();
  const qc = useQueryClient();
  const [status, setStatus] = useState<CancelRequestStatus>('pending');
  const [decision, setDecision] = useState<Decision | null>(null);
  const [note, setNote] = useState('');

  const decide = useMutation({
    mutationFn: ({ item, approve }: Decision) =>
      unwrap(
        approve
          ? api.POST('/v1/service-subscriptions/cancel-requests/{uuid}/approve', { params: { path: { uuid: item.uuid } }, body: decisionBody(note) })
          : api.POST('/v1/service-subscriptions/cancel-requests/{uuid}/reject', { params: { path: { uuid: item.uuid } }, body: decisionBody(note) }),
      ),
    onSuccess: () => {
      setDecision(null);
      setNote('');
      void qc.invalidateQueries({ queryKey: ['service-subscriptions'] });
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });
  const openDecision = (item: CancelQueueItem, approve: boolean) => {
    setNote('');
    setDecision({ item, approve });
  };

  return (
    <>
      <FleetList<CancelQueueItem>
        title={t('cancel_requests')}
        queryKey={['service-subscriptions', 'cancel-requests', { status }]}
        fetchPage={(offset) =>
          unwrap(api.GET('/v1/service-subscriptions/cancel-requests', { params: { query: { status, limit: PAGE_SIZE, offset } } }))
        }
        header={
          <View className="pb-1">
            <Chips<CancelRequestStatus>
              value={status}
              onChange={setStatus}
              options={CANCEL_REQUEST_STATUSES.map((s) => ({ value: s, label: t(`request_status.${s}`) }))}
            />
          </View>
        }
        empty={{ icon: 'inbox', title: t('cancel_requests_empty') }}
        renderItem={(r) => (
          <FleetRow
            title={r.item_name}
            subtitle={r.organization_name}
            meta={[fmt.date(r.created_at), t('fee', { fee: fmt.money(r.cancellation_fee, r.currency) })].join(' · ')}
            pill={<StatusPill label={t(`request_status.${r.status}`)} tone={cancelRequestTone(r.status)} />}>
            <Text variant="callout" selectable numberOfLines={4}>
              {r.reason}
            </Text>
            {r.decision_note ? (
              <Text variant="footnote" tone="muted" selectable>
                {t('decision_note', { note: r.decision_note })}
              </Text>
            ) : null}
            {r.status === 'pending' ? (
              <View className="mt-1 flex-row gap-2">
                <Button title={t('approve')} onPress={() => openDecision(r, true)} className="flex-1" />
                <Button title={t('reject')} variant="outline" onPress={() => openDecision(r, false)} className="flex-1" />
              </View>
            ) : null}
          </FleetRow>
        )}
      />
      {decision ? (
        <Sheet
          title={decision.approve ? t('approve_title') : t('reject_title')}
          saveTitle={decision.approve ? t('approve') : t('reject')}
          busy={decide.isPending}
          canSave={decision.approve || validReason(note)}
          onClose={() => setDecision(null)}
          onSave={() => decide.mutate(decision)}>
          <Text variant="callout" tone="muted">
            {decision.approve
              ? t('approve_body', { fee: fmt.money(decision.item.cancellation_fee, decision.item.currency) })
              : t('reject_body')}
          </Text>
          <Input
            label={decision.approve ? t('note_optional') : t('note_required')}
            placeholder={t('note_placeholder')}
            value={note}
            onChangeText={setNote}
            multiline
            maxLength={5000}
            className="min-h-24 py-3"
            autoFocus
          />
        </Sheet>
      ) : null}
    </>
  );
}
