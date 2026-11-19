/**
 * TEC-1240: the box / meter parts of an order line shared by the new order
 * cart and the order editor (one model, two screens): the measure badge and
 * the [Box | Meter] add buttons, the amount stepper (+/- 1 box or 0.5 m, the
 * number pad for typing), "≈ X boxes", the line credits of a meter line, and
 * the whole box hint that converts the line with one tap. Large touch
 * targets: everything stays reachable with one thumb.
 */
import * as Haptics from 'expo-haptics';
import { useTranslation } from 'react-i18next';
import { Pressable, TextInput, View } from 'react-native';

import { useFormat } from '@/features/admin/kit';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import {
  boxEquivalent,
  byMeter,
  type CartLine,
  isCutLine,
  lineMeasure,
  normalizeAmount,
  type SaleMeasure,
  stepMeters,
  wholeBoxSuggestion,
} from './order-lines';

/** Small measure badge of a line: "Box" or "Meter" (the line's measure never changes). */
export function MeasureBadge({ measure }: { measure: SaleMeasure }) {
  const { t } = useTranslation('orders');
  return (
    <View
      testID={`measure-${measure}`}
      className={cn('flex-row items-center gap-1 self-start rounded-md px-2 py-0.5', measure === 'meter' ? 'bg-primary/10' : 'bg-secondary')}>
      <Icon name={measure === 'meter' ? 'measure' : 'package'} tone="primary" size={12} />
      <Text variant="footnote" className="font-semibold">
        {t(`measure.${measure}`)}
      </Text>
    </View>
  );
}

/**
 * [+ Box | + Meter] of a product sold by the meter: one tap adds that line;
 * a measure already in the cart shows a check and opens nothing.
 */
