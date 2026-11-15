import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { KpiGrid } from '@/ui/kpi';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import { Sparkline } from './charts';
import { deltaPct, deltaTone, direction, formatDelta, type Good } from './delta';
import type { Kpi } from './envelope';
import { formatValue } from './format';
import { useChartTheme } from './theme';

export { KpiGrid };

/**
 * Change against the previous period: an arrow icon and the signed percent
 * (never color alone). Green when the metric moved the good way, red the
 * other way, muted when flat.
 */
export function DeltaBadge({ pct, good = 'up' }: { pct: number | null | undefined; good?: Good | null }) {
  const th = useChartTheme();
  const { t } = useTranslation('analytics');
  const dir = direction(pct);
  if (dir === null || pct === null || pct === undefined) return null;
  const tone = deltaTone(pct, good);
  const color = tone === 'good' ? th.good : tone === 'bad' ? th.bad : th.muted;
  const icon: IconName = dir === 'up' ? 'trendUp' : dir === 'down' ? 'trendDown' : 'remove';
  const text = formatDelta(pct, th.locale);
  return (
    <View
      accessible
      accessibilityLabel={t(`delta_a11y.${tone}`, { value: text })}
      className="flex-row items-center gap-1 self-start rounded-full px-1.5 py-0.5"
      style={{ backgroundColor: `${color}1f` }}>
      <Icon name={icon} color={color} size={12} weight="semibold" />
      <Text variant="footnote" className="font-semibold tabular-nums" style={{ color }}>
        {text}
      </Text>
    </View>
  );
}

const TILE = 'min-h-[112px] grow basis-[46%] justify-between gap-2 rounded-2xl border border-border bg-card p-3.5';

/**
 * One KPI: label, the value as the hero, the delta badge with the previous
 * value, and a sparkline of the period. With `onPress` (the KPI's link) the
 * tile opens the related list or record.
 */
export function KpiTile({ kpi, icon, currency, onPress }: { kpi: Kpi; icon?: IconName; currency?: string | null; onPress?: () => void }) {
  const th = useChartTheme();
  const { t } = useTranslation('analytics');
  const value = formatValue(kpi.value, kpi.unit, th.locale, currency);
  const pct = kpi.delta_pct ?? deltaPct(kpi.value, kpi.previous);
  const hasPrev = kpi.previous !== null && kpi.previous !== undefined;
  const spark = (kpi.spark ?? []).filter((v) => Number.isFinite(v));
  const body = (
    <>
      <View className="flex-row items-center gap-2">
        {icon ? (
          <View className="size-7 items-center justify-center rounded-full bg-primary/10">
            <Icon name={icon} tone="primary" size={14} weight="medium" />
          </View>
        ) : null}
        <Text variant="footnote" tone="muted" numberOfLines={2} className="flex-1">
          {kpi.label}
        </Text>
      </View>
      <View className="flex-row items-end justify-between gap-2">
        <Text variant="title" className="shrink text-[28px] font-bold leading-[34px] tracking-tight tabular-nums" numberOfLines={1} adjustsFontSizeToFit>
          {value}
        </Text>
        {spark.length > 1 ? <Sparkline values={spark} width={60} height={22} /> : null}
      </View>
      {pct !== null || hasPrev ? (
        <View className="flex-row flex-wrap items-center gap-x-2 gap-y-1">
          <DeltaBadge pct={pct} good={kpi.good} />
          {hasPrev ? (
            <Text variant="footnote" tone="muted" className="tabular-nums" numberOfLines={1}>
              {t('previous_value', { value: formatValue(kpi.previous, kpi.unit, th.locale, currency) })}
            </Text>
          ) : null}
        </View>
      ) : null}
    </>
  );
  const a11y = `${kpi.label}: ${value}`;
  if (!onPress) {
    return (
      <View accessible accessibilityLabel={a11y} className={TILE} style={{ borderCurve: 'continuous' }}>
        {body}
      </View>
    );
  }
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={a11y} onPress={onPress} className={`${TILE} active:opacity-80`} style={{ borderCurve: 'continuous' }}>
      {body}
    </Pressable>
  );
}

export function KpiTileSkeleton({ count = 4 }: { count?: number }) {
  return (
    <KpiGrid>
      {Array.from({ length: count }, (_, i) => (
        <Skeleton key={i} className="h-[112px] grow basis-[46%] rounded-2xl" />
      ))}
    </KpiGrid>
  );
}
