import * as Haptics from 'expo-haptics';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, Platform, Pressable, ScrollView, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Segmented } from '@/features/warehouse/parts';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import {
  allowedGranularities,
  type Compare,
  COMPARES,
  effectiveGranularity,
  type Granularity,
  isoDay,
  MAX_RANGE_DAYS,
  monthGrid,
  type PeriodState,
  type PeriodSupport,
  PRESETS,
  rangeError,
  stateDays,
  tapRange,
} from './period';
import { useChartTheme } from './theme';

function Pill({ label, on, onPress, icon }: { label: string; on: boolean; onPress: () => void; icon?: 'calendar' | 'settings' }) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ selected: on }}
      onPress={onPress}
      className={cn('min-h-9 flex-row items-center justify-center gap-1.5 rounded-full border px-3.5', on ? 'border-primary bg-primary' : 'border-border bg-card active:bg-muted')}>
      {icon ? <Icon name={icon} tone={on ? 'primary-foreground' : 'muted-foreground'} size={14} /> : null}
      <Text variant="footnote" tone={on ? 'onPrimary' : 'default'} className="font-semibold">
        {label}
      </Text>
    </Pressable>
  );
}

function shortRange(from: string, to: string, locale: string) {
  const f = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short', timeZone: 'UTC' });
  const d = (s: string) => new Date(`${s}T00:00:00Z`);
  return `${f.format(d(from))} – ${f.format(d(to))}`;
}

/**
 * Period bar of the analytics screens: preset chips (today, 7, 30, 90 days,
 * 12 months), a custom range, and an options chip for the bucket size and the
 * comparison. Only what the report takes is offered (`support`).
 */
export function PeriodBar({
  value,
  onChange,
  support = { periodic: true, granularity: true, compare: true },
  className,
}: {
  value: PeriodState;
  onChange: (next: PeriodState) => void;
  support?: PeriodSupport;
  className?: string;
}) {
  const { t } = useTranslation('analytics');
  const th = useChartTheme();
  const [sheet, setSheet] = useState(false);
  if (!support.periodic) return null;
  const custom = value.preset === 'custom' && value.from && value.to;
  const g = support.granularity ? effectiveGranularity(value) : null;
  const options = [g ? t(`granularity.${g}`) : null, support.compare && value.compare !== 'none' ? t(`compare.${value.compare}`) : null].filter(Boolean);

  return (
    <>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} className={cn('grow-0', className)} contentContainerClassName="gap-2 px-4 py-2">
        {PRESETS.map((p) => (
          <Pill key={p} label={t(`preset.${p}`)} on={value.preset === p} onPress={() => onChange({ ...value, preset: p, from: undefined, to: undefined })} />
        ))}
        <Pill
          icon="calendar"
          label={custom ? shortRange(value.from!, value.to!, th.locale) : t('preset.custom')}
          on={value.preset === 'custom'}
          onPress={() => setSheet(true)}
        />
        {support.granularity || support.compare ? (
          <Pill icon="settings" label={options.length ? options.join(' · ') : t('options')} on={false} onPress={() => setSheet(true)} />
        ) : null}
      </ScrollView>
      <Modal visible={sheet} animationType="slide" presentationStyle="pageSheet" onRequestClose={() => setSheet(false)}>
        {sheet ? (
          <PeriodSheet
            value={value}
            support={support}
            onClose={() => setSheet(false)}
            onApply={(next) => {
              onChange(next);
              setSheet(false);
            }}
          />
        ) : null}
      </Modal>
    </>
  );
}

