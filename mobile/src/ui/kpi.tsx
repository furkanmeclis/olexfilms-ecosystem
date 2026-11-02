import { Pressable, View } from 'react-native';

import { cn } from './cn';
import { Icon } from './icon';
import type { IconName } from './icons';
import { Skeleton } from './skeleton';
import { Text } from './text';

/** Two-column wrap of KPI cards; an odd last card spans the row. */
export function KpiGrid({ children, className }: { children: React.ReactNode; className?: string }) {
  return <View className={cn('flex-row flex-wrap gap-2.5', className)}>{children}</View>;
}

const CARD = 'min-h-[104px] grow basis-[46%] justify-between gap-3 rounded-2xl border border-border bg-card p-3.5';

/**
 * One KPI number (the dealer home look, used app-wide): a small icon and calm
 * label above, the number as the hero. With `onPress` the card is a button.
 */
export function KpiCard({
  icon,
  label,
  value,
  loading,
  onPress,
  accessibilityHint,
}: {
  icon: IconName;
  label: string;
  value: string;
  /** Keeps the card in place and shimmers the number while it loads. */
  loading?: boolean;
  onPress?: () => void;
  accessibilityHint?: string;
}) {
  const body = (
    <>
      <View className="flex-row items-center gap-2">
        <View className="size-7 items-center justify-center rounded-full bg-primary/10">
          <Icon name={icon} tone="primary" size={14} weight="medium" />
        </View>
        <Text variant="footnote" tone="muted" numberOfLines={2} className="flex-1">
          {label}
        </Text>
      </View>
      {loading ? (
        <Skeleton className="h-9 w-16 rounded-md" />
      ) : (
        <Text
          variant="title"
          className="text-[30px] font-bold leading-[36px] tracking-tight tabular-nums"
          numberOfLines={1}
          adjustsFontSizeToFit>
          {value}
        </Text>
      )}
    </>
  );
  const a11y = loading ? label : `${label}: ${value}`;
  if (!onPress) {
    return (
      <View accessible accessibilityLabel={a11y} className={CARD} style={{ borderCurve: 'continuous' }}>
        {body}
      </View>
    );
  }
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={a11y}
      accessibilityHint={accessibilityHint}
      onPress={onPress}
      className={cn(CARD, 'active:opacity-80')}
      style={{ borderCurve: 'continuous' }}>
      {body}
    </Pressable>
  );
}

/** Placeholder grid while the first numbers load. */
export function KpiGridSkeleton({ count = 4 }: { count?: number }) {
  return (
    <KpiGrid>
      {Array.from({ length: count }, (_, i) => (
        <Skeleton key={i} className="h-[104px] grow basis-[46%] rounded-2xl" />
      ))}
    </KpiGrid>
  );
}
