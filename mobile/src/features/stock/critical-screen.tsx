import { useQuery } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { useCan } from '@/core/spaces';
import { formatNumber } from '@/features/warehouse/format';
import { ListStatus, Segmented } from '@/features/warehouse/parts';
import { PullRefresh } from '@/ui/pull-refresh';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { measureText } from './stock-screen';

type S = components['schemas'];
type Rule = S['StockReorderRule'];
type Forecast = S['StockForecast'];
type View_ = 'critical' | 'forecast';

/** TEC-1229: tone of a reorder level. */
export function levelTone(level: string): StatusTone {
  if (level === 'below_min') return 'problem';
  if (level === 'reorder') return 'progress';
  if (level === 'days_low') return 'neutral';
  return 'done';
}

/**
 * Critical stock and forecasts (TEC-1229): the reorder rules whose level is
 * not ok (GET /v1/stock/critical, warehouse balance: available = on hand -
 * reserved) and, with the stock forecast add-on, the forecast list
 * (GET /v1/stock-forecasts). Same numbers as the web. No prices.
 */
export function CriticalStockScreen() {
  const { t } = useTranslation('warehouse');
  const canForecast = useCan('stock_forecast.read');
  const [view, setView] = useState<View_>('critical');
  const critical = useQuery({
    queryKey: ['stock', 'critical'],
    queryFn: () => unwrap(api.GET('/v1/stock/critical', { params: { query: { limit: 100 } } })),
  });
  const forecasts = useQuery({
    queryKey: ['stock', 'forecasts'],
    enabled: canForecast && view === 'forecast',
    queryFn: () => unwrap(api.GET('/v1/stock-forecasts', { params: { query: { limit: 100, sort: 'days_left' } } })),
  });
  const active = view === 'critical' ? critical : forecasts;
  const header = canForecast ? (
    <Segmented
      value={view}
      onChange={setView}
      options={[
        { value: 'critical', label: t('m.critical_stock') },
        { value: 'forecast', label: t('m.forecasts') },
      ]}
    />
  ) : null;
  return (
    <>
      <Stack.Screen options={{ title: t('m.critical_stock') }} />
      <FlatList<Rule | Forecast>
        data={(view === 'critical' ? critical.data?.items : forecasts.data?.items) ?? []}
        keyExtractor={(item) => item.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10 pt-2"
        refreshControl={<PullRefresh busy={active.isRefetching} onRefresh={() => void active.refetch()} />}
        ListHeaderComponent={header ? <View className="pb-2">{header}</View> : undefined}
        ListEmptyComponent={
          <ListStatus
            isLoading={active.isLoading}
            error={active.error}
            empty
            onRetry={() => void active.refetch()}
            emptyIcon="stock"
            emptyTitle={view === 'critical' ? t('m.critical_empty') : t('m.forecasts_empty')}
          />
        }
        renderItem={({ item }) =>
          view === 'critical' ? <RuleRow rule={item as Rule} /> : <ForecastRow forecast={item as Forecast} />
        }
      />
    </>
  );
}

/**
 * TEC-1243 (K14): the box thresholds count closed boxes (a roll_meter
 * product, which has no boxes, keeps counting its rolls); the meter
 * threshold (open rolls + cut pieces) shows and alerts apart.
 */
export function ruleBoxes(rule: Pick<Rule, 'product' | 'balance'>): number {
  const m = rule.balance.measures;
  return rule.product.unit_type === 'roll_meter' || !m ? rule.balance.available_quantity : m.available_boxes;
}

function RuleRow({ rule }: { rule: Rule }) {
  const { t } = useTranslation('warehouse');
  const measure = measureText(t, rule.balance.measures);
  const meterLow = rule.meter_level === 'below_min';
  return (
    <View className="gap-1.5 rounded-lg border border-border bg-card px-4 py-3">
      <View className="flex-row items-center gap-2">
        <Text variant="headline" className="flex-1" numberOfLines={2}>
          {rule.product.name}
        </Text>
        <Text variant="title">{formatNumber(ruleBoxes(rule))}</Text>
      </View>
      {measure ? (
        <Text variant="footnote" numberOfLines={2}>
          {measure}
        </Text>
      ) : null}
      {rule.min_meters ? (
        <Text variant="footnote" tone={meterLow ? 'danger' : 'muted'} numberOfLines={1}>
          {t('m.reorder_meters', {
            available: formatNumber(rule.balance.measures?.available_meters ?? 0, 2),
            min: formatNumber(rule.min_meters, 2),
          })}
        </Text>
      ) : null}
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {`${rule.warehouse.name} · ${t('m.reorder_thresholds', {
          min: rule.min_quantity,
          point: rule.reorder_point ?? '—',
          max: rule.max_quantity ?? '—',
        })}`}
      </Text>
      <View className="flex-row flex-wrap items-center gap-2">
        <StatusPill tone={levelTone(rule.level)} label={t(`m.level_${rule.level}`)} />
        {meterLow ? <StatusPill tone="problem" label={t('m.level_meters_below_min')} /> : null}
        {rule.balance.reserved_quantity > 0 ? (
          <Text variant="footnote" tone="muted">
            {t('m.balance_reserved', { count: rule.balance.reserved_quantity })}
          </Text>
        ) : null}
        {rule.balance.incoming_quantity > 0 ? (
          <Text variant="footnote" tone="muted">
            {t('m.balance_incoming', { count: rule.balance.incoming_quantity })}
          </Text>
        ) : null}
        {rule.suggested_quantity > 0 ? (
          <Text variant="footnote" tone="muted">
            {t('m.suggested', { count: rule.suggested_quantity })}
          </Text>
        ) : null}
      </View>
    </View>
  );
}

function ForecastRow({ forecast }: { forecast: Forecast }) {
  const { t } = useTranslation('warehouse');
  return (
    <View className="gap-1 rounded-lg border border-border bg-card px-4 py-3">
      <View className="flex-row items-center gap-2">
        <Text variant="headline" className="flex-1" numberOfLines={2}>
          {forecast.product.name}
        </Text>
        <Text variant="title">{forecast.days_left ? t('m.days_left', { days: forecast.days_left }) : '—'}</Text>
      </View>
      <Text variant="footnote" tone="muted">
        {`${t('m.on_hand', { count: forecast.on_hand_qty })}${
          forecast.suggested_qty ? ` · ${t('m.suggested', { count: forecast.suggested_qty })}` : ''
        }`}
      </Text>
    </View>
  );
}
