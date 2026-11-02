import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';
import Svg, { Path } from 'react-native-svg';
import { useCSSVariable } from 'uniwind';

import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import {
  applyPreset,
  isPresetActive,
  orderedKeys,
  presetAvailable,
  selectablePartKeys,
  splitPartKeys,
  VEHICLE_CATEGORY_ORDER,
  VEHICLE_OUTLINES,
  VEHICLE_PART_PRESETS,
  VEHICLE_PARTS,
  VEHICLE_RENDER_ORDER,
  VEHICLE_VIEWBOX,
  type PresetGroup,
  schemeColors,
} from './parts';

export type VehiclePartPickerProps = {
  /** Selected part keys (backend `applied_parts` shape). */
  value: readonly string[] | null | undefined;
  /** Without it the picker is read-only (view). */
  onChange?: (keys: string[]) => void;
  /** Forces the view mode even with `onChange` (e.g. a locked service). */
  readOnly?: boolean;
  /** Selectable parts (e.g. the product category's `available_parts`); all when omitted. */
  allowedKeys?: readonly string[] | null;
  /** List part names under the diagram (the selected ones in view). Default: on. */
  showLabels?: boolean;
  /** Draw the diagram in its own card frame; false when already inside a card. Default: true. */
  framed?: boolean;
  className?: string;
};

const STROKE = { strokeWidth: 1.5, strokeMiterlimit: 100 } as const;
const PRESET_GROUPS: readonly PresetGroup[] = ['body', 'windows'];

/** Part name: `dealer:part.<key>`; without a translation, the key made readable. */
export function usePartLabel() {
  const { t } = useTranslation('dealer');
  return useCallback((key: string) => t(`part.${key}`, { defaultValue: key.replace(/_/g, ' ') }), [t]);
}

function useSchemeColors() {
  const v = useCSSVariable([
    '--color-muted',
    '--color-border',
    '--color-muted-foreground',
    '--color-primary',
    '--color-success',
    '--color-card',
  ]);
  const s = (x: unknown, fb: string) => (typeof x === 'string' ? x : fb);
  return schemeColors({
    muted: s(v[0], '#eaf3ed'),
    border: s(v[1], '#d7e1da'),
    mutedForeground: s(v[2], '#55695d'),
    primary: s(v[3], '#003d25'),
    success: s(v[4], '#009966'),
    card: s(v[5], '#fcfefc'),
  });
}

/**
 * Vehicle body part picker. One component, three uses:
 * - view: no `onChange`; selected parts are highlighted, their names listed below.
 * - create / edit: with `onChange`; tap the diagram or a part name to toggle, plus quick presets.
 * Unknown keys are not drawn but are listed by name and kept through `onChange`.
 */
