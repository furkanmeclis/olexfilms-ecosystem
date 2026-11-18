import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardAvoidingView, Modal, Platform, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { RefRow } from '@/features/entity/ref-row';
import { Chips } from '@/features/warehouse/parts';
import { confirmAction } from '@/ui/confirm';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { List, SectionHeader } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  canPartialShip,
  receiptDiscrepancies,
  receiptValid,
  shippedUnits,
  shortLines,
  type ReceiptMark,
} from './discrepancy-logic';

type S = components['schemas'];
type Order = S['Order'];

const fail = (e: unknown) => toast.apiError(e);

export const discrepancyApi = {
  receive: (uuid: string, discrepancies: S['OrderDiscrepancyInput'][]) =>
    unwrap(
      api.POST('/v1/orders/{uuid}/transitions', {
        params: { path: { uuid } },
        body: { status: 'received', ...(discrepancies.length ? { discrepancies } : {}) },
      }),
    ),
  partialShip: (uuid: string) =>
    unwrap(api.POST('/v1/orders/{uuid}/partial-ship', { params: { path: { uuid } }, body: {} })),
  resolve: (uuid: string, discrepancy: string, resolution: S['OrderDiscrepancyResolveInput']['resolution']) =>
    unwrap(
      api.POST('/v1/orders/{uuid}/discrepancies/{discrepancy_uuid}/resolve', {
        params: { path: { uuid, discrepancy_uuid: discrepancy } },
        body: { resolution },
      }),
    ),
};

/**
 * TEC-1227: receive with a check. Every shipped unit starts received; a
 * unit can be set missing (stays in transit, the seller is told) or
 * damaged (received and recorded), each with a reason.
 */
export function ReceiptCheckSheet({
  order,
  onClose,
  onDone,
}: {
  order: Order;
  onClose: () => void;
  onDone: (o: Order) => void;
}) {
  const { t } = useTranslation('orders');
  const [marks, setMarks] = useState<Record<string, ReceiptMark>>({});
  const units = shippedUnits(order);
  const m = useMutation({
    mutationFn: () => discrepancyApi.receive(order.uuid, receiptDiscrepancies(marks)),
    onSuccess: (o) => {
      onDone(o);
      onClose();
    },
    onError: fail,
  });
  const mark = (u: string): ReceiptMark => marks[u] ?? { kind: 'ok', reason: '' };
  const set = (u: string, v: ReceiptMark) => setMarks((s) => ({ ...s, [u]: v }));
  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={1}>
            {t('receipt.title')}
          </Text>
          <Button title={t('common:cancel')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-3 px-4 pb-10" keyboardShouldPersistTaps="handled">
          <Text variant="footnote" tone="muted">
            {t('receipt.hint')}
          </Text>
          {units.map((u) => {
            const v = mark(u.unitUuid);
            return (
              <View key={u.unitUuid} className="gap-2 rounded-lg border border-border bg-card p-3">
                <Text variant="callout" numberOfLines={2}>
                  <Text variant="callout" className="font-mono">
                    {u.barcode}
                  </Text>
                  {` · ${u.product}`}
                </Text>
                <Chips
                  value={v.kind}
                  onChange={(k) => set(u.unitUuid, { kind: k, reason: k === 'ok' ? '' : v.reason })}
                  options={(['ok', 'missing', 'damaged'] as const).map((k) => ({ value: k, label: t(`receipt.kind.${k}`) }))}
                />
                {v.kind !== 'ok' ? (
                  <Input
                    label={t('receipt.reason')}
                    placeholder={t('receipt.reason_placeholder')}
                    value={v.reason}
                    maxLength={500}
                    onChangeText={(r) => set(u.unitUuid, { ...v, reason: r })}
                  />
                ) : null}
              </View>
            );
          })}
          <Button
            size="lg"
            title={t('receipt.confirm')}
            loading={m.isPending}
            disabled={!receiptValid(order, marks)}
            onPress={() => m.mutate()}
          />
        </ScrollView>
      </KeyboardAvoidingView>
    </Modal>
  );
}

