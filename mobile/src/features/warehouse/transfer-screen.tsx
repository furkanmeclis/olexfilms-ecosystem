import { useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { useCan } from '@/core/spaces';
import { transferMissing } from '@/features/orders/discrepancy-logic';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { PullRefresh } from '@/ui/pull-refresh';

import { resolveScan, type Transfer, transferApi, useTransfer, wk } from './api';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { confirmAction, formatDate, formatNumber, transferTone, whError } from './format';
import { Card, ErrorState, Field, IconButton, ListSkeleton } from './parts';
import { QtyStepper } from './qty-stepper';
import { locationPath } from './scan-result-card';
import { useDocAction } from './use-action';

type Loc = { uuid: string; code: string };

/** Warehouse transfer: lines, scanning units in, shipping and receiving. */
export function TransferScreen() {
  const { t } = useTranslation('warehouse');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const q = useTransfer(uuid);
  const write = useCan('warehouse.write');
  const qc = useQueryClient();
  const { busy, run } = useDocAction<Transfer>(wk.transfer(uuid));
  const [scanning, setScanning] = useState<'add' | 'receive' | null>(null);
  const [qty, setQty] = useState(1);
  const [source, setSource] = useState<Loc | null>(null);
  const [target, setTarget] = useState<Loc | null>(null);
  // TEC-1227: lines marked as not arrived; they stay in transit under the record.
  const [missing, setMissing] = useState<Set<string>>(new Set());
  const toggleMissing = (line: string) =>
    setMissing((m) => {
      const n = new Set(m);
      if (n.has(line)) n.delete(line);
      else n.add(line);
      return n;
    });

  const x = q.data;
  const lines = x?.lines ?? [];
  const unplaced = lines.filter((l) => !l.target_location && !missing.has(l.uuid));
  // Receiving needs a target for every line: per line, the transfer's own target, or a scanned bin.
  const receiveBlocked = unplaced.length > 0 && !x?.to_location && !target;

  const store = (next: Transfer) => qc.setQueryData(wk.transfer(uuid), next);

  /** Draft: a location QR picks the source bin for fixed barcodes; units are added. */
  const onAdd = async (code: string): Promise<ScanOutcome> => {
    if (!x) return { ok: false, title: t('scan.failed') };
    try {
      const r = await resolveScan(code, { warehouse: x.from_warehouse.uuid });
      if (r.status !== 'ok') return { ok: false, title: t(`scan_status.${r.status}`, { warehouse: '', actual: '', expected: '' }) };
      if (r.type === 'location' && r.location) {
        setSource({ uuid: r.location.uuid, code: r.location.full_code });
        return { ok: true, title: t('m.source_set', { location: r.location.full_code }) };
      }
      if (r.type !== 'unit' || !r.unit) return { ok: false, title: t('m.scan_unit_not_product') };
      const u = r.unit;
      if (u.unit_kind === 'fixed') {
        const src = source ?? (u.location ? { uuid: u.location.uuid, code: u.location.full_code } : null);
        if (!src) return { ok: false, title: t('fixed.source_scan') };
        store(
          await transferApi.addLines(uuid, {
            fixed: [{ barcode: u.barcode, quantity: qty, source_location_uuid: src.uuid }],
          }),
        );
        return { ok: true, title: `${r.product?.name ?? u.barcode} × ${qty}`, detail: src.code };
      }
      store(await transferApi.addLines(uuid, { barcodes: [u.barcode] }));
      return { ok: true, title: r.product?.name ?? u.barcode, detail: u.location ? locationPath(u.location) : undefined };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  /** In transit: a location QR of the target warehouse is the target; units go onto it. */
  const onReceive = async (code: string): Promise<ScanOutcome> => {
    if (!x) return { ok: false, title: t('scan.failed') };
    try {
      const r = await resolveScan(code, { warehouse: x.to_warehouse.uuid });
      if (r.status !== 'ok') {
        return {
          ok: false,
          title: t(`scan_status.${r.status}`, { warehouse: x.to_warehouse.name, actual: '', expected: '' }),
        };
      }
      if (r.type === 'location' && r.location) {
        setTarget({ uuid: r.location.uuid, code: r.location.full_code });
        return { ok: true, title: t('m.target_set', { location: r.location.full_code }) };
      }
      if (r.type !== 'unit' || !r.unit) return { ok: false, title: t('m.scan_unit_not_product') };
      const tgt = target;
      if (!tgt) return { ok: false, title: t('order_place.scan_location_first') };
      if (!lines.some((l) => l.barcode === r.unit!.barcode)) {
        return { ok: false, title: t('m.not_on_transfer', { barcode: r.unit.barcode }) };
      }
      store(await transferApi.place(uuid, { barcodes: [r.unit.barcode], location_uuid: tgt.uuid }));
      return { ok: true, title: r.product?.name ?? r.unit.barcode, detail: `→ ${tgt.code}` };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  if (!x) {
    return (
      <>
        <Stack.Screen options={{ title: t('transfer.title'), headerLargeTitle: false }} />
        <Screen>{q.isLoading ? <ListSkeleton rows={4} /> : <ErrorState error={q.error} onRetry={() => void q.refetch()} />}</Screen>
      </>
    );
  }

  const draft = x.status === 'draft';
  const transit = x.status === 'in_transit';

  return (
    <>
      <Stack.Screen options={{ title: x.transfer_no, headerLargeTitle: false }} />
      <Screen refreshControl={<PullRefresh busy={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        <Card>
          <View className="flex-row items-center gap-2 pb-1">
            <Icon name="truck" tone="primary" size={18} />
            <Text variant="headline" className="flex-1" numberOfLines={2}>
              {x.from_warehouse.name} → {x.to_warehouse.name}
            </Text>
            <StatusPill tone={transferTone(x.status)} label={t(`transfer_status.${x.status}`)} />
          </View>
          <Field label={t('transfer.target_title')} value={x.to_location?.full_code ?? x.to_location?.code} mono />
          <Field label={t('fields.note')} value={x.note} />
          <Field label={t('m.created')} value={formatDate(x.created_at)} />
          <Field label={t('m.shipped')} value={formatDate(x.shipped_at)} />
          <Field label={t('m.completed')} value={formatDate(x.completed_at)} />
          <Field label={t('m.cancelled')} value={formatDate(x.cancelled_at)} />
        </Card>

        {write && (draft || transit) ? (
          <View className="gap-2">
            {draft ? (
              <>
                <Button size="lg" title={t('m.scan_units')} icon={<Icon name="scan" tone="primary-foreground" />} onPress={() => setScanning('add')} />
                <View className="flex-row gap-2">
                  <Button
                    className="flex-1"
                    variant="secondary"
                    title={t('transfer.ship')}
                    loading={busy === 'ship'}
                    disabled={lines.length === 0}
                    onPress={() =>
                      confirmAction({
                        title: t('transfer.ship_title'),
                        body: t('transfer.ship_description'),
                        action: t('transfer.ship'),
                        onConfirm: () => void run('ship', () => transferApi.ship(uuid)),
                      })
                    }
                  />
                  <CancelButton busy={busy === 'cancel'} onCancel={() => void run('cancel', () => transferApi.cancel(uuid))} />
                </View>
                {lines.length === 0 ? (
                  <Text variant="footnote" tone="muted">
                    {t('transfer.ship_blocked')}
                  </Text>
                ) : null}
              </>
            ) : (
              <>
                <Button
                  size="lg"
                  title={t('m.scan_to_receive')}
                  icon={<Icon name="location" tone="primary-foreground" />}
                  onPress={() => setScanning('receive')}
                />
                <View className="flex-row gap-2">
                  <Button
                    className="flex-1"
                    variant="secondary"
                    title={t('transfer.complete')}
                    loading={busy === 'complete'}
                    // The backend refuses to complete while a line has no target location.
                    disabled={receiveBlocked || missing.size >= lines.length}
                    onPress={() =>
                      confirmAction({
                        title: t('transfer.complete_title'),
                        body: missing.size
                          ? t('discrepancy.complete_missing', { n: missing.size })
                          : t('transfer.complete_description'),
                        action: t('transfer.complete'),
                        onConfirm: () =>
                          void run(
                            'complete',
                            () =>
                              transferApi.complete(uuid, {
                                ...(target && unplaced.length ? { location_uuid: target.uuid } : {}),
                                ...(missing.size ? { missing: transferMissing(lines, arrivedOf(lines, missing)) } : {}),
                              }),
                            { done: () => setMissing(new Set()) },
                          ),
                      })
                    }
                  />
                  <CancelButton busy={busy === 'cancel'} onCancel={() => void run('cancel', () => transferApi.cancel(uuid))} />
                </View>
                {unplaced.length ? (
                  <Text variant="footnote" tone="muted">
                    {receiveBlocked ? `${t('transfer.complete_blocked')} ` : ''}
                    {t('transfer.target_hint', { warehouse: x.to_warehouse.name, count: unplaced.length })}
                  </Text>
                ) : null}
              </>
            )}
          </View>
        ) : null}

        <TransferDiscrepancies transfer={x} writable={write} busy={busy} run={run} />

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
                    {l.unit_kind === 'fixed' && l.quantity ? ` × ${formatNumber(l.quantity)}` : ''}
                  </Text>
                  <Text variant="footnote" tone="muted" numberOfLines={1}>
                    {l.source_location?.full_code ?? l.source_location?.code ?? '—'} →{' '}
                    {l.target_location?.full_code ?? l.target_location?.code ?? t('entry.unplaced')}
                  </Text>
                </View>
                {write && transit ? (
                  <Button
                    variant={missing.has(l.uuid) ? 'destructive' : 'outline'}
                    title={missing.has(l.uuid) ? t('discrepancy.kind.missing') : t('discrepancy.mark_missing')}
                    onPress={() => toggleMissing(l.uuid)}
                  />
                ) : null}
                {write && draft ? (
                  <IconButton
                    icon="delete"
                    tone="destructive"
                    label={t('entry.remove_line', { barcode: l.barcode })}
                    busy={busy === `del:${l.uuid}`}
                    onPress={() =>
                      confirmAction({
                        title: t('entry.remove_line_confirm', { barcode: l.barcode }),
                        action: t('m.remove'),
                        destructive: true,
                        onConfirm: () => void run(`del:${l.uuid}`, () => transferApi.deleteLine(uuid, l.uuid)),
                      })
                    }
                  />
                ) : null}
              </View>
            ))}
          </View>
        )}
      </Screen>

      <ContinuousScanner
        visible={scanning === 'add'}
        onClose={() => setScanning(null)}
        title={t('m.scan_units')}
        hint={t('transfer.scan_unit_hint')}
        onCode={onAdd}
        footer={
          <>
            <QtyStepper label={t('m.fixed_qty')} value={qty} onChange={setQty} />
            <Text variant="footnote" className="px-1 text-white/90">
              {source ? t('m.source_bin', { location: source.code }) : t('m.source_hint')}
            </Text>
          </>
        }
      />
      <ContinuousScanner
        visible={scanning === 'receive'}
        onClose={() => setScanning(null)}
        title={t('m.scan_to_receive')}
        hint={t('order_place.scan_location_hint')}
        onCode={onReceive}
        footer={
          <View className="gap-2 rounded-lg bg-card p-3">
            <Text variant="callout" className="font-semibold">
              {target ? t('m.target_bin', { location: target.code }) : t('order_place.no_target')}
            </Text>
            {target && unplaced.length ? (
              <Button
                variant="secondary"
                title={t('m.place_rest', { count: unplaced.length })}
                loading={busy === 'place'}
                onPress={() =>
                  void run('place', () =>
                    transferApi.place(uuid, { line_uuids: unplaced.map((l) => l.uuid), location_uuid: target.uuid }),
                  )
                }
              />
            ) : null}
          </View>
        }
      />
    </>
  );
}

/** The arrived lines: every line not marked missing. */
function arrivedOf(lines: { uuid: string }[], missing: Set<string>): Set<string> {
  return new Set(lines.filter((l) => !missing.has(l.uuid)).map((l) => l.uuid));
}

/**
 * TEC-1227: the discrepancy record of a completed transfer. A missing line
 * is found (placed on its target in the target warehouse) or returned to
 * its source; a surplus row is closed.
 */
function TransferDiscrepancies({
  transfer,
  writable,
  busy,
  run,
}: {
  transfer: Transfer;
  writable: boolean;
  busy: string | null;
  run: (name: string, fn: () => Promise<Transfer>) => Promise<void>;
}) {
  const { t } = useTranslation('warehouse');
  const rows = transfer.discrepancies ?? [];
  if (!rows.length) return null;
  return (
    <>
      <SectionHeader title={t('discrepancy.card_count', { n: transfer.open_discrepancies })} />
      <View className="gap-2">
        {rows.map((d) => (
          <View key={d.uuid} className="gap-1.5 rounded-lg border border-border bg-card p-3">
            <View className="flex-row items-center justify-between gap-2">
              <Text variant="callout" className="flex-1 font-mono" numberOfLines={1}>
                {d.barcode}
              </Text>
              <StatusPill
                tone={d.status === 'open' ? 'problem' : 'done'}
                label={d.resolution ? t(`discrepancy.resolution.${d.resolution}`) : t(`discrepancy.kind.${d.kind}`)}
              />
            </View>
            {d.product_name || d.note ? (
              <Text variant="footnote" tone="muted" numberOfLines={2}>
                {[d.product_name, d.note].filter(Boolean).join(' · ')}
              </Text>
            ) : null}
            {writable && d.status === 'open' ? (
              <View className="flex-row flex-wrap gap-2">
                {(d.kind === 'missing' ? (['found', 'returned_to_source'] as const) : (['closed'] as const)).map((r) => (
                  <Button
                    key={r}
                    variant={r === 'found' ? 'secondary' : 'outline'}
                    title={t(`discrepancy.resolve.${r}`)}
                    loading={busy === `disc:${d.uuid}:${r}`}
                    disabled={!!busy}
                    onPress={() => void run(`disc:${d.uuid}:${r}`, () => transferApi.resolveDiscrepancy(transfer.uuid, d.uuid, { resolution: r }))}
                  />
                ))}
              </View>
            ) : null}
          </View>
        ))}
      </View>
    </>
  );
}

function CancelButton({ busy, onCancel }: { busy: boolean; onCancel: () => void }) {
  const { t } = useTranslation('warehouse');
  return (
    <Button
      className="flex-1"
      variant="outline"
      title={t('transfer.cancel')}
      loading={busy}
      onPress={() =>
        confirmAction({
          title: t('transfer.cancel_title'),
          body: t('transfer.cancel_description'),
          action: t('transfer.cancel'),
          destructive: true,
          onConfirm: onCancel,
        })
      }
    />
  );
}