export function MeasureAddButtons({
  has,
  onAdd,
  compact,
}: {
  has: (m: SaleMeasure) => boolean;
  onAdd: (m: SaleMeasure) => void;
  compact?: boolean;
}) {
  const { t } = useTranslation('orders');
  return (
    <View className="flex-row overflow-hidden rounded-lg border border-border" accessibilityRole="radiogroup">
      {(['box', 'meter'] as const).map((m, i) => {
        const on = has(m);
        return (
          <Pressable
            key={m}
            testID={`add-${m}`}
            accessibilityRole="button"
            accessibilityState={{ selected: on }}
            accessibilityLabel={on ? t('measure.in_cart', { measure: t(`measure.${m}`) }) : t(`measure.add_${m}`)}
            onPress={() => {
              void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
              onAdd(m);
            }}
            className={cn(
              'flex-row items-center justify-center gap-1.5 active:opacity-70',
              compact ? 'min-h-11 px-3' : 'min-h-[52px] flex-1 px-4',
              i > 0 && 'border-s border-border',
              on ? 'bg-primary/10' : 'bg-card',
            )}>
            <Icon name={on ? 'check' : 'add'} tone="primary" size={compact ? 15 : 17} weight="semibold" />
            <Text variant={compact ? 'footnote' : 'body'} tone="primary" className="font-semibold">
              {t(`measure.${m}`)}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const STEP_BTN = 'size-12 items-center justify-center rounded-lg bg-secondary active:opacity-70';

/**
 * Amount of a line: [-] [field] [+]. A box line steps by 1 piece, a meter line
 * by 0.5 m and never below the minimum cut; the field opens the number pad
 * (decimal on a meter line). Under a meter line of a box: "≈ X boxes", the
 * minimum cut and, when known, the line amount.
 */
export function AmountStepper({
  line,
  onChange,
  invalid,
  total,
}: {
  line: CartLine;
  onChange: (v: string) => void;
  invalid?: boolean;
  /** Formatted line amount ("tutar"), shown under the field; omitted without a price. */
  total?: string | null;
}) {
  const { t } = useTranslation('orders');
  const f = useFormat();
  const meter = byMeter(line);
  const label = meter ? t('amount_meters') : t('amount_pieces');
  const step = (dir: 1 | -1) => {
    void Haptics.selectionAsync();
    if (meter) return onChange(stepMeters(line.amount, dir, line.min_cut));
    const raw = normalizeAmount(line.amount);
    const n = /^[0-9]+$/.test(raw) ? Number(raw) : 0;
    onChange(String(Math.max(1, n + dir)));
  };
  const boxes = boxEquivalent(line);
  const info = [
    boxes !== null ? t('measure.box_equivalent', { value: f.number(boxes, 1) }) : null,
    meter && line.min_cut ? t('measure.min_cut', { value: f.number(line.min_cut, 2) }) : null,
  ].filter(Boolean);
  return (
    <View className="gap-1.5">
      <View className="flex-row items-center gap-2">
        <Text variant="callout" tone="muted" className="flex-1">
          {label}
        </Text>
        <Pressable accessibilityRole="button" accessibilityLabel={t('measure.decrease')} onPress={() => step(-1)} className={STEP_BTN}>
          <Icon name="remove" tone="primary" size={20} weight="semibold" />
        </Pressable>
        <TextInput
          value={line.amount}
          onChangeText={onChange}
          accessibilityLabel={label}
          placeholder={meter ? t('placeholder_meters') : t('placeholder_pieces')}
          placeholderTextColorClassName="accent-muted-foreground"
          keyboardType={meter ? 'decimal-pad' : 'number-pad'}
          selectTextOnFocus
          className={cn(
            'min-h-12 w-24 rounded-md border bg-background px-2 text-center text-[19px] font-semibold text-foreground tabular-nums',
            invalid ? 'border-destructive' : 'border-border',
          )}
        />
        <Pressable accessibilityRole="button" accessibilityLabel={t('measure.increase')} onPress={() => step(1)} className={STEP_BTN}>
          <Icon name="add" tone="primary" size={20} weight="semibold" />
        </Pressable>
      </View>
      {info.length || total ? (
        <View className="flex-row items-center justify-between gap-3">
          <Text variant="footnote" tone="muted" className="flex-1">
            {info.join(' · ')}
          </Text>
          {total ? (
            <Text variant="footnote" className="font-semibold tabular-nums">
              {total}
            </Text>
          ) : null}
        </View>
      ) : null}
    </View>
  );
}

/**
 * K3/K12: usage credits of each cut piece of a meter line (default: the
 * product's, 5 unless set). Shared with the plotter mark of the line.
 */
export function LineCreditsField({
  line,
  onChange,
  invalid,
}: {
  line: CartLine;
  onChange: (v: string) => void;
  invalid?: boolean;
}) {
  const { t } = useTranslation('orders');
  if (!isCutLine(line)) return null;
  const fallback = line.default_credits ?? 5;
  return (
    <View className="gap-1">
      <View className="flex-row items-center gap-3">
        <Text variant="callout" tone="muted" className="flex-1">
          {t('measure.credits')}
        </Text>
        <TextInput
          value={line.line_credits ?? ''}
          onChangeText={onChange}
          accessibilityLabel={t('measure.credits')}
          placeholder={String(fallback)}
          placeholderTextColorClassName="accent-muted-foreground"
          keyboardType="number-pad"
          selectTextOnFocus
          className={cn(
            'min-h-12 w-24 rounded-md border bg-background px-2 text-center text-[17px] text-foreground tabular-nums',
            invalid ? 'border-destructive' : 'border-border',
          )}
        />
      </View>
      <Text variant="footnote" tone={invalid ? 'danger' : 'muted'}>
        {invalid ? t('errors.line_credits_invalid') : t('measure.credits_hint', { value: fallback })}
      </Text>
    </View>
  );
}

/**
 * Whole box hint of a meter line: a box (or boxes plus a valid rest) fits
 * better than cutting; one tap moves the boxes to the box line.
 */
export function WholeBoxHint({ line, onConvert, disabled }: { line: CartLine; onConvert: () => void; disabled?: boolean }) {
  const { t } = useTranslation('orders');
  const f = useFormat();
  const s = wholeBoxSuggestion(line);
  if (!s || lineMeasure(line) !== 'meter') return null;
  const what = s.meters > 0 ? t('measure.boxes_and_meters', { n: s.boxes, meters: f.number(s.meters, 2) }) : t('measure.boxes', { n: s.boxes });
  return (
    <View testID="whole-box-hint" className="gap-2 rounded-lg border border-warning/40 bg-warning/10 p-3">
      <View className="flex-row items-start gap-2">
        <Icon name="info" tone="primary" size={16} />
        <Text variant="callout" className="flex-1">
          {t('measure.whole_box_hint', { what })}
        </Text>
      </View>
      {disabled ? null : (
        <Pressable
          accessibilityRole="button"
          onPress={() => {
            void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
            onConvert();
          }}
          className="min-h-11 flex-row items-center justify-center gap-2 rounded-md bg-primary px-4 active:opacity-80">
          <Icon name="package" tone="primary-foreground" size={16} />
          <Text tone="onPrimary" className="font-semibold">
            {t('measure.convert', { what })}
          </Text>
        </Pressable>
      )}
    </View>
  );
}

/** "+ Meter" / "+ Box" under a line of a product sold by the meter, when the other measure is not in the cart yet. */
export function OtherMeasureButton({ measure, onPress }: { measure: SaleMeasure; onPress: () => void }) {
  const { t } = useTranslation('orders');
  return (
    <Pressable
      accessibilityRole="button"
      testID={`other-${measure}`}
      onPress={() => {
        void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
        onPress();
      }}
      className="min-h-11 flex-row items-center gap-1.5 self-start rounded-md px-1 active:opacity-70">
      <Icon name="add" tone="primary" size={15} weight="semibold" />
      <Text variant="callout" tone="primary">
        {t(`measure.add_${measure}`)}
      </Text>
    </Pressable>
  );
}
