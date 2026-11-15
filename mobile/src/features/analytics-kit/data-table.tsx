import { type Href, router, useSegments } from 'expo-router';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, ScrollView, View } from 'react-native';

import { SearchField } from '@/features/services/ui';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { rowLink, rowStatus } from './envelope';
import { formatValue } from './format';
import { linkPath } from './links';
import { alignOf, type Column, currencyOf, nextSort, num, type Row, searchRows, type Sort, sortRows, textOf, totals, type Total } from './table';
import { type ChartTheme, useChartTheme } from './theme';

const FIRST_W = 168;
const COL_W = 112;

/** Report status tone (`_status[key].tone`) as the app's status pill tone. */
function pillTone(tone: string): StatusTone {
  return tone === 'done' || tone === 'problem' || tone === 'progress' ? tone : 'neutral';
}

/** Text of one cell by column type (money in the row's currency, dates in the locale). */
export function cellText(col: Column, row: Row, th: ChartTheme): string {
  const v = row[col.key];
  if (v === null || v === undefined || v === '') return '–';
  switch (col.type) {
    case 'money':
      return formatValue(num(v), 'money', th.locale, currencyOf(col, row));
    case 'percent':
      return formatValue(num(v), 'percent', th.locale);
    case 'duration':
      // Durations come in hours (TEC-1210 fulfillment: approval → shipping).
      return formatValue(num(v), 'hours', th.locale);
    case 'integer':
    case 'number':
      return formatValue(num(v), 'count', th.locale);
    case 'date':
    case 'datetime': {
      const d = typeof v === 'string' ? new Date(v) : null;
      if (!d || Number.isNaN(d.getTime())) return String(v);
      return new Intl.DateTimeFormat(th.locale, col.type === 'date' ? { dateStyle: 'medium' } : { dateStyle: 'medium', timeStyle: 'short' }).format(d);
    }
    default:
      return textOf(v) || '–';
  }
}

function totalText(total: Total | undefined, th: ChartTheme, col: Column): string {
  if (!total) return '';
  if (total.kind === 'number') return formatValue(total.value, col.type === 'percent' ? 'percent' : col.type === 'duration' ? 'hours' : 'count', th.locale);
  return Object.entries(total.byCurrency)
    .map(([cur, v]) => formatValue(v, 'money', th.locale, cur))
    .join(' · ');
}

/**
 * Report table: tap a header to sort (first direction → other → off),
 * search over every text cell, a totals row of the rows shown, and rows that
 * open their record (`_link`). Scrolls sideways when the columns do not fit.
 */
export function DataTable({ columns, rows, searchable = true }: { columns: Column[]; rows: Row[]; searchable?: boolean }) {
  const { t, i18n } = useTranslation('analytics');
  const th = useChartTheme();
  const seg = useSegments() as string[];
  const space = seg[1] ?? 'bayi';
  const [q, setQ] = useState('');
  const [sort, setSort] = useState<Sort>(null);
  const shown = useMemo(
    () => sortRows(searchRows(rows, columns, q, i18n.language), columns, sort, i18n.language),
    [rows, columns, q, sort, i18n.language],
  );
  const sums = useMemo(() => totals(shown, columns), [shown, columns]);
  const hasTotals = Object.keys(sums).length > 0 && shown.length > 1;
  const width = (i: number) => (i === 0 ? FIRST_W : COL_W);

  return (
    <View className="gap-2">
      {searchable && rows.length > 5 ? <SearchField value={q} onChange={setQ} placeholder={t('table_search')} /> : null}
      <Text variant="footnote" tone="muted">
        {q ? t('rows_filtered', { count: shown.length, total: rows.length }) : t('rows', { count: rows.length })}
      </Text>
      <View className="overflow-hidden rounded-2xl border border-border bg-card" style={{ borderCurve: 'continuous' }}>
        <ScrollView horizontal showsHorizontalScrollIndicator bounces={false}>
          <View>
            <View className="flex-row border-b border-border bg-muted">
              {columns.map((c, i) => {
                const on = sort?.key === c.key;
                const right = alignOf(c) === 'right';
                return (
                  <Pressable
                    key={c.key}
                    accessibilityRole="button"
                    accessibilityLabel={on ? t(`sorted_${sort!.dir}`, { column: c.label }) : t('sort_by', { column: c.label })}
                    onPress={() => setSort((s) => nextSort(s, c))}
                    className={cn('min-h-11 flex-row items-center gap-1 px-3 py-2 active:opacity-60', right && 'justify-end')}
                    style={{ width: width(i) }}>
                    <Text variant="footnote" className={cn('shrink font-semibold', !on && 'text-muted-foreground')} numberOfLines={2}>
                      {c.label}
                    </Text>
                    <Icon name={on ? (sort!.dir === 'asc' ? 'chevronUp' : 'chevronDown') : 'sort'} tone={on ? 'foreground' : 'muted-foreground'} size={11} />
                  </Pressable>
                );
              })}
            </View>
            {shown.length === 0 ? (
              <Text variant="footnote" tone="muted" className="px-3 py-6">
                {t('no_match')}
              </Text>
            ) : null}
            {shown.map((row, ri) => {
              const link = rowLink(row);
              const href = link ? linkPath(space, link.kind, link.uuid, link.report) : null;
              const cells = columns.map((c, i) => {
                const right = alignOf(c) === 'right';
                const status = c.type === 'status' ? rowStatus(row, c.key) : null;
                return (
                  <View key={c.key} className={cn('justify-center px-3 py-2.5', right && 'items-end')} style={{ width: width(i) }}>
                    {status ? (
                      <StatusPill label={status.label} tone={pillTone(status.tone)} />
                    ) : (
                      <Text
                        variant="footnote"
                        className={cn(right && 'text-right tabular-nums', i === 0 && 'font-semibold', i === 0 && href && 'text-primary')}
                        numberOfLines={2}>
                        {cellText(c, row, th)}
                      </Text>
                    )}
                  </View>
                );
              });
              const cls = cn('flex-row', ri < shown.length - 1 && 'border-b border-border');
              return href ? (
                <Pressable key={ri} accessibilityRole="link" onPress={() => router.push(href as Href)} className={cn(cls, 'active:bg-muted')}>
                  {cells}
                </Pressable>
              ) : (
                <View key={ri} className={cls}>
                  {cells}
                </View>
              );
            })}
            {hasTotals ? (
              <View className="flex-row border-t-2 border-border bg-muted">
                {columns.map((c, i) => (
                  <View key={c.key} className={cn('justify-center px-3 py-2.5', alignOf(c) === 'right' && 'items-end')} style={{ width: width(i) }}>
                    <Text variant="footnote" className={cn('font-semibold', alignOf(c) === 'right' && 'text-right tabular-nums')} numberOfLines={2}>
                      {i === 0 && !sums[c.key] ? t('total') : totalText(sums[c.key], th, c)}
                    </Text>
                  </View>
                ))}
              </View>
            ) : null}
          </View>
        </ScrollView>
      </View>
    </View>
  );
}
