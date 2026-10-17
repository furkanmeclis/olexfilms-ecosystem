import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Linking, Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useCan } from '@/core/spaces';
import { Button } from '@/ui/button';
import { ExternalLink, PackagePlus, Share2, ShieldCheck, Trash } from '@/ui/icons';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  formatDate,
  formatDateTime,
  fullName,
  type Service,
  type ServiceStatus,
  serviceTone,
  useLinks,
  vehicleLabel,
  warrantyTone,
} from './lib';
import { IntakePhotos, intakeKey } from './intake-photos';
import { WarrantyQrButton } from './qr-sheet';
import { openWarranty, shareWarranty } from './share';
import { Sheet } from './sheet';
import { DetailSkeleton, ErrorState, errorText, InfoRow, Section } from './ui';

export function useService(uuid: string) {
  return useQuery({
    queryKey: ['services', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/services/{uuid}', { params: { path: { uuid } } })),
  });
}

/** After a write: the detail gets the returned service, lists refetch. */
export function useServiceWrite(uuid: string) {
  const qc = useQueryClient();
  return (s: Service) => {
    qc.setQueryData(['services', 'detail', uuid], s);
    void qc.invalidateQueries({ queryKey: ['services', 'list'] });
    void qc.invalidateQueries({ queryKey: ['dealer-home'] });
  };
}

export { openWarranty, shareWarranty } from './share';

export function ServiceDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('dealer');
  const links = useLinks();
  const canWrite = useCan('services.write');
  const svc = useService(uuid);
  const onWrite = useServiceWrite(uuid);
  const qc = useQueryClient();
  const [cancelOpen, setCancelOpen] = useState(false);
  const [reason, setReason] = useState('');

  const transition = useMutation({
    mutationFn: ({ status, note }: { status: ServiceStatus; note?: string }) =>
      unwrap(api.POST('/v1/services/{uuid}/transitions', { params: { path: { uuid } }, body: { status, note } })),
    onSuccess: (s) => {
      setCancelOpen(false);
      onWrite(s);
    },
    onError: (e) => {
      if (isApiError(e, 'PHOTO_STANDARD_INCOMPLETE')) {
        void qc.invalidateQueries({ queryKey: intakeKey(uuid) });
        Alert.alert(t('photos_missing_title'), t('photos_missing_body'));
        return;
      }
      Alert.alert(t('action_failed'), errorText(e));
    },
  });

  const removeItem = useMutation({
    mutationFn: (item: string) =>
      unwrap(api.DELETE('/v1/services/{uuid}/items/{item}', { params: { path: { uuid, item } } })),
    onSuccess: onWrite,
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const s = svc.data;

  const confirmTransition = (to: ServiceStatus) => {
    if (to === 'cancelled') {
      setReason('');
      setCancelOpen(true);
      return;
    }
    Alert.alert(t(`transition_confirm.${to}`), t('transition_confirm_body'), [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t(`transition.${to}`),
        style: 'default',
        onPress: () => transition.mutate({ status: to }),
      },
    ]);
  };

  const confirmRemove = (item: string, name: string) => {
    Alert.alert(t('item_remove_confirm'), name, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('item_remove'), style: 'destructive', onPress: () => removeItem.mutate(item) },
    ]);
  };

  return (
    <>
      <Stack.Screen options={{ title: s?.plate || s?.service_no || t('service'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={svc.isRefetching} onRefresh={() => void svc.refetch()} />}>
        {!s ? (
          svc.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={svc.error} onRetry={() => void svc.refetch()} />
          )
        ) : (
          <>
            <View className="gap-2 rounded-lg border border-border bg-card p-4">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="title" selectable className="flex-1">
                  {s.service_no}
                </Text>
                <StatusPill label={t(`service_status.${s.status}`)} tone={serviceTone(s.status)} />
              </View>
              <Text variant="footnote" tone="muted">
                {t('created_at', { date: formatDateTime(s.created_at, i18n.language) })}
                {s.completed_at ? ` · ${t('completed_at', { date: formatDate(s.completed_at, i18n.language) })}` : ''}
              </Text>
              {s.is_warranty_reapply ? (
                <Text variant="footnote" tone="primary">
                  {t('warranty_reapply')}
                </Text>
              ) : null}
              {s.cancel_reason ? (
                <Text variant="footnote" tone="danger">
                  {t('cancel_reason', { reason: s.cancel_reason })}
                </Text>
              ) : null}
              {s.available_transitions.length > 0 ? (
                <View className="mt-2 flex-row flex-wrap gap-2">
                  {s.available_transitions.map((to) => (
                    <Button
                      key={to}
                      title={t(`transition.${to}`)}
                      variant={to === 'cancelled' ? 'outline' : to === 'completed' ? 'primary' : 'secondary'}
                      loading={transition.isPending && transition.variables?.status === to}
                      disabled={transition.isPending}
                      onPress={() => confirmTransition(to)}
                      className="grow"
                    />
                  ))}
                </View>
              ) : null}
            </View>

            <Section title={t('vehicle')}>
              <InfoRow label={t('plate')} value={s.plate} />
              <InfoRow label={t('vehicle')} value={vehicleLabel(s.car_brand?.name, s.car_model?.name, s.model_year)} />
              <InfoRow label={t('vin')} value={s.vin} mono />
              <InfoRow label={t('km')} value={s.km !== null ? new Intl.NumberFormat(i18n.language).format(s.km) : null} />
              <InfoRow label={t('package')} value={s.package} />
              <InfoRow label={t('has_measurement')} value={s.has_measurement ? t('yes') : t('no')} last />
            </Section>

            <IntakePhotos
              uuid={s.uuid}
              canCapture={canWrite && s.editable && s.status !== 'completed' && s.status !== 'cancelled'}
            />

            <Section title={t('customer')}>
              <InfoRow
                label={t('name')}
                value={fullName(s.customer)}
                onPress={s.customer.anonymized ? undefined : () => router.push(links.customer(s.customer.uuid))}
              />
              <InfoRow
                label={t('phone')}
                value={s.customer.phone}
                onPress={s.customer.phone ? () => void Linking.openURL(`tel:${s.customer.phone}`) : undefined}
              />
              <InfoRow label={t('organization')} value={s.organization.name} last />
            </Section>

            <Section
              title={t('items')}
              right={
                canWrite && s.items_editable ? (
                  <Pressable accessibilityRole="button" hitSlop={8} onPress={() => router.push(links.addItem(s.uuid))}>
                    <Text variant="footnote" tone="primary" className="font-semibold">
                      {t('item_add')}
                    </Text>
                  </Pressable>
                ) : undefined
              }>
              {s.items && s.items.length > 0 ? (
                s.items.map((it, i) => (
                  <View
                    key={it.uuid}
                    className={`flex-row items-center gap-3 px-4 py-3 ${i < (s.items?.length ?? 0) - 1 ? 'border-b border-border' : ''}`}>
                    <View className="flex-1 gap-0.5">
                      <Text variant="body" numberOfLines={2}>
                        {it.product.name}
                      </Text>
                      <Text variant="footnote" tone="muted" numberOfLines={2}>
                        {[
                          it.barcode,
                          it.kind === 'partial' && it.meters ? t('meters_value', { value: it.meters }) : null,
                          it.quantity ? t('quantity_value', { value: it.quantity }) : null,
                          it.applied_parts.length ? t('parts_count', { count: it.applied_parts.length }) : null,
                        ]
                          .filter(Boolean)
                          .join(' · ')}
                      </Text>
                    </View>
                    {canWrite && s.items_editable ? (
                      <Pressable
                        accessibilityRole="button"
                        accessibilityLabel={t('item_remove')}
                        hitSlop={8}
                        disabled={removeItem.isPending}
                        onPress={() => confirmRemove(it.uuid, it.product.name)}>
                        <Icon as={Trash} tone="destructive" size={18} />
                      </Pressable>
                    ) : null}
                  </View>
                ))
              ) : (
                <View className="items-center gap-2 px-4 py-5">
                  <Icon as={PackagePlus} tone="muted-foreground" />
                  <Text variant="callout" tone="muted" className="text-center">
                    {t('items_empty')}
                  </Text>
                  {canWrite && s.items_editable ? (
                    <Button title={t('item_add')} variant="secondary" onPress={() => router.push(links.addItem(s.uuid))} />
                  ) : null}
                </View>
              )}
            </Section>

            {s.warranties && s.warranties.length > 0 ? (
              <Section title={t('warranties')}>
                {s.warranties.map((w, i) => (
                  <View
                    key={w.uuid}
                    className={`gap-2 px-4 py-3 ${i < (s.warranties?.length ?? 0) - 1 ? 'border-b border-border' : ''}`}>
                    <Pressable
                      accessibilityRole="button"
                      onPress={() => router.push(links.warranty(w.uuid))}
                      className="flex-row items-center gap-3 active:opacity-70">
                      <Icon as={ShieldCheck} tone="primary" size={20} />
                      <View className="flex-1 gap-0.5">
                        <Text variant="body" numberOfLines={1}>
                          {w.product_name}
                        </Text>
                        <Text variant="footnote" tone="muted">
                          {w.public_code} · {formatDate(w.start_at, i18n.language)} – {formatDate(w.end_at, i18n.language)}
                        </Text>
                      </View>
                      <StatusPill label={t(`warranty_status.${w.status}`)} tone={warrantyTone(w.status)} />
                    </Pressable>
                    <View className="flex-row gap-2">
                      <WarrantyQrButton code={w.public_code} />
                      <Button
                        title={t('warranty_open')}
                        variant="outline"
                        icon={<Icon as={ExternalLink} tone="primary" size={16} />}
                        onPress={() => void openWarranty(w.public_code)}
                        className="grow"
                      />
                      <Button
                        title={t('share')}
                        variant="outline"
                        icon={<Icon as={Share2} tone="primary" size={16} />}
                        onPress={() => shareWarranty(w.public_code, t('warranty_share_message', { code: w.public_code }))}
                        className="grow"
                      />
                    </View>
                  </View>
                ))}
              </Section>
            ) : null}

            {s.notes ? (
              <Section title={t('notes')}>
                <Text variant="callout" selectable className="px-4 py-3">
                  {s.notes}
                </Text>
              </Section>
            ) : null}

            {s.status_logs && s.status_logs.length > 0 ? (
              <Section title={t('history')}>
                {s.status_logs.map((l, i) => (
                  <View
                    key={`${l.created_at}-${i}`}
                    className={`gap-0.5 px-4 py-2.5 ${i < (s.status_logs?.length ?? 0) - 1 ? 'border-b border-border' : ''}`}>
                    <Text variant="callout">
                      {l.from_status ? `${t(`service_status.${l.from_status}`)} → ` : ''}
                      {t(`service_status.${l.to_status}`)}
                    </Text>
                    <Text variant="footnote" tone="muted">
                      {formatDateTime(l.created_at, i18n.language)}
                      {l.by_other_organization ? ` · ${t('by_other_org')}` : ''}
                      {l.note ? ` · ${l.note}` : ''}
                    </Text>
                  </View>
                ))}
              </Section>
            ) : null}
          </>
        )}
      </ScrollView>
      {cancelOpen ? (
        <Sheet
          title={t('transition.cancelled')}
          saveTitle={t('cancel_confirm')}
          busy={transition.isPending}
          onClose={() => setCancelOpen(false)}
          onSave={() => transition.mutate({ status: 'cancelled', note: reason.trim() || undefined })}>
          <Text variant="callout" tone="muted">
            {t('cancel_body')}
          </Text>
          <Input
            label={t('cancel_reason_label')}
            value={reason}
            onChangeText={setReason}
            multiline
            maxLength={500}
            className="min-h-24 py-3"
            autoFocus
          />
        </Sheet>
      ) : null}
    </>
  );
}