function PeriodSheet({
  value,
  support,
  onClose,
  onApply,
}: {
  value: PeriodState;
  support: PeriodSupport;
  onClose: () => void;
  onApply: (next: PeriodState) => void;
}) {
  const { t } = useTranslation('analytics');
  const insets = useSafeAreaInsets();
  const [draft, setDraft] = useState<PeriodState>(value);
  const [range, setRange] = useState<{ from?: string; to?: string }>(value.preset === 'custom' ? { from: value.from, to: value.to } : {});
  const rangeComplete = !!range.from && !!range.to;
  const err = rangeComplete ? rangeError(range.from, range.to) : null;
  const next: PeriodState = rangeComplete && !err ? { ...draft, preset: 'custom', from: range.from, to: range.to } : draft;
  const grans = allowedGranularities(stateDays(next));

  return (
    <View className="flex-1 bg-background" style={{ paddingTop: Platform.OS === 'ios' ? 0 : insets.top }}>
      <View className="flex-row items-center justify-between gap-2 pb-2 pe-2 ps-5 pt-4">
        <Text variant="title" numberOfLines={1} className="flex-1">
          {t('period_title')}
        </Text>
        <Button title={t('common:close')} variant="ghost" onPress={onClose} />
      </View>
      <ScrollView contentContainerClassName="gap-5 px-4 pb-10" contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}>
        <View className="gap-2">
          <Text variant="headline">{t('custom_range')}</Text>
          <Text variant="footnote" tone="muted">
            {t('custom_hint', { days: MAX_RANGE_DAYS })}
          </Text>
          <RangeCalendar range={range} onTap={(iso) => setRange((r) => tapRange(r, iso))} />
          {err ? (
            <Text variant="footnote" tone="danger">
              {t(`range_error.${err}`, { days: MAX_RANGE_DAYS })}
            </Text>
          ) : null}
        </View>

        {support.granularity ? (
          <View className="gap-2">
            <Text variant="headline">{t('granularity_title')}</Text>
            <Segmented<'auto' | Granularity>
              value={draft.granularity && grans.includes(draft.granularity) ? draft.granularity : 'auto'}
              onChange={(v) => setDraft((d) => ({ ...d, granularity: v === 'auto' ? null : v }))}
              options={[{ value: 'auto', label: t('granularity.auto') }, ...grans.map((g) => ({ value: g, label: t(`granularity.${g}`) }))]}
            />
          </View>
        ) : null}

        {support.compare ? (
          <View className="gap-2">
            <Text variant="headline">{t('compare_title')}</Text>
            <Segmented<Compare>
              value={draft.compare}
              onChange={(v) => setDraft((d) => ({ ...d, compare: v }))}
              options={COMPARES.map((c) => ({ value: c, label: t(`compare.${c}`) }))}
            />
          </View>
        ) : null}

        <Button title={t('apply')} size="lg" disabled={(!!range.from && !range.to) || !!err} onPress={() => onApply(next)} />
      </ScrollView>
    </View>
  );
}

/** First day of the week: Saturday in Arabic and Persian, Monday elsewhere. */
function weekStartOf(locale: string) {
  return locale.startsWith('ar') || locale.startsWith('fa') ? 6 : 1;
}

/** Month calendar for a range: first tap starts it, second closes it; months step back and forth. */
function RangeCalendar({ range, onTap }: { range: { from?: string; to?: string }; onTap: (iso: string) => void }) {
  const th = useChartTheme();
  const { t } = useTranslation('analytics');
  const today = isoDay(new Date());
  const anchor = range.to ?? range.from ?? today;
  const [ym, setYm] = useState({ y: Number(anchor.slice(0, 4)), m: Number(anchor.slice(5, 7)) - 1 });
  const ws = weekStartOf(th.locale);
  const weeks = monthGrid(ym.y, ym.m, ws);
  const title = new Intl.DateTimeFormat(th.locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(ym.y, ym.m, 1)));
  const weekday = new Intl.DateTimeFormat(th.locale, { weekday: 'narrow', timeZone: 'UTC' });
  const step = (d: number) => setYm(({ y, m }) => ({ y: m + d < 0 ? y - 1 : m + d > 11 ? y + 1 : y, m: (m + d + 12) % 12 }));
  const inRange = (iso: string) => !!range.from && !!range.to && iso >= range.from && iso <= range.to;

  return (
    <View className="gap-2 rounded-2xl border border-border bg-card p-3" style={{ borderCurve: 'continuous' }}>
      <View className="flex-row items-center justify-between">
        <Pressable accessibilityRole="button" accessibilityLabel={t('prev_month')} onPress={() => step(-1)} className="size-10 items-center justify-center active:opacity-60">
          <Icon name="back" size={18} />
        </Pressable>
        <Text variant="headline">{title}</Text>
        <Pressable accessibilityRole="button" accessibilityLabel={t('next_month')} onPress={() => step(1)} className="size-10 items-center justify-center active:opacity-60">
          <Icon name="forward" size={18} />
        </Pressable>
      </View>
      <View className="flex-row">
        {weeks[0].map((c) => (
          <Text key={c.iso} variant="caption" tone="muted" className="flex-1 text-center">
            {weekday.format(new Date(`${c.iso}T00:00:00Z`))}
          </Text>
        ))}
      </View>
      {weeks.map((w) => (
        <View key={w[0].iso} className="flex-row">
          {w.map((c) => {
            const edge = c.iso === range.from || c.iso === range.to;
            const mid = inRange(c.iso) && !edge;
            const future = c.iso > today;
            return (
              <Pressable
                key={c.iso}
                accessibilityRole="button"
                accessibilityState={{ selected: edge || mid, disabled: future }}
                disabled={future}
                onPress={() => {
                  void Haptics.selectionAsync().catch(() => undefined);
                  onTap(c.iso);
                }}
                className={cn('h-10 flex-1 items-center justify-center', mid && 'bg-primary/15', edge && 'rounded-full bg-primary')}>
                <Text
                  variant="callout"
                  tone={edge ? 'onPrimary' : c.inMonth ? 'default' : 'muted'}
                  className={cn('tabular-nums', future && 'opacity-30', c.iso === today && !edge && 'font-bold')}>
                  {new Intl.NumberFormat(th.locale).format(c.day)}
                </Text>
              </Pressable>
            );
          })}
        </View>
      ))}
    </View>
  );
}
