import { router } from 'expo-router';
import { Pressable, View } from 'react-native';

import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { Text } from '@/ui/text';

import { type EntityKind, KIND_ICONS } from './href';
import { useEntityHrefs } from './use-entity-href';

export type Ref = { uuid?: string | null; name: string; sub?: string | null };

/**
 * Label / record line of a detail section (TEC-1219): the record's icon and
 * name; with a screen for it (and the right to open it) a chevron, and a tap
 * pushes the record's detail. Otherwise the name is plain text. `id`
 * overrides the uuid where a screen is keyed otherwise (stock unit: barcode).
 * The record is `value`, not `ref`: React 19 reserves `ref`.
 */
export function RefRow({
  kind,
  value,
  label,
  id,
  last,
}: {
  kind: EntityKind;
  value: Ref | null | undefined;
  label: string;
  id?: string | null;
  last?: boolean;
}) {
  const hrefOf = useEntityHrefs();
  if (!value?.name) return null;
  const href = hrefOf(kind, id ?? value.uuid);
  const body = (
    <View className={cn('min-h-11 flex-row items-center gap-3 px-4 py-2.5', !last && 'border-b border-border')}>
      <Text variant="callout" tone="muted" className="w-32">
        {label}
      </Text>
      <View className="flex-1 flex-row items-center justify-end gap-2">
        <Icon name={KIND_ICONS[kind] as IconName} tone={href ? 'primary' : 'muted-foreground'} size={16} />
        <View className="shrink items-end">
          <Text variant="callout" tone={href ? 'primary' : 'default'} selectable={!href} numberOfLines={2} className="text-right">
            {value.name}
          </Text>
          {value.sub ? (
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {value.sub}
            </Text>
          ) : null}
        </View>
        {href ? <Icon name="forward" tone="muted-foreground" size={16} /> : null}
      </View>
    </View>
  );
  if (!href) return body;
  return (
    <Pressable accessibilityRole="link" accessibilityLabel={`${label}: ${value.name}`} onPress={() => router.push(href)} className="active:bg-muted">
      {body}
    </Pressable>
  );
}

/**
 * Inline record name inside a card or list line (order line product, stock
 * unit barcode): a link when the viewer can open it, plain text otherwise.
 */
export function RefText({
  kind,
  value,
  id,
  variant = 'body',
  numberOfLines,
  mono,
}: {
  kind: EntityKind;
  value: Ref | null | undefined;
  id?: string | null;
  variant?: 'body' | 'callout' | 'footnote';
  numberOfLines?: number;
  mono?: boolean;
}) {
  const hrefOf = useEntityHrefs();
  if (!value?.name) return null;
  const href = hrefOf(kind, id ?? value.uuid);
  return (
    <Text
      variant={variant}
      tone={href ? 'primary' : 'default'}
      numberOfLines={numberOfLines}
      className={cn(mono && 'font-mono')}
      accessibilityRole={href ? 'link' : undefined}
      onPress={href ? () => router.push(href) : undefined}>
      {value.name}
    </Text>
  );
}
