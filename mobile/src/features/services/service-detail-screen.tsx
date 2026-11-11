import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Linking, Pressable, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useCan } from '@/core/spaces';
import { ServiceMeasurementMaps } from '@/features/measure/point-map-view';
import { VehiclePartPicker } from '@/features/vehicle-parts';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { PullRefresh } from '@/ui/pull-refresh';

import { ServiceContractCard, useContractRequiredAlert } from './contract-card';
import { IntakePhotos, intakeKey } from './intake-photos';
import {
  formatDate,
  formatDateTime,
  formatNumber,
  fullName,
  type Service,
  type ServiceStatus,
  useLinks,
  vehicleLabel,
  warrantyTone,
} from './lib';
import { WarrantyQrButton } from './qr-sheet';
import { ServiceHero } from './service-hero';
import { openWarranty, shareWarranty } from './share';
import { Sheet } from './sheet';
import { DetailSkeleton, ErrorState, errorText, InfoRow, Section } from './ui';
import { partsFromItems } from './wizard';

export function useService(uuid: string, enabled = true) {
  return useQuery({
    queryKey: ['services', 'detail', uuid],
    enabled,
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

/**
 * One service for staff, top to bottom: the hero (car, plate, status,
 * customer), what to do next (continue the intake wizard, status
 * transitions), then customer, vehicle, products, body parts, warranties,
 * photos, measurements, notes and history. Actions follow the backend:
 * `available_transitions`, `items_editable` and `editable` already carry the
 * caller's role and the service state.
 */
export function ServiceDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('dealer');
  const links = useLinks();
  const canWrite = useCan('services.write');
  const canLinkMeasurements = useCan('measurements.link');
  const canReadMeasurements = useCan('measurements.read');
  const canMeasurements = canLinkMeasurements && canReadMeasurements;
  const { t: tm } = useTranslation('measure');
  const svc = useService(uuid);
  const onWrite = useServiceWrite(uuid);
  const qc = useQueryClient();
  const [cancelOpen, setCancelOpen] = useState(false);
  const [reason, setReason] = useState('');
  const [openItem, setOpenItem] = useState<string | null>(null);

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
      if (isApiError(e, 'VALIDATION_ERROR') && e.details?.some((d) => d.field === 'items')) {
        Alert.alert(t('action_failed'), t('complete_needs_items'));
        return;
      }
      // TEC-1083: the move needs a signed intake contract; the card below shows how to sign.
      if (isApiError(e, 'CONTRACT_REQUIRED')) {
        void svc.refetch();
        contractRequired();
        return;
      }
      Alert.alert(t('action_failed'), errorText(e));
    },
  });
  const contractRequired = useContractRequiredAlert(uuid);

  const removeItem = useMutation({
    mutationFn: (item: string) =>
      unwrap(api.DELETE('/v1/services/{uuid}/items/{item}', { params: { path: { uuid, item } } })),
    onSuccess: onWrite,
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const s = svc.data;
  // The backend refuses to complete a service without items (VALIDATION_ERROR on `items`).
  const noItems = !s?.items?.length;
  // Parts of every item on one diagram; an item row opens its own parts.
  const appliedParts = partsFromItems(s?.items);
  const itemsEditable = !!s && canWrite && s.items_editable;
  // The intake wizard continues a draft whose items may still change (web: "Continue in wizard").
  const canContinue = !!s && canWrite && s.status === 'draft' && s.items_editable;
  const transitions = s?.available_transitions ?? [];
  const forward = transitions.filter((to) => to !== 'cancelled');

  const confirmTransition = (to: ServiceStatus) => {
    if (to === 'cancelled') {
      setReason('');
      setCancelOpen(true);
      return;
    }
    Alert.alert(t(`transition_confirm.${to}`), to === 'completed' ? t('wizard.complete_body') : t('transition_confirm_body'), [
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
        contentContainerClassName="gap-5 px-4 pb-12 pt-2"
        refreshControl={<PullRefresh busy={svc.isRefetching} onRefresh={() => void svc.refetch()} />}>
        {!s ? (
          svc.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={svc.error} onRetry={() => void svc.refetch()} />
          )
        ) : (
          <>
            <ServiceHero s={s}>
              <View className="gap-1">
                <View className="flex-row items-center gap-2">
                  <Text variant="footnote" tone="muted">
                    {t('service_no')}
                  </Text>
                  <Text variant="footnote" selectable className="font-mono font-semibold" style={{ writingDirection: 'ltr' }}>
                    {s.service_no}
                  </Text>
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
              </View>
            </ServiceHero>

            {canContinue || transitions.length > 0 ? (
              <View className="gap-2">
                <Text variant="caption" tone="muted" className="px-1">
                  {t('next_steps')}
                </Text>
                {canContinue ? (
                  <Pressable
                    accessibilityRole="button"
                    onPress={() => router.push(links.wizard(s.uuid))}
                    className="flex-row items-center gap-3 rounded-2xl border border-primary/40 bg-card px-4 py-3.5 active:opacity-80"
                    style={{ borderCurve: 'continuous' }}>
                    <View className="size-9 items-center justify-center rounded-full bg-primary/10">
                      <Icon name="edit" tone="primary" size={17} />
                    </View>
                    <View className="min-w-0 flex-1 gap-0.5">
                      <Text variant="body" className="font-semibold">
                        {t('wizard.resume')}
                      </Text>
                      <Text variant="footnote" tone="muted" numberOfLines={2}>
                        {t('wizard.resume_hint')}
                      </Text>
                    </View>
                    <Icon name="forward" tone="muted-foreground" size={16} />
                  </Pressable>
                ) : null}
                {forward.length > 0 ? (
                  <View className="flex-row flex-wrap gap-2">
                    {forward.map((to) => (
                      <Button
                        key={to}
                        title={t(`transition.${to}`)}
                        variant={to === 'completed' ? 'primary' : 'secondary'}
                        size="lg"
                        loading={transition.isPending && transition.variables?.status === to}
                        disabled={transition.isPending || (to === 'completed' && noItems)}
                        onPress={() => confirmTransition(to)}
                        className="grow basis-[45%]"
                      />
                    ))}
                  </View>
                ) : null}
                {noItems && transitions.includes('completed') ? (
                  <Text variant="footnote" tone="muted" className="px-1">
                    {t('complete_needs_items')}
                  </Text>
                ) : null}
              </View>
            ) : null}

            <ServiceContractCard s={s} onChanged={onWrite} />

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

            <Section title={t('vehicle')}>
              <InfoRow label={t('plate')} value={s.plate} mono />
              <InfoRow label={t('vehicle')} value={vehicleLabel(s.car_brand?.name, s.car_model?.name, s.model_year)} />
              <InfoRow label={t('vin')} value={s.vin} mono />
              <InfoRow label={t('km')} value={s.km !== null ? formatNumber(s.km, i18n.language) : null} />
              <InfoRow label={t('package')} value={s.package} />
              <InfoRow label={t('has_measurement')} value={s.has_measurement ? t('yes') : t('no')} last />
            </Section>

            <Section
              title={t('items')}
              right={
                itemsEditable && s.items?.length ? (
                  <Pressable
                    accessibilityRole="button"
                    hitSlop={8}
                    onPress={() => router.push(links.addItem(s.uuid))}
                    className="flex-row items-center gap-1">
                    <Icon name="add" tone="primary" size={14} weight="semibold" />
                    <Text variant="footnote" tone="primary" className="font-semibold">
                      {t('item_add')}
                    </Text>
                  </Pressable>
                ) : undefined
              }>
              {s.items && s.items.length > 0 ? (
                s.items.map((it, i) => (
                  <View key={it.uuid} className={cn(i < (s.items?.length ?? 0) - 1 && 'border-b border-border')}>
                    <View className="flex-row items-center gap-3 px-4 py-3">
                      <Pressable
                        accessibilityRole="button"
                        accessibilityState={{ expanded: openItem === it.uuid }}
                        disabled={!it.applied_parts.length}
                        onPress={() => setOpenItem((cur) => (cur === it.uuid ? null : it.uuid))}
                        className="min-w-0 flex-1 flex-row items-center gap-3 active:opacity-70">
                        <View className="size-9 items-center justify-center rounded-full bg-secondary">
                          <Icon name="barcode" tone="primary" size={16} />
                        </View>
                        <View className="min-w-0 flex-1 gap-0.5">
                          <Text variant="body" numberOfLines={2}>
                            {it.product.name}
                          </Text>
                          <Text variant="footnote" tone="muted" numberOfLines={2}>
                            {[
                              it.barcode,
                              it.kind === 'partial' && it.meters ? t('meters_value', { value: it.meters }) : null,
                              it.quantity ? t('quantity_value', { value: formatNumber(it.quantity, i18n.language) }) : null,
                              it.applied_parts.length ? t('parts_count', { count: it.applied_parts.length }) : null,
                            ]
                              .filter(Boolean)
                              .join(' · ')}
                          </Text>
                        </View>
                        {it.applied_parts.length ? (
                          <Icon name={openItem === it.uuid ? 'chevronUp' : 'chevronDown'} tone="muted-foreground" size={16} />
                        ) : null}
                      </Pressable>
                      {itemsEditable ? (
                        <Pressable
                          accessibilityRole="button"
                          accessibilityLabel={t('item_remove')}
                          hitSlop={8}
                          disabled={removeItem.isPending}
                          onPress={() => confirmRemove(it.uuid, it.product.name)}>
                          <Icon name="delete" tone="destructive" size={18} />
                        </Pressable>
                      ) : null}
                    </View>
                    {openItem === it.uuid ? (
                      <VehiclePartPicker value={it.applied_parts} readOnly framed={false} className="px-3 pb-3" />
                    ) : null}
                  </View>
                ))
              ) : (
                <View className="items-center gap-2 px-4 py-5">
                  <Icon name="packageIn" tone="muted-foreground" />
                  <Text variant="callout" tone="muted" className="text-center">
                    {t('items_empty')}
                  </Text>
                  {itemsEditable ? (
                    <Button title={t('item_add')} variant="secondary" icon="add" onPress={() => router.push(links.addItem(s.uuid))} />
                  ) : null}
                </View>
              )}
            </Section>

            {appliedParts.length > 0 ? (
              <Section title={t('applied_parts')}>
                <VehiclePartPicker value={appliedParts} readOnly framed={false} className="p-3" />
              </Section>
            ) : null}

            {s.warranties && s.warranties.length > 0 ? (
              <Section title={t('warranties')}>
                {s.warranties.map((w, i) => (
                  <View key={w.uuid} className={cn('gap-2.5 px-4 py-3', i < (s.warranties?.length ?? 0) - 1 && 'border-b border-border')}>
                    <Pressable
                      accessibilityRole="button"
                      onPress={() => router.push(links.warranty(w.uuid))}
                      className="flex-row items-center gap-3 active:opacity-70">
                      <View className="size-9 items-center justify-center rounded-full bg-primary/10">
                        <Icon name="warranty" tone="primary" size={17} />
                      </View>
                      <View className="min-w-0 flex-1 gap-0.5">
                        <Text variant="body" numberOfLines={1}>
                          {w.product_name}
                        </Text>
                        <Text variant="footnote" tone="muted">
                          <Text variant="footnote" tone="muted" className="font-mono">
                            {w.public_code}
                          </Text>
                          {` · ${formatDate(w.start_at, i18n.language)} – ${formatDate(w.end_at, i18n.language)}`}
                        </Text>
                      </View>
                      <StatusPill label={t(`warranty_status.${w.status}`)} tone={warrantyTone(w.status)} />
                    </Pressable>
                    <View className="flex-row flex-wrap gap-2">
                      <WarrantyQrButton code={w.public_code} />
                      <Button
                        title={t('warranty_open')}
                        variant="outline"
                        icon={<Icon name="externalLink" tone="primary" size={16} />}
                        onPress={() => void openWarranty(w.public_code)}
                        className="grow"
                      />
                      <Button
                        title={t('share')}
                        variant="outline"
                        icon={<Icon name="share" tone="primary" size={16} />}
                        onPress={() => shareWarranty(w.public_code, t('warranty_share_message', { code: w.public_code }))}
                        className="grow"
                      />
                    </View>
                  </View>
                ))}
              </Section>
            ) : null}

            <IntakePhotos
              uuid={s.uuid}
              canCapture={canWrite && s.editable && s.status !== 'completed' && s.status !== 'cancelled'}
            />

            {s.has_measurement && canMeasurements ? (
              <Section title={tm('service_measurements')}>
                <ServiceMeasurementMaps serviceUuid={s.uuid} />
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
                {s.status_logs.map((l, i) => {
                  const last = i === (s.status_logs?.length ?? 0) - 1;
                  return (
                    <View key={`${l.created_at}-${i}`} className="flex-row gap-3 px-4">
                      <View className="items-center pt-4">
                        <View className={cn('size-2.5 rounded-full', i === 0 ? 'bg-primary' : 'bg-muted-foreground/50')} />
                        {!last ? <View className="mt-1 w-px flex-1 bg-border" /> : null}
                      </View>
                      <View className={cn('min-w-0 flex-1 gap-0.5 py-2.5', !last && 'border-b border-border')}>
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
                    </View>
                  );
                })}
              </Section>
            ) : null}

            {transitions.includes('cancelled') ? (
              <Button
                title={t('transition.cancelled')}
                variant="ghost"
                icon={<Icon name="blocked" tone="destructive" size={16} />}
                disabled={transition.isPending}
                onPress={() => confirmTransition('cancelled')}
                className="self-center"
              />
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
            placeholder={t('placeholder_cancel_reason')}
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
