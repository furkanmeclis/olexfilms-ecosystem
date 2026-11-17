import { useQueryClient } from '@tanstack/react-query';
import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { Image } from 'expo-image';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, KeyboardAvoidingView, Linking, Modal, Platform, Pressable, ScrollView, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { useCan } from '@/core/spaces';
import { PickerField, PickerSheet } from '@/features/picker-sheet';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { SectionHeader } from '@/ui/list';
import { Text } from '@/ui/text';

import { entryApi, type StockEntry, type StockEntryAttachment, useArrivalReport, useSuppliers, useUndoCheck, wk } from './api';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { alertError, confirmAction, formatDate, whError } from './format';
import { Card, Chips, Field } from './parts';
import { canSharePdf, sharePanelPdf } from './pdf-share';
import { arrivalChecked, buildDecisions, type DamagedChoice, type ExtraChoice, isIsoDate, unknownExtras } from './receiving';
import { StepUpCard } from './step-up-card';

/** TEC-1225: the delivery of a container entry (supplier, container, waybill, arrival date). */
export function DeliveryCard({ entry, editable, onSaved }: { entry: StockEntry; editable: boolean; onSaved: (x: StockEntry) => void }) {
  const { t } = useTranslation('warehouse');
  const [editing, setEditing] = useState(false);
  return (
    <Card>
      <View className="flex-row items-center gap-2 pb-1">
        <Icon name="truck" tone="primary" size={18} />
        <Text variant="headline" className="flex-1">
          {t('arrival.title')}
        </Text>
        {editable ? <Button variant="ghost" title={t('m.receiving.edit_delivery')} onPress={() => setEditing(true)} /> : null}
      </View>
      <Field label={t('delivery.supplier')} value={entry.supplier?.name} />
      <Field label={t('delivery.container_no')} value={entry.container_no} mono />
      <Field label={t('delivery.waybill_no')} value={entry.waybill_no} mono />
      <Field label={t('delivery.arrival_date')} value={entry.arrival_date} />
      {editing ? <DeliverySheet entry={entry} onClose={() => setEditing(false)} onSaved={onSaved} /> : null}
    </Card>
  );
}

export type DeliveryValue = {
  supplier?: { uuid: string; name: string } | null;
  container: string;
  waybill: string;
  arrival: string;
};

/** Supplier picker + container / waybill / arrival inputs, shared by the new-entry screen. */
export function DeliveryFields({
  value,
  onChange,
  tried,
}: {
  value: DeliveryValue;
  onChange: (v: DeliveryValue) => void;
  tried?: boolean;
}) {
  const { t } = useTranslation('warehouse');
  const [open, setOpen] = useState(false);
  const suppliers = useSuppliers(open);
  const items = suppliers.data?.items ?? [];
  const dateBad = Boolean(tried && value.arrival && !isIsoDate(value.arrival));
  return (
    <View className="gap-3">
      <PickerField
        icon="truck"
        label={t('delivery.supplier')}
        placeholder={t('delivery.pick_supplier')}
        value={value.supplier?.name ?? null}
        onPress={() => setOpen(true)}
      />
      <PickerSheet
        visible={open}
        title={t('delivery.supplier')}
        options={items.map((s) => ({
          key: s.uuid,
          title: s.name,
          subtitle: s.tax_no ?? undefined,
        }))}
        selectedKey={value.supplier?.uuid}
        loading={suppliers.isLoading}
        error={suppliers.error}
        errorMessage={suppliers.error ? whError(suppliers.error) : undefined}
        onRetry={() => void suppliers.refetch()}
        onPick={(o) => onChange({ ...value, supplier: { uuid: o.key, name: o.title } })}
        onClose={() => setOpen(false)}
      />
      <Input
        label={t('delivery.container_no')}
        placeholder={t('delivery.container_no')}
        autoCapitalize="characters"
        maxLength={64}
        value={value.container}
        onChangeText={(v) => onChange({ ...value, container: v })}
      />
      <Input
        label={t('delivery.waybill_no')}
        placeholder={t('delivery.waybill_no')}
        maxLength={64}
        value={value.waybill}
        onChangeText={(v) => onChange({ ...value, waybill: v })}
      />
      <Input
        label={t('delivery.arrival_date')}
        placeholder="2026-11-17"
        keyboardType="numbers-and-punctuation"
        maxLength={10}
        value={value.arrival}
        hint={t('m.receiving.date_hint')}
        error={dateBad ? t('m.receiving.date_invalid') : undefined}
        onChangeText={(v) => onChange({ ...value, arrival: v })}
      />
    </View>
  );
}