/** TEC-1227: the receipt records of an order and their resolutions (no prices). */
export function DiscrepancySection({ order, onUpdated }: { order: Order; onUpdated: (o: Order) => void }) {
  const { t } = useTranslation('orders');
  const m = useMutation({
    mutationFn: (v: { uuid: string; resolution: S['OrderDiscrepancyResolveInput']['resolution'] }) =>
      discrepancyApi.resolve(order.uuid, v.uuid, v.resolution),
    onSuccess: onUpdated,
    onError: fail,
  });
  const rows = order.discrepancies ?? [];
  if (!rows.length) return null;
  return (
    <>
      <SectionHeader title={t('receipt.section', { n: order.open_discrepancies ?? 0 })} />
      <View className="gap-2">
        {rows.map((d) => (
          <View key={d.uuid} className="gap-1.5 rounded-lg border border-border bg-card p-3">
            <View className="flex-row items-center justify-between gap-2">
              <Text variant="callout" className="flex-1 font-mono" numberOfLines={1}>
                {d.barcode}
              </Text>
              <StatusPill
                tone={d.status === 'open' ? 'problem' : 'done'}
                label={d.resolution ? t(`receipt.resolution.${d.resolution}`) : t(`receipt.kind.${d.kind}`)}
              />
            </View>
            <Text variant="footnote" tone="muted" numberOfLines={2}>
              {d.product_name} · {d.reason}
            </Text>
            {d.can_resolve_as.length ? (
              <View className="flex-row flex-wrap gap-2">
                {d.can_resolve_as.map((r) => (
                  <Button
                    key={r}
                    variant={r === 'closed' ? 'outline' : 'secondary'}
                    title={t(`receipt.resolve.${r}`)}
                    loading={m.isPending && m.variables?.uuid === d.uuid && m.variables.resolution === r}
                    disabled={m.isPending}
                    onPress={() => m.mutate({ uuid: d.uuid, resolution: r })}
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

/** TEC-1227: the order a backorder was split from and its backorders. */
export function BackorderRows({ order }: { order: Order }) {
  const { t } = useTranslation('orders');
  const links = [
    ...(order.backorder_of ? [{ link: order.backorder_of, label: t('partial.source') }] : []),
    ...(order.backorders ?? []).map((b) => ({ link: b, label: t('partial.backorder') })),
  ];
  if (!links.length) return null;
  return (
    <List>
      {links.map(({ link, label }, i, arr) => (
        <RefRow
          key={link.uuid}
          kind="order"
          label={label}
          value={{ uuid: link.uuid, name: link.order_no, sub: link.status_label }}
          last={i === arr.length - 1}
        />
      ))}
    </List>
  );
}

/**
 * TEC-1227: "Ship partially" on a preparing order with stock short: the
 * picked part stays, the rest becomes a backorder; then onReady opens the
 * ship step.
 */
export function PartialShipButton({ order, onReady }: { order: Order; onReady: (o: Order) => void }) {
  const { t } = useTranslation('orders');
  const qc = useQueryClient();
  const m = useMutation({
    mutationFn: () => discrepancyApi.partialShip(order.uuid),
    onSuccess: (o) => {
      void qc.invalidateQueries({ queryKey: ['warehouse', 'orders'] });
      void qc.invalidateQueries({ queryKey: ['orders', 'list'] });
      onReady(o);
    },
    onError: fail,
  });
  if (!canPartialShip(order)) return null;
  const short = shortLines(order);
  return (
    <Button
      variant="outline"
      title={t('partial.button')}
      loading={m.isPending}
      onPress={() =>
        confirmAction({
          title: t('partial.title'),
          body: [t('partial.description'), ...short.map((l) => t('partial.line', { name: l.name, picked: l.picked, rest: l.rest }))].join('\n'),
          action: t('partial.confirm'),
          onConfirm: () => m.mutate(),
        })
      }
    />
  );
}
