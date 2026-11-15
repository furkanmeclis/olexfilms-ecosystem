import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type GestureResponderEvent, Pressable, View } from 'react-native';
import Svg, { Circle, G, Line, Path, Text as SvgText } from 'react-native-svg';

import { cn } from '@/ui/cn';
import { Text } from '@/ui/text';

import type { Point, Series } from './envelope';
import { bucketLabel, compact, formatValue, guessGranularity } from './format';
import { MAX_CATEGORIES, withAlpha } from './palette';
import {
  arcPath,
  bandLayout,
  donutAngles,
  extent,
  labelIndices,
  linear,
  linePath,
  nearestIndex,
  niceTicks,
  pointX,
} from './scale';
import { type ChartTheme, useChartTheme } from './theme';

const AXIS_FONT = 11;
const PAD_TOP = 8;
const PAD_BOTTOM = 20;
const MARK = 2; // line width
const DOT = 4; // radius: 8 pt markers
const BAR_RADIUS = 4;

/** Bar with rounded top corners only (the baseline stays square). */
function barPath(x: number, y: number, w: number, h: number, r: number) {
  if (h <= 0 || w <= 0) return '';
  const rr = Math.min(r, w / 2, h);
  return `M${x},${y + h}V${y + rr}Q${x},${y} ${x + rr},${y}H${x + w - rr}Q${x + w},${y} ${x + w},${y + rr}V${y + h}Z`;
}

export type LegendItem = { key: string; label: string; color: string; dashed?: boolean };

/** Legend row: swatch and label per series; the previous period as a dashed swatch. */
export function Legend({ items }: { items: LegendItem[] }) {
  return (
    <View className="flex-row flex-wrap gap-x-4 gap-y-1.5">
      {items.map((it) => (
        <View key={it.key} className="flex-row items-center gap-1.5">
          <Svg width={16} height={8}>
            <Line
              x1={1}
              y1={4}
              x2={15}
              y2={4}
              stroke={it.color}
              strokeWidth={it.dashed ? MARK : 4}
              strokeLinecap="round"
              strokeDasharray={it.dashed ? '3 3' : undefined}
            />
          </Svg>
          <Text variant="footnote" numberOfLines={1}>
            {it.label}
          </Text>
        </View>
      ))}
    </View>
  );
}

type TrendProps = {
  /** Series sharing the same buckets and unit (one y axis). At most 8 are drawn. */
  series: Series[];
  kind?: 'line' | 'bar';
  granularity?: string | null;
  height?: number;
  /** Draws the previous period (`points[].previous`) as dashed lines when present. */
  showPrevious?: boolean;
  currency?: string | null;
  /** Hides the legend (e.g. a single series in a titled card). */
  legend?: boolean;
};

/**
 * Line or grouped bar chart over time with one y axis, localized axis labels,
 * the previous period as a dashed line in the series' color, and a touch
 * crosshair with a tooltip (tap or drag; tap the same bucket again to hide).
 */