export function VehiclePartPicker({
  value,
  onChange,
  readOnly,
  allowedKeys,
  showLabels = true,
  framed = true,
  className,
}: VehiclePartPickerProps) {
  const { t } = useTranslation('common');
  const label = usePartLabel();
  const colors = useSchemeColors();
  const editable = Boolean(onChange) && !readOnly;
  const [detailed, setDetailed] = useState(false);

  const { known, unknown } = useMemo(() => splitPartKeys(value), [value]);
  const selected = useMemo(() => new Set(known), [known]);
  const selectable = useMemo(() => selectablePartKeys(allowedKeys), [allowedKeys]);
  // Allowed keys the diagram cannot draw (e.g. a category-specific part): offered as chips.
  const extraAllowed = useMemo(() => splitPartKeys(allowedKeys).unknown, [allowedKeys]);

  const emit = useCallback(
    (next: Iterable<string>, extra: readonly string[] = unknown) => onChange?.([...orderedKeys(next), ...extra]),
    [onChange, unknown],
  );
  const toggleExtra = useCallback(
    (key: string) => emit(selected, unknown.includes(key) ? unknown.filter((k) => k !== key) : [...unknown, key]),
    [emit, selected, unknown],
  );
  const toggle = useCallback(
    (key: string) => {
      if (!editable || !selectable.has(key)) return;
      const next = new Set(selected);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      emit(next);
    },
    [editable, emit, selectable, selected],
  );

  const parts = VEHICLE_RENDER_ORDER.map((p) => {
    const on = selected.has(p.key);
    const can = selectable.has(p.key);
    const glass = p.category === 'windows';
    return (
      <Path
        key={p.key}
        d={p.d}
        fillRule={p.evenOdd ? 'evenodd' : undefined}
        fill={on ? (glass ? colors.windowOn : colors.bodyOn) : glass ? colors.window : colors.body}
        opacity={editable && !can && !on ? 0.35 : 1}
        stroke={on ? colors.strokeOn : colors.stroke}
        {...STROKE}
        onPress={editable && can ? () => toggle(p.key) : undefined}
      />
    );
  });

  const listKeys = editable ? [] : known;
  // In edit mode the allowed extras get toggle chips below; only the other unknown keys are listed here.
  const listUnknown = editable ? unknown.filter((k) => !extraAllowed.includes(k)) : unknown;
  const count = known.length + unknown.length;

  return (
    <View className={cn('gap-3', className)}>
      <View
        accessible={!editable}
        accessibilityLabel={
          !editable ? [t('vehicle_parts.scheme'), ...known.map(label)].join(', ') : t('vehicle_parts.scheme')
        }
        className={cn('w-full items-center', framed && 'rounded-lg border border-border bg-card px-2 py-1')}
        style={{ aspectRatio: VEHICLE_VIEWBOX.width / VEHICLE_VIEWBOX.height }}>
        <Svg width="100%" height="100%" viewBox={`0 0 ${VEHICLE_VIEWBOX.width} ${VEHICLE_VIEWBOX.height}`}>
          {VEHICLE_OUTLINES.map((o, i) => (
            <Path key={i} d={o.d} fill="none" fillRule={o.evenOdd ? 'evenodd' : undefined} stroke={colors.stroke} {...STROKE} />
          ))}
          {parts}
        </Svg>
      </View>

      {showLabels && (listKeys.length > 0 || listUnknown.length > 0) ? (
        <View className="flex-row flex-wrap gap-2">
          {listKeys.map((k) => (
            <PartChip key={k} label={label(k)} on glass={k.startsWith('window_')} />
          ))}
          {listUnknown.map((k) => (
            <PartChip key={k} label={label(k)} />
          ))}
        </View>
      ) : null}
      {showLabels && !editable && count === 0 ? (
        <Text variant="footnote" tone="muted">
          {t('vehicle_parts.none')}
        </Text>
      ) : null}

      {editable ? (
        <>
          <View className="flex-row items-center justify-between gap-3">
            <Text variant="footnote" tone="muted">
              {t('vehicle_parts.selected', { count })}
            </Text>
            {count > 0 ? (
              <Pressable accessibilityRole="button" onPress={() => emit([], [])} hitSlop={8}>
                <Text variant="footnote" className="font-semibold text-primary">
                  {t('vehicle_parts.clear')}
                </Text>
              </Pressable>
            ) : null}
          </View>

          {PRESET_GROUPS.map((g) => {
            const presets = VEHICLE_PART_PRESETS.filter((p) => p.group === g && presetAvailable(p, selectable));
            if (!presets.length) return null;
            return (
              <View key={g} className="gap-2">
                <Text variant="caption" tone="muted">
                  {t(`vehicle_parts.cat.${g}`)}
                </Text>
                <View className="flex-row flex-wrap gap-2">
                  {presets.map((p) => {
                    const on = isPresetActive(selected, p, selectable);
                    return (
                      <PartChip
                        key={p.id}
                        label={t(`vehicle_parts.preset.${p.id}`)}
                        on={on}
                        onPress={() => emit(applyPreset(selected, p, selectable))}
                      />
                    );
                  })}
                </View>
              </View>
            );
          })}

          {extraAllowed.length > 0 ? (
            <View className="gap-2">
              <Text variant="caption" tone="muted">
                {t('vehicle_parts.cat.other')}
              </Text>
              <View className="flex-row flex-wrap gap-2">
                {extraAllowed.map((k) => (
                  <PartChip key={k} label={label(k)} on={unknown.includes(k)} onPress={() => toggleExtra(k)} />
                ))}
              </View>
            </View>
          ) : null}

          <Pressable
            accessibilityRole="button"
            accessibilityState={{ expanded: detailed }}
            onPress={() => setDetailed((o) => !o)}
            className="min-h-11 flex-row items-center justify-between gap-2 rounded-lg border border-border bg-card px-3.5 active:bg-muted">
            <Text variant="callout" className="flex-1 font-semibold">
              {t('vehicle_parts.detailed')}
            </Text>
            <Icon name={detailed ? 'chevronUp' : 'chevronDown'} tone="primary" size={18} weight="semibold" />
          </Pressable>

          {detailed
            ? VEHICLE_CATEGORY_ORDER.map((c) => {
                const items = VEHICLE_PARTS.filter((p) => p.category === c && selectable.has(p.key));
                if (!items.length) return null;
                return (
                  <View key={c} className="gap-2">
                    <Text variant="caption" tone="muted">
                      {t(`vehicle_parts.cat.${c}`)}
                    </Text>
                    <View className="flex-row flex-wrap gap-2">
                      {items.map((p) => (
                        <PartChip
                          key={p.key}
                          label={label(p.key)}
                          on={selected.has(p.key)}
                          glass={c === 'windows'}
                          onPress={() => toggle(p.key)}
                        />
                      ))}
                    </View>
                  </View>
                );
              })
            : null}
        </>
      ) : null}
    </View>
  );
}

function PartChip({ label, on, glass, onPress }: { label: string; on?: boolean; glass?: boolean; onPress?: () => void }) {
  const colors = useSchemeColors();
  const tone = on ? (glass ? 'border-success bg-success' : 'border-primary bg-primary') : 'border-border bg-card';
  // On dark, a selected chip wears the diagram's colors so it does not outshine the diagram.
  const style = on && colors.dark ? { backgroundColor: glass ? colors.windowOn : colors.bodyOn, borderColor: colors.strokeOn } : undefined;
  const body = (
    <Text variant="footnote" className={cn('font-semibold', on && !colors.dark && 'text-primary-foreground')}>
      {label}
    </Text>
  );
  if (!onPress) return <View style={style} className={cn('min-h-8 justify-center rounded-full border px-3', tone)}>{body}</View>;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ selected: !!on }}
      onPress={onPress}
      style={style}
      className={cn('min-h-9 justify-center rounded-full border px-3.5', tone)}>
      {body}
    </Pressable>
  );
}
