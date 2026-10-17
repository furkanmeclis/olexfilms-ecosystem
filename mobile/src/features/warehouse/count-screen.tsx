import { useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { useCan } from '@/core/spaces';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  countApi,
  type CountResolution,
  type StockCount,
  type StockCountLine,
  useCount,
  useCountReport,
  useCountScans,
  wk,
} from './api';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { confirmAction, countTone, formatDate, formatNumber, isCode, whError } from './format';
import { Card, Chips, ErrorState, Field, IconButton, ListSkeleton } from './parts';
import { QtyStepper } from './qty-stepper';
import { useDocAction } from './use-action';

/** Quantity is asked for fixed barcodes and SKUs (web rule: product_qty, unit_first). */
const showsQuantity = (c: StockCount) => c.method === 'product_qty' || c.method === 'unit_first';

function defaultResolution(line: StockCountLine): CountResolution | undefined {
  if (line.resolution && line.allowed_resolutions.includes(line.resolution)) return line.resolution;
  if (line.allowed_resolutions.includes('ignore')) return 'ignore';
  return line.allowed_resolutions[0];
}

/** Stock count: start, scan into it, finish, then review and approve the differences. */
export function CountScreen() {
  const { t } = useTranslation('warehouse');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const qc = useQueryClient();
  const write = useCan('warehouse.write');
  const adjust = useCan('stock.adjust');
  const me = useSession((s) => s.me?.user.uuid);
  const q = useCount(uuid);
  const c = q.data;
  const counting = c?.status === 'in_progress';
  const reviewing = c?.status === 'pending_review' || c?.status === 'approved';
  const scans = useCountScans(uuid, !!c && (counting || c.status === 'pending_review'));
  const report = useCountReport(uuid, !!c && reviewing);
  const { busy, run } = useDocAction<StockCount>(wk.count(uuid));
  const [scanning, setScanning] = useState(false);
  const [qty, setQty] = useState(1);
  const [context, setContext] = useState<string | null>(null);
  const [choices, setChoices] = useState<Record<string, CountResolution>>({});

  const refreshAll = () => {
    void q.refetch();
    if (counting) void scans.refetch();
    if (reviewing) void report.refetch();
  };

  const onCode = async (code: string): Promise<ScanOutcome> => {
    const send = async (withQty: boolean) =>
      countApi.scan(uuid, { code, quantity: withQty && qty > 1 ? qty : undefined });
    try {
      let r;
      try {
        r = await send(true);
      } catch (e) {
        // A serial unit or a location takes no quantity: count it as one.
        if (qty > 1 && isCode(e, 'VALIDATION_ERROR')) r = await send(false);
        else throw e;
      }
      if (r.location_context) setContext(r.location_context.full_code);
      void qc.invalidateQueries({ queryKey: wk.countScans(uuid) });
      void qc.invalidateQueries({ queryKey: wk.count(uuid) });
      const s = r.scan;
      if (s.kind === 'location') return { ok: true, title: t('count.context', { location: s.location?.full_code ?? s.raw_code }) };
      if (s.kind === 'fixed' || s.kind === 'product') setQty(1);
      const exp = r.expected;
      const detail =
        c?.visibility === 'guided' && exp
          ? exp.in_scope
            ? `${t('count.expected')}: ${formatNumber(exp.quantity)}${exp.location ? ` · ${exp.location.full_code}` : ''}`
            : t('count.not_expected_here')
          : s.location?.full_code;
      return {
        ok: true,
        title: t('count.last_scan', { code: s.product?.name ?? s.raw_code, quantity: s.quantity }),
        detail,
      };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  if (!c) {
    return (
      <>
        <Stack.Screen options={{ title: t('count.title'), headerLargeTitle: false }} />
        <Screen>{q.isLoading ? <ListSkeleton rows={4} /> : <ErrorState error={q.error} onRetry={() => void q.refetch()} />}</Screen>
      </>
    );
  }

  const scope =
    c.scope_type === 'warehouse'
      ? t('count_scope.warehouse')
      : `${t(`count_scope.${c.scope_type}`)}: ${c.scope_room?.name ?? c.scope_location?.full_code ?? c.scope_product?.name ?? ''}`;
  const needsStartApproval = c.status === 'draft' && c.start_approval_required && !c.start_approved_at;
  const open = c.status === 'draft' || c.status === 'in_progress' || c.status === 'pending_review';
  const toResolve = (report.data?.lines ?? []).filter((l) => l.result !== 'matched');
  const selfCreated = !!me && c.created_by_user_uuid === me;

  const approve = () => {
    const resolutions: { line_uuid: string; resolution: CountResolution }[] = [];
    for (const l of toResolve) {
      const r = choices[l.uuid] ?? defaultResolution(l);
      if (!r) return Alert.alert(t('count.resolve_all'));
      resolutions.push({ line_uuid: l.uuid, resolution: r });
    }
    confirmAction({
      title: t('count.approve_title'),
      body: t('count.approve_description', { count: resolutions.length }),
      action: t('count.approve'),
      onConfirm: () =>
        void run('approve', async () => {
          const rep = await countApi.approve(uuid, { resolutions });
          qc.setQueryData(wk.countReport(uuid), rep);
          return rep.count;
        }),
    });
  };

  return (
    <>
      <Stack.Screen options={{ title: t('count.title'), headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={refreshAll} />}>
        <Card>
          <View className="flex-row items-center gap-2 pb-1">
            <Icon name="order" tone="primary" size={18} />
            <Text variant="headline" className="flex-1" numberOfLines={2}>
              {c.warehouse.name}
            </Text>
            <StatusPill tone={countTone(c.status)} label={t(`count_status.${c.status}`)} />
          </View>
          <Field label={t('fields.scope')} value={scope} />
          <Field label={t('fields.method')} value={t(`count_method.${c.method}`)} />
          <Field label={t('fields.visibility')} value={t(`count_visibility.${c.visibility}`)} />
          <Field label={t('fields.note')} value={c.note} />
          <Field label={t('m.created')} value={formatDate(c.created_at)} />
          <Field label={t('m.started')} value={formatDate(c.started_at)} />
          <Field label={t('m.completed')} value={formatDate(c.completed_at)} />
          {c.progress ? (
            <Field
              label={t('count.progress')}
              value={t('count.progress_text', {
                scans: c.progress.scans,
                units: c.progress.serial_units,
                quantity: c.progress.fixed_quantity + c.progress.product_quantity,
              })}
            />
          ) : null}
          {c.visibility === 'guided' && c.expected ? (
            <Field
              label={t('count.expected_title')}
              value={t('count.expected_totals', { units: c.expected.serial_units, quantity: c.expected.fixed_quantity })}
            />
          ) : null}
          {c.summary ? (
            <Field label={t('count.report_title')} value={t('count.summary', { lines: c.summary.lines, unresolved: c.summary.unresolved })} />
          ) : null}
        </Card>

        {write && open ? (
          <View className="gap-2">
            {c.status === 'draft' ? (
              <>
                {needsStartApproval ? (
                  <>
                    <Text variant="footnote" tone="muted">
                      {t('count.approval_needed')}
                    </Text>
                    <Button
                      size="lg"
                      title={t('count.approve_start')}
                      loading={busy === 'approve_start'}
                      onPress={() => void run('approve_start', () => countApi.approveStart(uuid))}
                    />
                  </>
                ) : (
                  <Button size="lg" title={t('count.start')} loading={busy === 'start'} onPress={() => void run('start', () => countApi.start(uuid))} />
                )}
                <Text variant="footnote" tone="muted">
                  {t(`count_method_hint.${c.method}`)}
                </Text>
              </>
            ) : null}
            {counting ? (
              <>
                <Button size="lg" title={t('count.scan_title')} icon={<Icon name="scan" tone="primary-foreground" />} onPress={() => setScanning(true)} />
                <Button
                  variant="secondary"
                  title={t('count.complete')}
                  loading={busy === 'complete'}
                  onPress={() =>
                    confirmAction({
                      title: t('count.complete_title'),
                      body: t('count.complete_description'),
                      action: t('count.complete'),
                      onConfirm: () =>
                        void run('complete', () => countApi.complete(uuid), {
                          done: () => void qc.invalidateQueries({ queryKey: wk.countReport(uuid) }),
                        }),
                    })
                  }
                />
              </>
            ) : null}
            {c.status === 'pending_review' && adjust ? (
              <>
                {selfCreated ? (
                  <Text variant="footnote" tone="muted">
                    {t('count.self_approval_hint')}
                  </Text>
                ) : null}
                <Button size="lg" title={t('count.approve')} loading={busy === 'approve'} disabled={!report.data} onPress={approve} />
              </>
            ) : null}
            <Button
              variant="outline"
              title={t('count.cancel')}
              loading={busy === 'cancel'}
              onPress={() =>
                confirmAction({
                  title: t('count.cancel_title'),
                  body: t('count.cancel_description'),
                  action: t('count.cancel'),
                  destructive: true,
                  onConfirm: () => void run('cancel', () => countApi.cancel(uuid)),
                })
              }
            />
          </View>
        ) : null}

        {reviewing ? (
          <>
            <SectionHeader title={t('count.report_title')} />
            {report.isLoading ? (
              <ListSkeleton rows={3} />
            ) : report.error && !report.data ? (
              <ErrorState error={report.error} onRetry={() => void report.refetch()} />
            ) : toResolve.length === 0 ? (
              <Text variant="callout" tone="muted" className="px-4">
                {t('count.no_differences')}
              </Text>
            ) : (
              <View className="gap-2">
                {toResolve.map((l) => (
                  <View key={l.uuid} className="gap-2 rounded-lg border border-border bg-card px-4 py-3">
                    <View className="flex-row items-start gap-2">
                      <View className="flex-1 gap-0.5">
                        <Text variant="callout" className="font-semibold" numberOfLines={2}>
                          {l.product.name}
                        </Text>
                        <Text variant="footnote" tone="muted" className="font-mono" numberOfLines={1}>
                          {l.unit?.barcode ?? l.product.sku}
                        </Text>
                      </View>
                      <StatusPill tone="problem" label={t(`count_result.${l.result}`)} />
                    </View>
                    <Text variant="footnote" tone="muted">
                      {t('count.expected')}: {formatNumber(l.expected_quantity)}
                      {l.expected_location ? ` · ${l.expected_location.full_code}` : ''}
                      {'  ·  '}
                      {t('count.counted')}: {formatNumber(l.counted_quantity)}
                      {l.counted_location ? ` · ${l.counted_location.full_code}` : ''}
                    </Text>
                    {c.status === 'pending_review' && adjust && l.allowed_resolutions.length ? (
                      <Chips
                        value={choices[l.uuid] ?? defaultResolution(l) ?? 'ignore'}
                        onChange={(v) => setChoices((s) => ({ ...s, [l.uuid]: v }))}
                        options={l.allowed_resolutions.map((r) => ({ value: r, label: t(`count_resolution.${r}`) }))}
                      />
                    ) : l.resolution ? (
                      <Text variant="footnote">
                        {t('count.resolution')}: {t(`count_resolution.${l.resolution}`)}
                      </Text>
                    ) : null}
                  </View>
                ))}
              </View>
            )}
          </>
        ) : null}

        {counting || c.status === 'pending_review' ? (
          <>
            <SectionHeader title={`${t('count.scans_title')} (${scans.data?.total ?? 0})`} />
            {scans.isLoading ? (
              <ListSkeleton rows={3} />
            ) : (scans.data?.items.length ?? 0) === 0 ? (
              <Text variant="callout" tone="muted" className="px-4">
                {t('count.no_scans')}
              </Text>
            ) : (
              <View className="gap-2">
                {scans.data?.items.map((s) => (
                  <View key={s.uuid} className="flex-row items-center gap-2 rounded-lg border border-border bg-card py-2.5 pe-1 ps-4">
                    <View className="flex-1 gap-0.5">
                      <Text variant="callout" className="font-semibold" numberOfLines={1}>
                        {s.kind === 'location' ? (s.location?.full_code ?? s.raw_code) : (s.product?.name ?? s.raw_code)}
                      </Text>
                      <Text variant="footnote" tone="muted" numberOfLines={1}>
                        {s.kind === 'location' ? t('scan.type.location') : t(`count_scan_kind.${s.kind}`)}
                        {s.kind !== 'location' ? ` · ${s.unit?.barcode ?? s.raw_code} × ${s.quantity}` : ''}
                        {s.kind !== 'location' && s.location ? ` · ${s.location.full_code}` : ''}
                        {` · ${formatDate(s.created_at)}`}
                      </Text>
                    </View>
                    {write && counting ? (
                      <IconButton
                        icon="delete"
                        tone="destructive"
                        label={t('count.remove_scan', { code: s.raw_code })}
                        busy={busy === `del:${s.uuid}`}
                        onPress={() =>
                          void run(`del:${s.uuid}`, () => countApi.deleteScan(uuid, s.uuid), {
                            done: () => {
                              void scans.refetch();
                              void q.refetch();
                            },
                          })
                        }
                      />
                    ) : null}
                  </View>
                ))}
              </View>
            )}
          </>
        ) : null}
      </Screen>

      <ContinuousScanner
        visible={scanning}
        onClose={() => {
          setScanning(false);
          void scans.refetch();
          void q.refetch();
        }}
        title={t('count.scan_title')}
        hint={t(`count_method_hint.${c.method}`)}
        onCode={onCode}
        footer={
          <>
            {showsQuantity(c) ? <QtyStepper label={t('count.quantity_hint')} value={qty} onChange={setQty} /> : null}
            <Text variant="footnote" className="px-1 text-white/90">
              {context ? t('count.context', { location: context }) : t('count.no_context')}
            </Text>
          </>
        }
      />
    </>
  );
}