export function TrendChart({ series: all, kind = 'line', granularity, height = 180, showPrevious = true, currency, legend = true }: TrendProps) {
  const th = useChartTheme();
  const { t } = useTranslation('analytics');
  const [width, setWidth] = useState(0);
  const [active, setActive] = useState<number | null>(null);
  const series = all.slice(0, MAX_CATEGORIES);
  const base = series[0];
  if (!base) return null;
  const n = base.points.length;
  const gran = granularity ?? guessGranularity(base.points.map((p) => p.key));
  const unit = base.unit;
  const prevShown = showPrevious && series.some((s) => s.points.some((p) => p.previous !== null && p.previous !== undefined));

  const values = series.flatMap((s) => s.points.flatMap((p) => (prevShown ? [p.value, p.previous] : [p.value])));
  const { min, max } = extent(values);
  const ticks = niceTicks(min, max, 4);
  const tickText = ticks.map((v) => compact(v, th.locale));
  const padStart = Math.max(...tickText.map((s) => s.length)) * 6.5 + 8;
  const plotW = Math.max(0, width - padStart - 4);
  const plotH = height - PAD_TOP - PAD_BOTTOM;
  const y = linear(ticks[0], ticks[ticks.length - 1], PAD_TOP + plotH, PAD_TOP);
  const band = bandLayout(n, series.length, plotW, n > 20 ? 0.15 : 0.3);
  const xAt = (i: number) => padStart + (kind === 'bar' ? band.center(i) : pointX(i, n, plotW));
  const xLabels = labelIndices(n, width > 0 ? Math.max(2, Math.floor(plotW / 64)) : 2);

  const onTouch = (e: GestureResponderEvent, toggle: boolean) => {
    const idx = nearestIndex(e.nativeEvent.locationX - padStart, n, plotW, kind === 'bar' ? 'band' : 'point');
    if (idx < 0) return;
    setActive((cur) => (toggle && cur === idx ? null : idx));
  };

  const legendItems: LegendItem[] = [
    ...series.map((s, i) => ({ key: s.key, label: s.label, color: th.palette[i] })),
    ...(prevShown ? [{ key: '__prev', label: t('previous_period'), color: th.muted, dashed: true }] : []),
  ];
  const showLegend = legend && (series.length > 1 || prevShown);
  const summary = series.map((s) => `${s.label}: ${formatValue(s.points.reduce((a, p) => a + p.value, 0), unit, th.locale, currency)}`).join(', ');

  return (
    <View className="gap-2">
      <View
        accessible
        accessibilityRole="image"
        accessibilityLabel={summary}
        style={{ height }}
        onLayout={(e) => setWidth(e.nativeEvent.layout.width)}
        onStartShouldSetResponder={() => true}
        onMoveShouldSetResponder={() => true}
        onResponderGrant={(e) => onTouch(e, true)}
        onResponderMove={(e) => onTouch(e, false)}
        onResponderTerminationRequest={() => true}>
        {width > 0 ? (
          <Svg width={width} height={height}>
            {/* Grid and axis in the background. */}
            {ticks.map((v, i) => (
              <G key={`t${i}`}>
                <Line x1={padStart} x2={width - 4} y1={y(v)} y2={y(v)} stroke={th.grid} strokeWidth={v === 0 ? 1 : 0.5} />
                <SvgText x={padStart - 6} y={y(v) + 4} fontSize={AXIS_FONT} fill={th.muted} textAnchor="end">
                  {tickText[i]}
                </SvgText>
              </G>
            ))}
            {xLabels.map((i) => (
              <SvgText
                key={`x${i}`}
                x={n > 1 && i === 0 ? padStart : n > 1 && i === n - 1 ? width - 4 : xAt(i)}
                y={height - 5}
                fontSize={AXIS_FONT}
                fill={th.muted}
                textAnchor={n > 1 && i === 0 ? 'start' : n > 1 && i === n - 1 ? 'end' : 'middle'}>
                {bucketLabel(base.points[i].key, gran, th.locale)}
              </SvgText>
            ))}
            {active !== null ? (
              <Line x1={xAt(active)} x2={xAt(active)} y1={PAD_TOP} y2={PAD_TOP + plotH} stroke={th.muted} strokeWidth={1} />
            ) : null}

            {kind === 'bar'
              ? series.map((s, si) =>
                  s.points.map((p, i) => {
                    const top = y(Math.max(0, p.value));
                    const zero = y(0);
                    return (
                      <Path
                        key={`${s.key}${i}`}
                        d={barPath(padStart + band.x(i, si) + 0.5, top, Math.max(1, band.bar - 1), zero - top, BAR_RADIUS)}
                        fill={active === null || active === i ? th.palette[si] : withAlpha(th.palette[si], 0.45)}
                      />
                    );
                  }),
                )
              : null}

            {prevShown
              ? series.map((s, si) => (
                  <Path
                    key={`p${s.key}`}
                    d={linePath(s.points.map((p, i) => (p.previous === null || p.previous === undefined ? null : { x: xAt(i), y: y(p.previous) })))}
                    stroke={withAlpha(th.palette[si], 0.7)}
                    strokeWidth={MARK}
                    strokeDasharray="4 4"
                    strokeLinecap="round"
                    fill="none"
                  />
                ))
              : null}

            {kind === 'line'
              ? series.map((s, si) => (
                  <G key={`l${s.key}`}>
                    <Path
                      d={linePath(s.points.map((p, i) => ({ x: xAt(i), y: y(p.value) })))}
                      stroke={th.palette[si]}
                      strokeWidth={MARK}
                      strokeLinejoin="round"
                      strokeLinecap="round"
                      fill="none"
                    />
                    {s.points.map((p, i) =>
                      n <= 12 || active === i ? (
                        <Circle key={i} cx={xAt(i)} cy={y(p.value)} r={DOT} fill={th.palette[si]} stroke={th.card} strokeWidth={1.5} />
                      ) : null,
                    )}
                  </G>
                ))
              : null}
          </Svg>
        ) : null}
        {active !== null && width > 0 ? (
          <Tooltip th={th} series={series} index={active} x={xAt(active)} width={width} gran={gran} unit={unit} currency={currency} prev={prevShown} />
        ) : null}
      </View>
      {showLegend ? <Legend items={legendItems} /> : null}
    </View>
  );
}

