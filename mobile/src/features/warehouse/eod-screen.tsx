import { useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, RefreshControl, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { CardRow, Chips, ErrorState, ListSkeleton, PagedList, useChildHref, useFormat } from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { useCurrentWarehouse } from './context';
import {
  type EodGroup,
  eodKeys,
  type EodReport,
  type EodScope,
  type EodTotals,
  generateEodReport,
  shareEodPdf,
  useEodReport,
  useEodReports,
} from './eod-api';
import { alertError, go } from './format';
import { Card, Field } from './parts';
import { canSharePdf } from './pdf-share';

const GROUP_ICONS: Record<EodGroup, IconName> = {
  entry: 'packageIn',
  placement: 'location',
  transfer: 'transfer',
  order: 'order',
  consumption: 'service',
  return: 'back',
  adjustment: 'edit',
  disposal: 'delete',
};

function HeaderIcon({ icon, label, onPress }: { icon: IconName; label: string; onPress: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      hitSlop={8}
      onPress={onPress}
      className="size-8 items-center justify-center">
      <Icon name={icon} tone="primary" />
    </Pressable>
  );
}

function reportTitle(r: EodReport, system: string) {
  return r.warehouse ? `${r.warehouse.name} · ${r.warehouse.code}` : system;
}

/** End-of-day reports: newest day first, the system report before the warehouse reports. */
export function EodListScreen() {
  const { t } = useTranslation('warehouse');
  const f = useFormat();
  const href = useChildHref();
  const qc = useQueryClient();
  const write = useCan('warehouse.write');
  const { warehouse } = useCurrentWarehouse();
  const [scope, setScope] = useState<EodScope>('all');
  const [busy, setBusy] = useState(false);
  const list = useEodReports(scope);

  const generate = async (warehouseUuid?: string) => {
    setBusy(true);
    try {
      const r = await generateEodReport(warehouseUuid);
      qc.setQueryData(eodKeys.report(r.uuid), r);
      void qc.invalidateQueries({ queryKey: ['warehouse', 'eod', 'list'] });
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      router.push(href(r.uuid));
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  const askGenerate = () =>
    Alert.alert(t('eod.generate_title'), t('eod.generate_description'), [
      { text: t('eod.scope.system'), onPress: () => void generate() },
      ...(warehouse ? [{ text: warehouse.name, onPress: () => void generate(warehouse.uuid) }] : []),
      { text: t('common:cancel'), style: 'cancel' as const },
    ]);

  return (
    <>
      <Stack.Screen
        options={{
          title: t('eod.title'),
          headerLargeTitle: false,
          headerRight: () => (
            <View className="flex-row items-center gap-3">
              <HeaderIcon icon="settings" label={t('settings.title')} onPress={() => go('/depo/eod/settings')} />
              {write ? (
                <HeaderIcon icon="add" label={t('eod.generate')} onPress={busy ? () => {} : askGenerate} />
              ) : null}
            </View>
          ),
        }}
      />
      <PagedList
        query={list}
        keyOf={(r) => r.uuid}
        header={
          <Chips
            options={(['all', 'system', 'warehouse'] as EodScope[]).map((k) => ({ key: k, label: t(`eod.scope.${k}`) }))}
            value={scope}
            onChange={setScope}
          />
        }
        empty={{ icon: 'report', title: t('eod.empty_title'), body: t('eod.empty_description') }}
        renderItem={({ item }) => (
          <CardRow
            title={f.date(item.report_date)}
            subtitle={reportTitle(item, t('eod.scope.system'))}
            meta={t('eod.row_meta', {
              moves: f.number(item.summary.totals.movement_count),
              units: f.number(item.summary.totals.unit_count),
            })}
            trailing={<StatusPill label={t(`eod.kind.${item.kind}`)} tone={item.kind === 'manual' ? 'progress' : 'neutral'} />}
            onPress={() => router.push(href(item.uuid))}
          />
        )}
      />
    </>
  );
}

function TotalsCard({ totals }: { totals: EodTotals }) {
  const { t } = useTranslation('warehouse');
  const f = useFormat();
  const cells = [
    { key: 'moves', label: t('eod.totals.movements'), value: f.number(totals.movement_count) },
    { key: 'units', label: t('eod.totals.units'), value: f.number(totals.unit_count) },
    { key: 'in', label: t('eod.totals.in'), value: f.number(totals.quantity_in) },
    { key: 'out', label: t('eod.totals.out'), value: f.number(totals.quantity_out) },
  ];
  return (
    <View className="gap-2.5">
      <View className="flex-row flex-wrap gap-2.5">
        {cells.map((c) => (
          <View key={c.key} className="grow basis-[46%] gap-1 rounded-lg border border-border bg-card p-3">
            <Text variant="title" className="tabular-nums">
              {c.value}
            </Text>
            <Text variant="footnote" tone="muted">
              {c.label}
            </Text>
          </View>
        ))}
      </View>
      <Card>
        <Field label={t('eod.totals.meters_in')} value={`${f.number(totals.meters_in, 2)} m`} />
        <Field label={t('eod.totals.meters_out')} value={`${f.number(totals.meters_out, 2)} m`} />
      </Card>
    </View>
  );
}

function inOut(f: ReturnType<typeof useFormat>, x: EodTotals) {
  const parts = [`+${f.number(x.quantity_in)}`, `−${f.number(x.quantity_out)}`];
  if (Number(x.meters_in) || Number(x.meters_out)) parts.push(`${f.number(x.meters_in, 2)} / ${f.number(x.meters_out, 2)} m`);
  return parts.join(' · ');
}

export function EodDetailScreen() {
  const { t } = useTranslation('warehouse');
  const f = useFormat();
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const q = useEodReport(uuid);
  const [sharing, setSharing] = useState(false);
  const r = q.data;

  const abort = useRef<AbortController | null>(null);
  useEffect(() => () => abort.current?.abort(), []);

  const share = async () => {
    if (!r) return;
    abort.current?.abort();
    const ctrl = new AbortController();
    abort.current = ctrl;
    setSharing(true);
    try {
      await shareEodPdf(r, ctrl.signal);
    } catch (e) {
      if (!ctrl.signal.aborted) alertError(e);
    } finally {
      if (!ctrl.signal.aborted) setSharing(false);
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: r ? f.date(r.report_date) : t('eod.title'), headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton />
        ) : !r ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <Card>
              <Text variant="headline">{reportTitle(r, t('eod.scope.system'))}</Text>
              <Field label={t('eod.period')} value={`${f.dateTime(r.period_start)} – ${f.dateTime(r.period_end)}`} />
              <Field label={t('eod.timezone')} value={r.timezone} />
              <Field label={t('eod.generated_at')} value={`${f.dateTime(r.generated_at)} · ${t(`eod.kind.${r.kind}`)}`} />
            </Card>

            <TotalsCard totals={r.summary.totals} />

            {r.summary.groups.length ? (
              <>
                <SectionHeader title={t('eod.groups')} />
                <List>
                  {r.summary.groups.map((g, i) => (
                    <ListRow
                      key={g.group}
                      icon={GROUP_ICONS[g.group]}
                      title={t(`eod.group.${g.group}`)}
                      subtitle={inOut(f, g)}
                      trailing={
                        <Text variant="callout" tone="muted" className="tabular-nums">
                          {f.number(g.movement_count)}
                        </Text>
                      }
                      last={i === r.summary.groups.length - 1}
                    />
                  ))}
                </List>
              </>
            ) : (
              <Text variant="callout" tone="muted">
                {t('eod.no_movements')}
              </Text>
            )}

            {r.summary.products.length ? (
              <>
                <SectionHeader title={t('eod.products')} />
                <List>
                  {r.summary.products.map((p, i) => (
                    <ListRow
                      key={`${p.product_uuid}-${p.type}`}
                      title={p.product_name}
                      subtitle={`${p.sku} · ${t(`stock.movement.${p.type}`, { defaultValue: p.type })} · ${inOut(f, p)}`}
                      last={i === r.summary.products.length - 1}
                    />
                  ))}
                </List>
              </>
            ) : null}

            {canSharePdf ? (
              <Button
                title={t('eod.share_pdf')}
                icon="share"
                variant="secondary"
                size="lg"
                loading={sharing}
                onPress={() => void share()}
              />
            ) : null}
          </>
        )}
      </Screen>
    </>
  );
}