function DeliverySheet({ entry, onClose, onSaved }: { entry: StockEntry; onClose: () => void; onSaved: (x: StockEntry) => void }) {
  const { t } = useTranslation('warehouse');
  const [value, setValue] = useState<DeliveryValue>({
    supplier: entry.supplier,
    container: entry.container_no ?? '',
    waybill: entry.waybill_no ?? '',
    arrival: entry.arrival_date ?? '',
  });
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);
  const save = async () => {
    setTried(true);
    if (value.arrival && !isIsoDate(value.arrival)) return;
    setBusy(true);
    try {
      onSaved(
        await entryApi.updateHeader(entry.uuid, {
          supplier_uuid: value.supplier?.uuid ?? '',
          container_no: value.container.trim(),
          waybill_no: value.waybill.trim(),
          arrival_date: value.arrival.trim(),
        }),
      );
      onClose();
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={1}>
            {t('m.receiving.edit_delivery')}
          </Text>
          <Button title={t('common:cancel')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-4 px-4 pb-10" keyboardShouldPersistTaps="handled">
          <DeliveryFields value={value} onChange={setValue} tried={tried} />
          <Button size="lg" title={t('common:save')} loading={busy} onPress={() => void save()} />
        </ScrollView>
      </KeyboardAvoidingView>
    </Modal>
  );
}

/** TEC-1225: arrival scan (expected / received / missing / extras) of a draft. */
export function ArrivalCard({ entry, onChanged }: { entry: StockEntry; onChanged: () => void }) {
  const { t } = useTranslation('warehouse');
  const report = useArrivalReport(entry.uuid, true);
  const [scanning, setScanning] = useState(false);
  const r = report.data;
  const onCode = async (code: string): Promise<ScanOutcome> => {
    try {
      const res = await entryApi.arrivalScan(entry.uuid, code);
      void report.refetch();
      onChanged();
      return {
        ok: res.result !== 'extra',
        title: res.line?.product.name ?? res.barcode,
        detail: `${t(`arrival.result.${res.result}`)} · ${res.scanned}/${res.expected}`,
      };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };
  return (
    <Card>
      <View className="flex-row items-center gap-2 pb-1">
        <Icon name="scan" tone="primary" size={18} />
        <Text variant="headline" className="flex-1">
          {t('m.receiving.arrival_summary')}
        </Text>
      </View>
      <Field label={t('arrival.expected')} value={String(r?.expected ?? entry.line_count)} />
      <Field label={t('arrival.received')} value={String(r?.scanned ?? entry.scanned_count ?? 0)} />
      <Field label={t('receiving.kind.missing')} value={String(r?.missing.length ?? 0)} />
      <Field label={t('arrival.extras')} value={String(r?.extras.length ?? 0)} />
      <Text variant="footnote" tone="muted" className="pt-1">
        {t('arrival.hint')}
      </Text>
      <Button
        className="mt-2"
        variant="secondary"
        title={t('m.receiving.arrival_scan')}
        icon={<Icon name="scan" tone="primary" />}
        onPress={() => setScanning(true)}
      />
      <ContinuousScanner
        visible={scanning}
        onClose={() => setScanning(false)}
        title={t('m.receiving.arrival_scan')}
        hint={t('arrival.scan')}
        onCode={onCode}
        footer={
          <View className="rounded-lg bg-card p-3">
            <Text variant="callout" className="font-semibold">
              {t('m.receiving.arrival_counts', {
                scanned: r?.scanned ?? 0,
                expected: r?.expected ?? 0,
                extras: r?.extras.length ?? 0,
              })}
            </Text>
          </View>
        }
      />
    </Card>
  );
}

/**
 * TEC-1225: confirm with the discrepancy record. Missing lines are left
 * out, extras are added or rejected, damaged lines are taken into stock or
 * not (reason and photo required).
 */
export function ReceivingSheet({
  entry,
  onClose,
  onConfirmed,
}: {
  entry: StockEntry;
  onClose: () => void;
  onConfirmed: (x: StockEntry) => void;
}) {
  const { t } = useTranslation('warehouse');
  const qc = useQueryClient();
  const report = useArrivalReport(entry.uuid, true);
  const [extras, setExtras] = useState<Record<string, ExtraChoice>>({});
  const [damaged, setDamaged] = useState<DamagedChoice[]>([]);
  const [picking, setPicking] = useState(false);
  const [camera, setCamera] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const checked = arrivalChecked(entry, report.data);
  const unknown = unknownExtras(report.data);
  const { decisions, pending } = buildDecisions(report.data, checked, extras, damaged);
  const missing = checked ? (report.data?.missing ?? []).filter((l) => !damaged.some((d) => d.barcode === l.barcode)) : [];
  const lines = (entry.lines ?? []).filter((l) => !damaged.some((d) => d.barcode === l.barcode));
  const patch = (barcode: string, next: Partial<DamagedChoice>) =>
    setDamaged((all) => all.map((d) => (d.barcode === barcode ? { ...d, ...next } : d)));

  const submit = async () => {
    setBusy(true);
    try {
      const next = await entryApi.confirmWith(entry.uuid, decisions);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      onConfirmed(next);
      onClose();
    } catch (e) {
      void report.refetch();
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={1}>
            {t('receiving.confirm_title')}
          </Text>
          <Button title={t('common:cancel')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-4 px-4 pb-10" keyboardShouldPersistTaps="handled">
          <Text variant="callout" tone="muted">
            {t('receiving.confirm_description')}
          </Text>

          {checked ? (
            <View className="gap-2">
              <SectionHeader title={t('receiving.missing_title', { count: missing.length })} />
              {missing.length ? (
                <>
                  <Text variant="footnote" tone="muted">
                    {t('receiving.missing_hint')}
                  </Text>
                  {missing.map((l) => (
                    <View key={l.uuid} className="rounded-lg border border-border bg-card px-4 py-2.5">
                      <Text variant="callout" className="font-semibold" numberOfLines={1}>
                        {l.product.name}
                      </Text>
                      <Text variant="footnote" tone="muted" className="font-mono">
                        {l.barcode}
                      </Text>
                    </View>
                  ))}
                </>
              ) : (
                <Text variant="callout" tone="muted">
                  {t('receiving.none')}
                </Text>
              )}
            </View>
          ) : null}

          <View className="gap-2">
            <SectionHeader
              title={t('receiving.extras_title', {
                count: report.data?.extras.length ?? 0,
              })}
            />
            {(report.data?.extras ?? []).length === 0 ? (
              <Text variant="callout" tone="muted">
                {t('receiving.none')}
              </Text>
            ) : (
              (report.data?.extras ?? []).map((x) => (
                <View key={x.barcode} className="gap-2 rounded-lg border border-border bg-card px-4 py-3">
                  <Text variant="callout" className="font-semibold" numberOfLines={1}>
                    {x.product?.name ?? t('receiving.unknown')}
                  </Text>
                  <Text variant="footnote" tone="muted" className="font-mono">
                    {x.barcode}
                  </Text>
                  <Chips
                    value={extras[x.barcode] ?? ('' as ExtraChoice)}
                    onChange={(v) => setExtras((all) => ({ ...all, [x.barcode]: v }))}
                    options={[
                      ...(unknown.has(x.barcode)
                        ? []
                        : [
                            {
                              value: 'added' as const,
                              label: t('receiving.decision.added'),
                            },
                          ]),
                      {
                        value: 'rejected' as const,
                        label: t('receiving.decision.rejected'),
                      },
                    ]}
                  />
                </View>
              ))
            )}
          </View>

          <View className="gap-2">
            <SectionHeader title={t('receiving.damaged_title')} />
            {damaged.map((d) => (
              <View key={d.barcode} className="gap-2 rounded-lg border border-border bg-card px-4 py-3">
                <View className="flex-row items-center gap-2">
                  <Text variant="callout" className="flex-1 font-mono">
                    {d.barcode}
                  </Text>
                  <Button
                    variant="ghost"
                    title={t('m.remove')}
                    onPress={() => setDamaged((all) => all.filter((x) => x.barcode !== d.barcode))}
                  />
                </View>
                <Chips
                  value={d.decision}
                  onChange={(v) => patch(d.barcode, { decision: v })}
                  options={[
                    {
                      value: 'accepted' as const,
                      label: t('receiving.decision.accepted'),
                    },
                    {
                      value: 'excluded' as const,
                      label: t('receiving.decision.excluded'),
                    },
                  ]}
                />
                <Input
                  label={t('receiving.reason')}
                  placeholder={t('receiving.reason')}
                  value={d.reason}
                  maxLength={500}
                  multiline
                  className="min-h-16 py-3"
                  onChangeText={(v) => patch(d.barcode, { reason: v })}
                />
                <Text variant="footnote" tone={d.attachmentUuids.length ? 'muted' : 'danger'}>
                  {d.attachmentUuids.length
                    ? t('m.receiving.photos_count', {
                        count: d.attachmentUuids.length,
                      })
                    : t('receiving.photos_required')}
                </Text>
                <Button
                  variant="secondary"
                  title={t('receiving.add_photo')}
                  icon={<Icon name="camera" tone="primary" />}
                  onPress={() => setCamera(d.barcode)}
                />
              </View>
            ))}
            <Button
              variant="outline"
              title={t('receiving.add_damaged')}
              icon={<Icon name="add" tone="primary" />}
              disabled={lines.length === 0}
              onPress={() => setPicking(true)}
            />
          </View>

          {pending.length ? (
            <Text variant="callout" tone="danger">
              {t('receiving.pending', { count: pending.length })}
            </Text>
          ) : null}
          <Button
            size="lg"
            title={t('receiving.confirm_with_record')}
            loading={busy}
            disabled={pending.length > 0 || report.isLoading}
            onPress={() => void submit()}
          />
        </ScrollView>
      </KeyboardAvoidingView>
      <PickerSheet
        visible={picking}
        title={t('receiving.add_damaged')}
        options={lines.map((l) => ({
          key: l.barcode,
          title: l.product.name,
          subtitle: l.barcode,
        }))}
        onPick={(o) =>
          setDamaged((all) => [
            ...all,
            {
              barcode: o.key,
              decision: 'accepted',
              reason: '',
              attachmentUuids: [],
            },
          ])
        }
        onClose={() => setPicking(false)}
      />
      {camera ? (
        <EntryPhotoCapture
          entryUuid={entry.uuid}
          onClose={() => setCamera(null)}
          onUploaded={(att) => {
            const barcode = camera;
            setDamaged((all) => all.map((d) => (d.barcode === barcode ? { ...d, attachmentUuids: [...d.attachmentUuids, att.uuid] } : d)));
            void qc.invalidateQueries({ queryKey: wk.entry(entry.uuid) });
          }}
        />
      ) : null}
    </Modal>
  );
}

/** Full-screen camera for one damage photo: take, review, upload (JPEG, kind photo). */
function EntryPhotoCapture({
  entryUuid,
  onClose,
  onUploaded,
}: {
  entryUuid: string;
  onClose: () => void;
  onUploaded: (a: StockEntryAttachment) => void;
}) {
  const { t } = useTranslation('warehouse');
  const insets = useSafeAreaInsets();
  const cam = useRef<CameraView>(null);
  const [permission, request] = useCameraPermissions();
  const [shot, setShot] = useState<string | null>(null);
  const [taking, setTaking] = useState(false);
  const [ready, setReady] = useState(false);
  const [uploading, setUploading] = useState(false);

  const take = async () => {
    if (!cam.current || taking) return;
    setTaking(true);
    try {
      const pic = await cam.current.takePictureAsync({ quality: 0.7 });
      if (pic?.uri) setShot(pic.uri);
    } catch {
      alertError(null);
    } finally {
      setTaking(false);
    }
  };
  const use = async () => {
    if (!shot) return;
    setUploading(true);
    try {
      const att = await entryApi.uploadPhoto(entryUuid, shot);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      onUploaded(att);
      onClose();
    } catch (e) {
      alertError(e);
    } finally {
      setUploading(false);
    }
  };

  return (
    <Modal
      visible
      animationType="slide"
      presentationStyle="fullScreen"
      onRequestClose={() => {
        if (!uploading) onClose();
      }}>
      <View className="flex-1 bg-black" style={{ paddingTop: insets.top, paddingBottom: insets.bottom }}>
        <View className="flex-row items-center gap-3 px-4 py-3">
          <Pressable accessibilityRole="button" accessibilityLabel={t('common:close')} hitSlop={10} disabled={uploading} onPress={onClose}>
            <Icon name="close" tone="primary-foreground" />
          </Pressable>
          <Text variant="headline" className="flex-1 text-white">
            {t('receiving.add_photo')}
          </Text>
        </View>
        <View className="flex-1 overflow-hidden">
          {shot ? (
            <Image source={{ uri: shot }} style={{ flex: 1 }} contentFit="contain" />
          ) : !permission ? null : !permission.granted ? (
            <View className="flex-1 items-center justify-center gap-3 px-8">
              <Text variant="body" className="text-center text-white">
                {t('claims:camera_needed')}
              </Text>
              <Button
                title={permission.canAskAgain ? t('claims:camera_allow') : t('claims:open_settings')}
                onPress={() => void (permission.canAskAgain ? request() : Linking.openSettings())}
              />
            </View>
          ) : (
            <CameraView ref={cam} style={{ flex: 1 }} facing="back" onCameraReady={() => setReady(true)} />
          )}
        </View>
        <View className="flex-row items-center justify-center gap-4 px-4 py-5">
          {shot ? (
            <>
              <Button
                title={t('claims:photo_retake')}
                variant="secondary"
                onPress={() => setShot(null)}
                disabled={uploading}
                className="grow"
              />
              <Button
                title={t('claims:photo_use')}
                icon={<Icon name="addPhoto" tone="primary-foreground" size={18} />}
                loading={uploading}
                onPress={() => void use()}
                className="grow"
              />
            </>
          ) : permission?.granted ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('claims:photo_take')}
              disabled={!ready || taking}
              onPress={() => void take()}
              className="size-[72px] items-center justify-center rounded-full border-4 border-white active:opacity-70">
              {taking ? <ActivityIndicator colorClassName="accent-on-media" /> : <View className="size-14 rounded-full bg-white" />}
            </Pressable>
          ) : null}
        </View>
      </View>
    </Modal>
  );
}

/** TEC-1225: the discrepancy record of a confirmed entry and its PDF. */
export function ReceiptCard({ entry }: { entry: StockEntry }) {
  const { t } = useTranslation('warehouse');
  const [sharing, setSharing] = useState(false);
  if (entry.status === 'draft' || entry.mode === 'import') return null;
  const items = entry.discrepancies ?? [];
  const share = async () => {
    setSharing(true);
    try {
      await sharePanelPdf(entryApi.receiptPdfPath(entry.uuid), {}, `tutanak-${entry.container_no ?? entry.uuid.slice(0, 8)}.pdf`);
    } catch (e) {
      alertError(e);
    } finally {
      setSharing(false);
    }
  };
  return (
    <View className="gap-2">
      <SectionHeader title={t('receiving.record_title')} />
      <Card>
        {items.length === 0 ? (
          <Text variant="callout" tone="muted">
            {t('receiving.record_clean')}
          </Text>
        ) : (
          items.map((d) => (
            <View key={d.uuid} className="gap-0.5 border-b border-border py-2 last:border-b-0">
              <Text variant="callout" className="font-semibold">
                {t(`receiving.kind.${d.kind}`)} · {t(`receiving.decision.${d.decision}`)}
              </Text>
              <Text variant="footnote" tone="muted" numberOfLines={2}>
                <Text variant="footnote" tone="muted" className="font-mono">
                  {d.barcode}
                </Text>
                {` · ${d.product?.name ?? t('receiving.unknown')}`}
                {d.reason ? ` · ${d.reason}` : ''}
              </Text>
            </View>
          ))
        )}
        {canSharePdf ? (
          <Button
            className="mt-2"
            variant="secondary"
            icon="share"
            title={t('receiving.record_pdf')}
            loading={sharing}
            onPress={() => void share()}
          />
        ) : null}
      </Card>
    </View>
  );
}

/**
 * TEC-1225: undo a confirmed manual entry (stock.adjust + step-up). Units
 * that moved after the entry block the undo and are listed.
 */
export function UndoCard({ entry, onUndone }: { entry: StockEntry; onUndone: (x: StockEntry) => void }) {
  const { t } = useTranslation('warehouse');
  const adjust = useCan('stock.adjust');
  const write = useCan('warehouse.write');
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
  const [stepUp, setStepUp] = useState(false);
  const check = useUndoCheck(entry.uuid, open);
  if (!adjust || !write || entry.status !== 'confirmed' || entry.mode === 'import') return null;
  const blockers = check.data?.blockers ?? [];
  const undo = async () => {
    try {
      const next = await entryApi.undo(entry.uuid, reason.trim());
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      setOpen(false);
      setStepUp(false);
      onUndone(next);
    } catch (e) {
      setStepUp(false);
      void check.refetch();
      alertError(e);
    }
  };
  if (!open) {
    return (
      <Button variant="outline" icon={<Icon name="undo" tone="foreground" />} title={t('receiving.undo')} onPress={() => setOpen(true)} />
    );
  }
  return (
    <Card>
      <Text variant="headline">{t('receiving.undo_title')}</Text>
      <Text variant="footnote" tone="muted">
        {t('receiving.undo_description', {
          count: check.data?.lines ?? entry.line_count,
        })}
      </Text>
      {blockers.length ? (
        <View className="gap-1 pt-2">
          <Text variant="callout" tone="danger" className="font-semibold">
            {t('receiving.undo_blocked', { count: blockers.length })}
          </Text>
          {blockers.map((b) => (
            <Text key={b.barcode} variant="footnote" tone="muted">
              <Text variant="footnote" className="font-mono">
                {b.barcode}
              </Text>
              {` · ${b.product.name} · ${t(`receiving.movement.${b.movement_type}`, { defaultValue: b.movement_type })} · ${formatDate(b.at)}`}
            </Text>
          ))}
        </View>
      ) : null}
      <Input
        className="mt-2"
        label={t('receiving.undo_reason')}
        placeholder={t('receiving.undo_reason')}
        value={reason}
        maxLength={500}
        onChangeText={setReason}
      />
      <View className="gap-2 pt-2">
        {stepUp ? (
          <StepUpCard action={t('receiving.undo_confirm')} onVerified={undo} onCancel={() => setStepUp(false)} />
        ) : (
          <Button
            variant="destructive"
            title={t('receiving.undo_confirm')}
            disabled={!check.data?.allowed || !reason.trim()}
            onPress={() =>
              confirmAction({
                title: t('receiving.undo_title'),
                body: t('receiving.undo_description', {
                  count: check.data?.lines ?? entry.line_count,
                }),
                action: t('receiving.undo_confirm'),
                destructive: true,
                onConfirm: () => setStepUp(true),
              })
            }
          />
        )}
        <Button variant="ghost" title={t('common:cancel')} onPress={() => setOpen(false)} />
      </View>
    </Card>
  );
}