const TIP_W = 176;

function Tooltip({
  th,
  series,
  index,
  x,
  width,
  gran,
  unit,
  currency,
  prev,
}: {
  th: ChartTheme;
  series: Series[];
  index: number;
  x: number;
  width: number;
  gran: string;
  unit: string;
  currency?: string | null;
  prev: boolean;
}) {
  const { t } = useTranslation('analytics');
  const key = series[0].points[index]?.key ?? '';
  const left = Math.min(Math.max(0, x - TIP_W / 2), Math.max(0, width - TIP_W));
  return (
    <View
      pointerEvents="none"
      className="absolute top-0 gap-1 rounded-md border border-border bg-card px-2.5 py-2"
      style={{ left, width: TIP_W, boxShadow: '0 2px 8px rgba(0,0,0,0.12)' }}>
      <Text variant="footnote" className="font-semibold">
        {bucketLabel(key, gran, th.locale, true)}
      </Text>
      {series.map((s, si) => {
        const p: Point | undefined = s.points[index];
        if (!p) return null;
        return (
          <View key={s.key} className="gap-0.5">
            <View className="flex-row items-center gap-1.5">
              <View className="size-2 rounded-full" style={{ backgroundColor: th.palette[si] }} />
              <Text variant="footnote" className="flex-1" numberOfLines={1}>
                {s.label}
              </Text>
              <Text variant="footnote" className="font-semibold tabular-nums">
                {formatValue(p.value, unit, th.locale, currency)}
              </Text>
            </View>
            {prev && p.previous !== null && p.previous !== undefined ? (
              <Text variant="footnote" tone="muted" className="ps-3.5 tabular-nums" numberOfLines={1}>
                {t('previous_value', { value: formatValue(p.previous, unit, th.locale, currency) })}
              </Text>
            ) : null}
          </View>
        );
      })}
    </View>
  );
}

/** Tiny trend line of a KPI (no axis); the last value gets a dot. */
export function Sparkline({ values, width = 72, height = 24, color }: { values: readonly number[]; width?: number; height?: number; color?: string }) {
  const th = useChartTheme();
  if (values.length < 2) return null;
  const { min, max } = extent(values);
  const y = linear(min, max === min ? min + 1 : max, height - 3, 3);
  const pts = values.map((v, i) => ({ x: 3 + pointX(i, values.length, width - 6), y: y(v) }));
  const c = color ?? th.palette[0];
  const last = pts[pts.length - 1];
  return (
    <Svg width={width} height={height} accessibilityElementsHidden importantForAccessibility="no-hide-descendants">
      <Path d={linePath(pts)} stroke={c} strokeWidth={MARK} strokeLinejoin="round" strokeLinecap="round" fill="none" />
      <Circle cx={last.x} cy={last.y} r={2.5} fill={c} />
    </Svg>
  );
}

export type RankItem = {
  key: string;
  label: string;
  value: number;
  /** Shown instead of the formatted value (e.g. "12 orders"). */
  valueText?: string;
  /** Second line (amounts, city). */
  sub?: string | null;
  onPress?: () => void;
};

