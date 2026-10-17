import { useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { entryApi, resolveScan, type StockEntry, useEntry, wk } from './api';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { confirmAction, entryTone, formatDate, formatNumber, isCode, whError } from './format';
import { Card, ErrorState, Field, IconButton, ListSkeleton } from './parts';
import { ReserveSheet } from './reserve-sheet';
import { useDocAction } from './use-action';

/**
 * Stock entry (goods receipt): scan a bin QR, then the printed labels of the
 * units going onto it; each label is added and placed at once. Confirm posts
 * the stock.
 */
export function EntryScreen() {
  const { t } = useTranslation('warehouse');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const qc = useQueryClient();
  const write = useCan('warehouse.write');
  const q = useEntry(uuid);
  const { busy, run } = useDocAction<StockEntry>(wk.entry(uuid));
  const [scanning, setScanning] = useState(false);
  const [bin, setBin] = useState<{ uuid: string; code: string } | null>(null);
  const [reserving, setReserving] = useState(false);

  const x = q.data;
  const lines = (x?.lines ?? []).filter((l) => !l.undone);
  const unplaced = lines.filter((l) => !l.location);
  const store = (next: StockEntry) => qc.setQueryData(wk.entry(uuid), next);

  const onCode = async (code: string): Promise<ScanOutcome> => {
    if (!x?.warehouse) return { ok: false, title: t('entries.no_warehouse') };
    try {
      // A label the resolver does not know yet still goes to the entry by its code.
      const r = await resolveScan(code, { warehouse: x.warehouse.uuid }).catch((e: unknown) => {
        if (isCode(e, 'SCAN_NO_MATCH')) return null;
        throw e;
      });
      if (r?.status === 'ok' && r.type === 'location' && r.location) {
        setBin({ uuid: r.location.uuid, code: r.location.full_code });
        return { ok: true, title: t('m.target_set', { location: r.location.full_code }) };
      }
      if (r?.status === 'warehouse_mismatch') {
        return { ok: false, title: t('scan_status.warehouse_mismatch', { warehouse: r.location?.path[0]?.name ?? '' }) };
      }
      if (r?.status === 'ok' && r.type === 'product') return { ok: false, title: t('m.scan_unit_not_product') };
      const barcode = r?.unit?.barcode ?? code;
      const onEntry = lines.some((l) => l.barcode === barcode);
      if (onEntry) {
        // Already a line (reserved or scanned before): only its shelf changes.
        if (!bin) return { ok: false, title: t('order_place.scan_location_first') };
        store(await entryApi.place(uuid, { barcodes: [barcode], location_uuid: bin.uuid }));
      } else {
        if (x.mode === 'generate_new') return { ok: false, title: t('m.not_on_entry', { barcode }) };
        let next = await entryApi.addLines(uuid, { barcodes: [barcode] });
        if (bin) next = await entryApi.place(uuid, { barcodes: [barcode], location_uuid: bin.uuid });
        store(next);
      }
      return { ok: true, title: r?.product?.name ?? barcode, detail: bin ? `→ ${bin.code}` : t('entry.unplaced') };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  if (!x) {
    return (
      <>
        <Stack.Screen options={{ title: t('entry.title'), headerLargeTitle: false }} />
        <Screen>{q.isLoading ? <ListSkeleton rows={4} /> : <ErrorState error={q.error} onRetry={() => void q.refetch()} />}</Screen>
      </>
    );
  }

  const draft = x.status === 'draft';

  return (
    <>
      <Stack.Screen options={{ title: t('entry.title'), headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        <Card>
          <View className="flex-row items-center gap-2 pb-1">
            <Icon name="packageIn" tone="primary" size={18} />
            <Text variant="headline" className="flex-1" numberOfLines={2}>
              {x.warehouse?.name ?? t('entries.no_warehouse')}
            </Text>
            <StatusPill tone={entryTone(x.status)} label={t(`entry_status.${x.status}`)} />
          </View>
          <Field label={t('fields.mode')} value={t(`entry_mode.${x.mode}`)} />
          <Field label={t('entry.lines')} value={t('entries.placed_of', { placed: lines.length - unplaced.length, total: lines.length })} />
          <Field label={t('fields.note')} value={x.note} />
          <Field label={t('m.created')} value={formatDate(x.created_at)} />
          <Field label={t('entry_status.confirmed')} value={formatDate(x.confirmed_at)} />
          <Field label={t('entry_status.cancelled')} value={formatDate(x.cancelled_at)} />
        </Card>

        {write && draft ? (
          <View className="gap-2">
            {x.mode === 'generate_new' ? (
              <Button size="lg" variant="secondary" title={t('entry.generate')} onPress={() => setReserving(true)} />
            ) : null}
            <Button
              size="lg"
              title={x.mode === 'generate_new' ? t('m.scan_to_place') : t('m.scan_labels')}
              icon={<Icon name="scan" tone="primary-foreground" />}
              onPress={() => setScanning(true)}
            />
            <View className="flex-row gap-2">
              <Button
                className="flex-1"
                variant="secondary"
                title={t('entry.confirm')}
                loading={busy === 'confirm'}
                disabled={lines.length === 0}
                onPress={() =>
                  confirmAction({
                    title: t('entry.confirm_title'),
                    body: unplaced.length
                      ? `${t('entry.confirm_description')}\n\n${t('entry.confirm_blocked')}`
                      : t('entry.confirm_description'),
                    action: t('entry.confirm'),
                    onConfirm: () => void run('confirm', () => entryApi.confirm(uuid)),
                  })
                }
              />
              <Button
                className="flex-1"
                variant="outline"
                title={t('entry.cancel')}
                loading={busy === 'cancel'}
                onPress={() =>
                  confirmAction({
                    title: t('entry.cancel_title'),
                    body: t('entry.cancel_description'),
                    action: t('entry.cancel'),
                    destructive: true,
                    onConfirm: () => void run('cancel', () => entryApi.cancel(uuid)),
                  })
                }
              />
            </View>
            <Text variant="footnote" tone="muted">
              {t('m.entry_flow_hint')}
            </Text>
          </View>
        ) : null}

        <SectionHeader title={`${t('entry.lines')} (${lines.length})`} />
        {lines.length === 0 ? (
          <Text variant="callout" tone="muted" className="px-4">
            {t('entry.no_lines')}
          </Text>
        ) : (
          <View className="gap-2">
            {lines.map((l) => (
              <View key={l.uuid} className="flex-row items-center gap-2 rounded-lg border border-border bg-card py-2.5 pe-1 ps-4">
                <View className="flex-1 gap-0.5">
                  <Text variant="callout" className="font-semibold" numberOfLines={1}>
                    {l.product.name}
                  </Text>
                  <Text variant="footnote" tone="muted" numberOfLines={1}>
                    <Text variant="footnote" tone="muted" className="font-mono">
                      {l.barcode}
                    </Text>
                    {l.unit_kind === 'fixed' ? ` × ${formatNumber(l.quantity)}` : ''}
                    {` · ${l.location?.full_code ?? l.location?.code ?? t('entry.unplaced')}`}
                  </Text>
                </View>
                {write && draft ? (
                  <IconButton
                    icon="delete"
                    tone="destructive"
                    label={t('entry.remove_line', { barcode: l.barcode })}
                    busy={busy === `del:${l.uuid}`}
                    onPress={() => void run(`del:${l.uuid}`, () => entryApi.deleteLine(uuid, l.uuid))}
                  />
                ) : null}
              </View>
            ))}
          </View>
        )}
      </Screen>

      {reserving ? (
        <ReserveSheet
          onClose={() => setReserving(false)}
          onSubmit={async (v) => store(await entryApi.addLines(uuid, v))}
        />
      ) : null}
      <ContinuousScanner
        visible={scanning}
        onClose={() => setScanning(false)}
        title={t('m.scan_labels')}
        hint={t('entry.scan_barcode_hint')}
        onCode={onCode}
        footer={
          <View className="gap-2 rounded-lg bg-card p-3">
            <Text variant="callout" className="font-semibold">
              {bin ? t('m.target_bin', { location: bin.code }) : t('m.entry_no_bin')}
            </Text>
            {bin && unplaced.length ? (
              <Button
                variant="secondary"
                title={t('m.place_rest', { count: unplaced.length })}
                loading={busy === 'place'}
                onPress={() =>
                  void run('place', () => entryApi.place(uuid, { line_uuids: unplaced.map((l) => l.uuid), location_uuid: bin.uuid }))
                }
              />
            ) : null}
          </View>
        }
      />
    </>
  );
}
