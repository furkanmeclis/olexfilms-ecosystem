import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Linking, Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useCan } from '@/core/spaces';
import { formatDateTime, fullName } from '@/features/services/lib';
import { Sheet } from '@/features/services/sheet';
import { DetailSkeleton, ErrorState, errorText, InfoRow, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { fetchAssignees, leadEventsKey, leadKey } from './hooks';
import {
  canMarkLost,
  isOverdue,
  type LeadStatus,
  leadName,
  leadStatusTone,
  leadTemperatureTone,
  nextLeadStatuses,
  statusBody,
} from './lib';

/** One lead: contact, pipeline status, assignee, notes and the timeline. */
export function LeadDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('leads');
  const qc = useQueryClient();
  const canWrite = useCan('leads.write');
  const [lostOpen, setLostOpen] = useState(false);
  const [lostReason, setLostReason] = useState('');
  const [assignOpen, setAssignOpen] = useState(false);
  const [note, setNote] = useState('');

  const q = useQuery({
    queryKey: leadKey(uuid),
    queryFn: () => unwrap(api.GET('/v1/leads/{uuid}', { params: { path: { uuid } } })),
  });
  const lead = q.data;

  const customer = useQuery({
    queryKey: ['customers', 'detail', lead?.customer_uuid],
    queryFn: () => unwrap(api.GET('/v1/customers/{uuid}', { params: { path: { uuid: lead?.customer_uuid as string } } })),
    enabled: !!lead?.customer_uuid,
  });
  const events = useQuery({
    queryKey: leadEventsKey(uuid),
    queryFn: () => unwrap(api.GET('/v1/leads/{uuid}/events', { params: { path: { uuid } } })),
  });
  const assignees = useQuery({
    queryKey: ['leads', 'assignees', lead?.organization_uuid],
    queryFn: () => fetchAssignees(lead?.organization_uuid),
    enabled: !!lead,
  });

  const written = (l: NonNullable<typeof lead>) => {
    qc.setQueryData(leadKey(uuid), l);
    void qc.invalidateQueries({ queryKey: ['leads', 'list'] });
    void qc.invalidateQueries({ queryKey: leadEventsKey(uuid) });
  };
  const setStatus = useMutation({
    mutationFn: (v: { status: LeadStatus; reason: string }) =>
      unwrap(api.POST('/v1/leads/{uuid}/status', { params: { path: { uuid } }, body: statusBody(v.status, v.reason) })),
    onSuccess: (l) => {
      setLostOpen(false);
      setLostReason('');
      written(l);
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });
  const assign = useMutation({
    mutationFn: (assignee: string | null) =>
      unwrap(api.POST('/v1/leads/{uuid}/assign', { params: { path: { uuid } }, body: { assignee_user_uuid: assignee } })),
    onSuccess: (l) => {
      setAssignOpen(false);
      written(l);
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });
  const addNote = useMutation({
    mutationFn: (body: string) => unwrap(api.POST('/v1/leads/{uuid}/notes', { params: { path: { uuid } }, body: { body } })),
    onSuccess: () => {
      setNote('');
      void qc.invalidateQueries({ queryKey: leadEventsKey(uuid) });
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const pickStatus = (status: LeadStatus) => {
    if (status === 'lost') {
      setLostReason('');
      setLostOpen(true);
      return;
    }
    Alert.alert(t(`status_to.${status}`), t('status_confirm_body'), [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t(`status_to.${status}`), onPress: () => setStatus.mutate({ status, reason: '' }) },
    ]);
  };

  const assignee = assignees.data?.items.find((a) => a.uuid === lead?.assignee_user_uuid);
  const title = lead ? leadName(lead) || (customer.data ? fullName(customer.data) : '') || t(`target.${lead.target_type}`) : t('lead');

  return (
    <>
      <Stack.Screen options={{ title, headerLargeTitle: false }} />
      <ScrollView
        keyboardShouldPersistTaps="handled"
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={
          <RefreshControl
            refreshing={q.isRefetching}
            onRefresh={() => {
              void q.refetch();
              void events.refetch();
            }}
          />
        }>
        {!lead ? (
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
                  {title}
                </Text>
                <StatusPill label={t(`status.${lead.status}`)} tone={leadStatusTone(lead.status)} />
              </View>
              <View className="flex-row flex-wrap items-center gap-2">
                <StatusPill label={t(`temperature.${lead.temperature}`)} tone={leadTemperatureTone(lead.temperature)} />
                <Text variant="footnote" tone="muted">
                  {[t(`target.${lead.target_type}`), t(`source.${lead.source}`)].join(' · ')}
                </Text>
              </View>
              {lead.status === 'lost' && lead.lost_reason ? (
                <Text variant="footnote" tone="danger">
                  {t('lost_reason', { reason: lead.lost_reason })}
                </Text>
              ) : null}
              {canWrite ? (
                <View className="mt-2 flex-row flex-wrap gap-2">
                  {nextLeadStatuses(lead.status).map((s) => (
                    <Button
                      key={s}
                      title={t(`status_to.${s}`)}
                      variant={s === 'won' ? 'primary' : s === 'lost' ? 'outline' : 'secondary'}
                      disabled={setStatus.isPending}
                      loading={setStatus.isPending && setStatus.variables?.status === s}
                      onPress={() => pickStatus(s)}
                      className="grow"
                    />
                  ))}
                </View>
              ) : null}
            </View>

            <Section title={t('contact')}>
              {customer.data ? <InfoRow label={t('customer')} value={fullName(customer.data)} /> : null}
              <InfoRow label={t('company')} value={lead.candidate_company_name} />
              <InfoRow label={t('contact_name')} value={lead.candidate_contact_name} />
              <InfoRow
                label={t('phone')}
                value={lead.candidate_phone_e164 ?? customer.data?.phone}
                onPress={
                  (lead.candidate_phone_e164 ?? customer.data?.phone)
                    ? () => void Linking.openURL(`tel:${lead.candidate_phone_e164 ?? customer.data?.phone}`)
                    : undefined
                }
              />
              <InfoRow
                label={t('email')}
                value={lead.candidate_email ?? customer.data?.email}
                onPress={
                  (lead.candidate_email ?? customer.data?.email)
                    ? () => void Linking.openURL(`mailto:${encodeURIComponent(lead.candidate_email ?? customer.data?.email ?? '')}`)
                    : undefined
                }
                last
              />
            </Section>

            <Section title={t('follow_up_title')}>
              <InfoRow
                label={t('follow_up_label')}
                value={lead.follow_up_date ? formatDateTime(lead.follow_up_date, i18n.language) : t('none')}
              />
              {isOverdue(lead.follow_up_date, lead.status) ? (
                <Text variant="footnote" tone="danger" className="px-4 pb-3">
                  {t('follow_up.overdue')}
                </Text>
              ) : null}
              <Pressable
                accessibilityRole="button"
                disabled={!canWrite}
                onPress={() => setAssignOpen(true)}
                className="min-h-11 flex-row items-center gap-3 border-t border-border px-4 py-2.5 active:bg-muted">
                <Text variant="callout" tone="muted" className="w-32">
                  {t('assignee')}
                </Text>
                <Text variant="callout" className="flex-1 text-right">
                  {assignee ? fullName(assignee) : t('unassigned')}
                </Text>
                {canWrite ? <Icon name="forward" tone="muted-foreground" size={16} /> : null}
              </Pressable>
            </Section>

            {lead.notes ? (
              <Section title={t('notes')}>
                <Text variant="callout" selectable className="px-4 py-3">
                  {lead.notes}
                </Text>
              </Section>
            ) : null}

            {canWrite ? (
              <View className="gap-2">
                <Input
                  label={t('note_add')}
                  placeholder={t('note_placeholder')}
                  value={note}
                  onChangeText={setNote}
                  multiline
                  maxLength={2000}
                  className="min-h-20 py-3"
                />
                <Button
                  title={t('note_save')}
                  variant="secondary"
                  disabled={!note.trim()}
                  loading={addNote.isPending}
                  onPress={() => addNote.mutate(note.trim())}
                />
              </View>
            ) : null}

            <Section title={t('timeline')}>
              {events.data && events.data.items.length > 0 ? (
                events.data.items.map((e, i, all) => (
                  <View key={e.uuid} className={`gap-0.5 px-4 py-2.5 ${i < all.length - 1 ? 'border-b border-border' : ''}`}>
                    <Text variant="callout">{t(`event.${e.event_type}`)}</Text>
                    <Text variant="footnote" tone="muted">
                      {formatDateTime(e.created_at, i18n.language)}
                    </Text>
                    {e.payload?.body ? (
                      <Text variant="callout" selectable>
                        {String(e.payload.body)}
                      </Text>
                    ) : null}
                  </View>
                ))
              ) : (
                <Text variant="callout" tone="muted" className="px-4 py-3">
                  {events.isError ? errorText(events.error) : t('timeline_empty')}
                </Text>
              )}
            </Section>
          </>
        )}
      </ScrollView>

      {lostOpen ? (
        <Sheet
          title={t('status_to.lost')}
          saveTitle={t('status_to.lost')}
          busy={setStatus.isPending}
          canSave={canMarkLost(lostReason)}
          onClose={() => setLostOpen(false)}
          onSave={() => setStatus.mutate({ status: 'lost', reason: lostReason })}>
          <Text variant="callout" tone="muted">
            {t('lost_body')}
          </Text>
          <Input
            label={t('lost_reason_label')}
            placeholder={t('lost_reason_placeholder')}
            value={lostReason}
            onChangeText={setLostReason}
            multiline
            maxLength={500}
            className="min-h-24 py-3"
            autoFocus
          />
        </Sheet>
      ) : null}
      {assignOpen ? (
        <Sheet
          title={t('assignee')}
          saveTitle={t('common:close')}
          busy={assign.isPending}
          onClose={() => setAssignOpen(false)}
          onSave={() => setAssignOpen(false)}>
          <View className="overflow-hidden rounded-lg border border-border bg-card">
            {[{ uuid: '', name: t('unassigned'), surname: '' }, ...(assignees.data?.items ?? [])].map((a, i, all) => {
              const on = (lead?.assignee_user_uuid ?? '') === a.uuid;
              return (
                <Pressable
                  key={a.uuid || 'none'}
                  accessibilityRole="button"
                  accessibilityState={{ selected: on }}
                  disabled={assign.isPending}
                  onPress={() => assign.mutate(a.uuid || null)}
                  className={`min-h-11 flex-row items-center gap-3 px-4 py-2.5 active:bg-muted ${i < all.length - 1 ? 'border-b border-border' : ''}`}>
                  <Text variant="body" className="flex-1">
                    {fullName(a)}
                  </Text>
                  {on ? <Icon name="check" tone="primary" size={20} /> : null}
                </Pressable>
              );
            })}
          </View>
          {assignees.isError ? (
            <Text variant="footnote" tone="danger">
              {errorText(assignees.error)}
            </Text>
          ) : null}
        </Sheet>
      ) : null}
    </>
  );
}