/** Horizontal ranking bars, largest first as given; rows with `onPress` open their record. */
export function RankBars({ items, unit, numbered = true }: { items: RankItem[]; unit?: string; numbered?: boolean }) {
  const th = useChartTheme();
  const max = Math.max(1, ...items.map((i) => i.value));
  return (
    <View className="gap-3">
      {items.map((it, i) => {
        const body = (
          <View className="gap-1">
            <View className="flex-row items-baseline justify-between gap-2">
              <Text variant="footnote" className="flex-1" numberOfLines={1}>
                {numbered ? `${new Intl.NumberFormat(th.locale).format(i + 1)}. ${it.label}` : it.label}
              </Text>
              <Text variant="footnote" className="font-semibold tabular-nums">
                {it.valueText ?? formatValue(it.value, unit, th.locale)}
              </Text>
            </View>
            {it.sub ? (
              <Text variant="footnote" tone="muted" className="tabular-nums" numberOfLines={1}>
                {it.sub}
              </Text>
            ) : null}
            <View className="h-2 overflow-hidden rounded-full" style={{ backgroundColor: th.grid }}>
              <View className="h-2 rounded-full" style={{ width: `${Math.max(0, (it.value / max) * 100)}%`, backgroundColor: th.palette[0] }} />
            </View>
          </View>
        );
        return it.onPress ? (
          <Pressable key={it.key} accessibilityRole="button" onPress={it.onPress} className="active:opacity-70">
            {body}
          </Pressable>
        ) : (
          <View key={it.key}>{body}</View>
        );
      })}
    </View>
  );
}

export type Slice = { key: string; label: string; value: number };

/**
 * Donut of a distribution with the total in the middle and a legend with
 * values and shares. Past 7 categories the rest fold into "other" (grey),
 * so palette colors never repeat.
 */
export function Donut({ items, unit, size = 132 }: { items: Slice[]; unit?: string; size?: number }) {
  const th = useChartTheme();
  const { t } = useTranslation('analytics');
  const sorted = [...items].filter((s) => s.value > 0).sort((a, b) => b.value - a.value);
  const head = sorted.length > MAX_CATEGORIES ? sorted.slice(0, MAX_CATEGORIES - 1) : sorted;
  const rest = sorted.slice(head.length);
  const slices = rest.length ? [...head, { key: '__other', label: t('other'), value: rest.reduce((a, s) => a + s.value, 0) }] : head;
  const colors = slices.map((s, i) => (s.key === '__other' ? th.muted : th.palette[i]));
  const total = slices.reduce((a, s) => a + s.value, 0);
  const angles = donutAngles(slices.map((s) => s.value));
  const c = size / 2;
  const pct = (v: number) => new Intl.NumberFormat(th.locale, { style: 'percent', maximumFractionDigits: 0 }).format(total ? v / total : 0);

  return (
    <View className="flex-row items-center gap-4">
      <View style={{ width: size, height: size }} accessible accessibilityRole="image" accessibilityLabel={slices.map((s) => `${s.label} ${pct(s.value)}`).join(', ')}>
        <Svg width={size} height={size}>
          {total === 0 ? (
            <Circle cx={c} cy={c} r={c - 10} stroke={th.grid} strokeWidth={16} fill="none" />
          ) : (
            slices.map((s, i) => (
              // A 1 pt gap in the card color between slices.
              <Path key={s.key} d={arcPath(c, c - 18, c - 2, angles[i].start, angles[i].end)} fill={colors[i]} stroke={th.card} strokeWidth={1} />
            ))
          )}
        </Svg>
        <View className="absolute inset-0 items-center justify-center">
          <Text variant="headline" className="tabular-nums" numberOfLines={1} adjustsFontSizeToFit>
            {formatValue(total, unit, th.locale)}
          </Text>
        </View>
      </View>
      <View className="flex-1 gap-1.5">
        {slices.map((s, i) => (
          <View key={s.key} className="flex-row items-center gap-2">
            <View className="size-2.5 rounded-full" style={{ backgroundColor: colors[i] }} />
            <Text variant="footnote" className="flex-1" numberOfLines={1}>
              {s.label}
            </Text>
            <Text variant="footnote" className="font-semibold tabular-nums">
              {formatValue(s.value, unit, th.locale)}
            </Text>
            <Text variant="footnote" tone="muted" className="w-10 text-right tabular-nums">
              {pct(s.value)}
            </Text>
          </View>
        ))}
      </View>
    </View>
  );
}

/** Card around a chart: title, optional trailing text (total, delta) and the body. */
export function ChartCard({
  title,
  trailing,
  children,
  className,
}: {
  title?: string;
  trailing?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <View className={cn('gap-3 rounded-2xl border border-border bg-card p-3.5', className)} style={{ borderCurve: 'continuous' }}>
      {title || trailing ? (
        <View className="flex-row items-center justify-between gap-2">
          {title ? (
            <Text variant="callout" className="flex-1 font-semibold" numberOfLines={1}>
              {title}
            </Text>
          ) : (
            <View className="flex-1" />
          )}
          {trailing}
        </View>
      ) : null}
      {children}
    </View>
  );
}
