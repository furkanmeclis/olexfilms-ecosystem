import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { Image } from 'expo-image';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Modal, Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { apiImage } from '@/features/services/intake-photos';
import { formatDateTime } from '@/features/services/lib';
import { Sheet } from '@/features/services/sheet';
import { DetailSkeleton, ErrorState, errorText, InfoRow, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { ClaimPhotoCapture, claimKey } from './claim-photo-capture';
import { useClaimGrants } from './hooks';
import { canAddClaimPhoto, canSubmitClaim, type ClaimStatus, claimActions, claimTone } from './lib';

type Sheets = 'reject' | 'reopen' | null;

/** One claim: status, coverage, parts, photos, timeline and the actions the caller may take. */
export function ClaimDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('claims');
  const qc = useQueryClient();
  const { grants, orgType } = useClaimGrants();
  const [sheet, setSheet] = useState<Sheets>(null);
  const [reason, setReason] = useState('');
  const [capture, setCapture] = useState(false);
  const [viewing, setViewing] = useState<string | null>(null);

  const q = useQuery({
    queryKey: claimKey(uuid),
    queryFn: () => unwrap(api.GET('/v1/warranty-claims/{uuid}', { params: { path: { uuid } } })),
  });
  const c = q.data;

  const done = () => {
    setSheet(null);
    setReason('');
    void qc.invalidateQueries({ queryKey: ['warranty-claims'] });
  };
  const transition = useMutation({
    mutationFn: (v: { status: ClaimStatus; rejection_reason?: string }) =>
      unwrap(api.POST('/v1/warranty-claims/{uuid}/status', { params: { path: { uuid } }, body: v })),
    onSuccess: (updated) => {
      qc.setQueryData(claimKey(uuid), (old: typeof c) => ({ ...old, ...updated }));
      done();
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });
  const reopen = useMutation({
    mutationFn: (r: string) =>
      unwrap(api.POST('/v1/warranty-claims/{uuid}/reopen', { params: { path: { uuid } }, body: { reason: r } })),
    onSuccess: (updated) => {
      qc.setQueryData(claimKey(uuid), (old: typeof c) => ({ ...old, ...updated }));
      done();
    },
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const confirm = (title: string, status: ClaimStatus) =>
    Alert.alert(title, t('confirm_body'), [
      { text: t('common:cancel'), style: 'cancel' },
      { text: title, onPress: () => transition.mutate({ status }) },
    ]);

  const actions = c ? claimActions(c.status, grants, orgType) : null;
  const photoCount = c?.photos?.length ?? 0;
  const partKeys = (c?.parts ?? []).map((p) => p.part_key);
  const submitReady = canSubmitClaim({ parts: partKeys, description: c?.description ?? '', photos: photoCount });
  const busy = transition.isPending || reopen.isPending;

  return (
    <>
      <Stack.Screen options={{ title: c ? t('claim_no', { no: c.claim_no }) : t('claim'), headerLargeTitle: false }} />
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
                <Text variant="title" className="flex-1">
                  {t('claim_no', { no: c.claim_no })}
                </Text>
                <StatusPill label={t(`status.${c.status}`)} tone={claimTone(c.status)} />
              </View>
              {c.product_name ? <Text variant="callout">{c.product_name}</Text> : null}
              <Text variant="footnote" tone="muted">
                {[c.warranty_no, c.plate, formatDateTime(c.created_at, i18n.language)].filter(Boolean).join(' · ')}
              </Text>
              {c.status === 'rejected' && c.rejection_reason ? (
                <Text variant="footnote" tone="danger">
                  {t('rejection_reason', { reason: c.rejection_reason })}
                </Text>
              ) : null}
              {actions && (actions.submitReview || actions.forward || actions.decide || actions.reopen) ? (
                <View className="mt-2 flex-row flex-wrap gap-2">
                  {actions.submitReview ? (
                    <Button
                      title={t('action.submit_review')}
                      disabled={busy || !submitReady}
                      loading={transition.isPending && transition.variables?.status === 'dealer_review'}
                      onPress={() => confirm(t('action.submit_review'), 'dealer_review')}
                      className="grow"
                    />
                  ) : null}
                  {actions.forward ? (
                    <Button
                      title={t('action.forward')}
                      disabled={busy}
                      loading={transition.isPending && transition.variables?.status === 'center_review'}
                      onPress={() => confirm(t('action.forward'), 'center_review')}
                      className="grow"
                    />
                  ) : null}
                  {actions.decide ? (
                    <>
                      <Button
                        title={t('action.approve')}
                        disabled={busy}
                        loading={transition.isPending && transition.variables?.status === 'approved'}
                        onPress={() => confirm(t('action.approve'), 'approved')}
                        className="grow"
                      />
                      <Button
                        title={t('action.reject')}
                        variant="outline"
                        disabled={busy}
                        onPress={() => {
                          setReason('');
                          setSheet('reject');
                        }}
                        className="grow"
                      />
                    </>
                  ) : null}
                  {actions.reopen ? (
                    <Button
                      title={t('action.reopen')}
                      variant="outline"
                      disabled={busy}
                      onPress={() => {
                        setReason('');
                        setSheet('reopen');
                      }}
                      className="grow"
                    />
                  ) : null}
                </View>
              ) : null}
              {actions?.submitReview && !submitReady ? (
                <Text variant="footnote" tone="muted">
                  {t('submit_needs')}
                </Text>
              ) : null}
            </View>

            <Section title={t('coverage')}>
              <View className="gap-1 px-4 py-3">
                <View className="flex-row items-center gap-2">
                  <Icon name={c.coverage_check.ok ? 'check' : 'alert'} tone={c.coverage_check.ok ? 'primary' : 'destructive'} size={18} />
                  <Text variant="callout" className="font-semibold">
                    {c.coverage_check.ok ? t('coverage_ok') : t('coverage_not_ok')}
                  </Text>
                </View>
                {c.coverage_check.reasons.map((r) => (
                  <Text key={r} variant="footnote" tone="muted">
                    {t(`coverage_reason.${r}`)}
                  </Text>
                ))}
              </View>
            </Section>

            <Section title={t('details')}>
              <InfoRow label={t('organization')} value={c.organization_name} />
              <InfoRow label={t('service')} value={c.service_no} />
              <InfoRow label={t('plate')} value={c.plate} last />
            </Section>

            {c.description ? (
              <Section title={t('description')}>
                <Text variant="callout" selectable className="px-4 py-3">
                  {c.description}
                </Text>
              </Section>
            ) : null}

            <Section title={t('parts')}>
              {c.parts && c.parts.length > 0 ? (
                c.parts.map((p, i) => (
                  <View key={p.uuid} className={`gap-0.5 px-4 py-2.5 ${i < (c.parts?.length ?? 0) - 1 ? 'border-b border-border' : ''}`}>
                    <Text variant="callout">{p.part_key}</Text>
                    {p.note ? (
                      <Text variant="footnote" tone="muted">
                        {p.note}
                      </Text>
                    ) : null}
                  </View>
                ))
              ) : (
                <Text variant="callout" tone="muted" className="px-4 py-3">
                  {t('parts_empty')}
                </Text>
              )}
            </Section>

            <Section
              title={t('photos', { count: photoCount })}
              right={
                canAddClaimPhoto(c.status, grants.write) ? (
                  <Pressable accessibilityRole="button" hitSlop={8} onPress={() => setCapture(true)}>
                    <Text variant="footnote" tone="primary" className="font-semibold">
                      {t('photo_add')}
                    </Text>
                  </Pressable>
                ) : undefined
              }>
              {photoCount > 0 ? (
                <View className="flex-row flex-wrap gap-2 p-3">
                  {c.photos?.map((p) => (
                    <Pressable
                      key={p.uuid}
                      accessibilityRole="imagebutton"
                      accessibilityLabel={t('photo_open')}
                      onPress={() => setViewing(p.uuid)}>
                      <Image
                        source={apiImage(`/v1/warranty-claims/${uuid}/photos/${p.uuid}`)}
                        style={{ width: 96, height: 72, borderRadius: 8 }}
                        contentFit="cover"
                      />
                    </Pressable>
                  ))}
                </View>
              ) : (
                <Text variant="callout" tone="muted" className="px-4 py-3">
                  {t('photos_empty')}
                </Text>
              )}
            </Section>

            {c.ai_summary ? (
              <Section title={t('ai_triage')}>
                <View className="gap-1 px-4 py-3">
                  <Text variant="callout" selectable>
                    {c.ai_summary}
                  </Text>
                  <Text variant="footnote" tone="muted">
                    {[c.ai_damage_type, c.ai_confidence].filter(Boolean).join(' · ')}
                  </Text>
                </View>
              </Section>
            ) : null}

            {c.cost_summary ? (
              <Section title={t('cost')}>
                <InfoRow label={t('cost_product')} value={`${c.cost_summary.product_cost} ${c.cost_summary.currency}`} />
                <InfoRow label={t('cost_labor')} value={`${c.cost_summary.labor} ${c.cost_summary.currency}`} last />
              </Section>
            ) : null}

            {c.reapply_service ? (
              <Section title={t('reapply')}>
                <InfoRow label={t('service')} value={c.reapply_service.service_no} />
                <InfoRow label={t('reapply_status')} value={c.reapply_service.status} last />
              </Section>
            ) : null}

            {c.events && c.events.length > 0 ? (
              <Section title={t('timeline')}>
                {c.events.map((e, i) => (
                  <View key={e.uuid} className={`gap-0.5 px-4 py-2.5 ${i < (c.events?.length ?? 0) - 1 ? 'border-b border-border' : ''}`}>
                    <Text variant="callout">
                      {e.event_type === 'status_changed' && e.to_status
                        ? `${e.from_status ? `${t(`status.${e.from_status}`)} → ` : ''}${t(`status.${e.to_status}`)}`
                        : t(`event.${e.event_type}`)}
                    </Text>
                    <Text variant="footnote" tone="muted">
                      {formatDateTime(e.created_at, i18n.language)}
                      {e.note ? ` · ${e.note}` : ''}
                    </Text>
                  </View>
                ))}
              </Section>
            ) : null}
          </>
        )}
      </ScrollView>

      {sheet === 'reject' ? (
        <Sheet
          title={t('action.reject')}
          saveTitle={t('action.reject')}
          busy={transition.isPending}
          canSave={reason.trim().length > 0}
          onClose={() => setSheet(null)}
          onSave={() => transition.mutate({ status: 'rejected', rejection_reason: reason.trim() })}>
          <Text variant="callout" tone="muted">
            {t('reject_body')}
          </Text>
          <Input
            label={t('reason_label')}
            placeholder={t('reason_placeholder')}
            value={reason}
            onChangeText={setReason}
            multiline
            maxLength={1000}
            className="min-h-24 py-3"
            autoFocus
          />
        </Sheet>
      ) : null}
      {sheet === 'reopen' ? (
        <Sheet
          title={t('action.reopen')}
          saveTitle={t('action.reopen')}
          busy={reopen.isPending}
          canSave={reason.trim().length > 0}
          onClose={() => setSheet(null)}
          onSave={() => reopen.mutate(reason.trim())}>
          <Text variant="callout" tone="muted">
            {t('reopen_body')}
          </Text>
          <Input
            label={t('reason_label')}
            placeholder={t('reason_placeholder')}
            value={reason}
            onChangeText={setReason}
            multiline
            maxLength={1000}
            className="min-h-24 py-3"
            autoFocus
          />
        </Sheet>
      ) : null}
      {capture ? <ClaimPhotoCapture claimUuid={uuid} onClose={() => setCapture(false)} /> : null}
      {viewing ? (
        <Modal visible animationType="fade" presentationStyle="fullScreen" onRequestClose={() => setViewing(null)}>
          <Pressable accessibilityRole="button" accessibilityLabel={t('common:close')} onPress={() => setViewing(null)} className="flex-1 bg-black">
            <Image
              source={apiImage(`/v1/warranty-claims/${uuid}/photos/${viewing}`)}
              style={{ flex: 1 }}
              contentFit="contain"
            />
          </Pressable>
        </Modal>
      ) : null}
    </>
  );
}
